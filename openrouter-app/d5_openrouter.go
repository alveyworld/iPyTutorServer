package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// Timeouts per call type (design §9).
const (
	d5InterviewerTotalTimeout = 12 * time.Second
	// d5CoachingTimeout: a coaching reply runs to ~500 tokens and comes after the results,
	// outside the 10 s interview target. 12 s cut a live coaching reply short.
	d5CoachingTimeout = 30 * time.Second
	// d5OpeningTimeout: openings run to 300-400 tokens and come once per part; 12 s cut
	// some Bug openings off mid-reply in live tests.
	d5OpeningTimeout = 20 * time.Second
	// d5DraftTimeout: pre-drafted openings (Phase 2) use reasoning off the live path.
	d5DraftTimeout                 = 90 * time.Second
	d5InterviewerFirstTokenTimeout = 5 * time.Second
	d5InterviewerRetryWindow       = 4 * time.Second
	d5EvaluatorTimeout             = 30 * time.Second
	d5LevelsTimeout                = 5 * time.Second
	d5BugStrategyTimeout           = 8 * time.Second // reads the whole Bug conversation
)

// d5HTTPClient has no client-wide timeout: each call carries its own context deadline.
var d5HTTPClient = &http.Client{}

type d5Request struct {
	Model     string         `json:"model"`
	Messages  []chatMessage  `json:"messages"`
	Stream    bool           `json:"stream,omitempty"`
	MaxTokens int            `json:"max_tokens,omitempty"`
	Reasoning map[string]any `json:"reasoning,omitempty"`
	Provider  map[string]any `json:"provider,omitempty"`
	Stop      []string       `json:"stop,omitempty"`
	Usage     map[string]any `json:"usage,omitempty"`
}

type d5Usage struct {
	PromptTokens            int `json:"prompt_tokens"`
	CompletionTokens        int `json:"completion_tokens"`
	CompletionTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

type d5CallResult struct {
	Content  string
	Usage    d5Usage
	Provider string
	Finish   string
	TTFT     time.Duration
	Elapsed  time.Duration
	Cut      bool
	Attempts int
}

// d5Stops are stop sequences for a simulated candidate reply. Some providers ignore stop
// sequences, so requests also set require_parameters and Go cuts the stream itself.
var d5Stops = []string{"\nStudent:", "\nCandidate:", "\nYou:", "\nA:"}

// interviewerRequest builds an Interviewer call (design §10): streamed, reasoning off,
// capped, routed for throughput to providers that honour every parameter.
func interviewerRequest(model string, messages []chatMessage, maxTokens int) d5Request {
	return d5Request{
		Model:     resolveChatModel(model),
		Messages:  messages,
		Stream:    true,
		MaxTokens: maxTokens,
		Reasoning: map[string]any{"enabled": false},
		Provider:  map[string]any{"sort": "throughput", "require_parameters": true},
		Stop:      d5Stops,
		Usage:     map[string]any{"include": true},
	}
}

func evaluatorRequest(model string, messages []chatMessage) d5Request {
	return d5Request{
		Model:    resolveChatModel(model),
		Messages: messages,
		// Reasoning off: in live runs it took 7-60 s, ignored a reasoning.max_tokens cap,
		// and used the whole budget, returning empty briefs. Without it the brief is ~300
		// tokens and arrives within the student's typing time.
		MaxTokens: 700,
		Reasoning: map[string]any{"enabled": false},
		Provider:  map[string]any{"require_parameters": true},
		Usage:     map[string]any{"include": true},
	}
}

func levelsRequest(model string, messages []chatMessage) d5Request {
	return d5Request{
		Model:     resolveChatModel(model),
		Messages:  messages,
		MaxTokens: 60,
		Reasoning: map[string]any{"enabled": false},
		Provider:  map[string]any{"sort": "throughput", "require_parameters": true},
		Usage:     map[string]any{"include": true},
	}
}

func defectCheckRequest(model string, messages []chatMessage) d5Request {
	return d5Request{
		Model:     resolveChatModel(model),
		Messages:  messages,
		MaxTokens: 80,
		Reasoning: map[string]any{"enabled": false},
		Provider:  map[string]any{"sort": "throughput", "require_parameters": true},
		Usage:     map[string]any{"include": true},
	}
}

// withReasoningBudget turns reasoning on for a background call that is off the live path,
// with room in max_tokens for the reasoning tokens.
func withReasoningBudget(req d5Request) d5Request {
	req.Reasoning = map[string]any{"effort": "medium"}
	req.MaxTokens += 4000
	req.Provider = map[string]any{"require_parameters": true}
	return req
}

func d5Post(ctx context.Context, apiKey, sessionID string, req d5Request) (*http.Response, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	openRouterHeaders(httpReq, apiKey, sessionID)
	return d5HTTPClient.Do(httpReq)
}

// d5SimulatedReplyPattern matches a role label starting a line ("Student:", "You:"),
// the start of a made-up candidate reply. It is anchored to the line start so prose such
// as "here's what I'd like from you: ..." is not cut.
var d5SimulatedReplyPattern = regexp.MustCompile(`(?im)^[ \t>*_]*(?:student|candidate|applicant|you|user|me|a)[*_]*[ \t]*:`)

func d5SimulatedReplyCut(content string) int {
	if loc := d5SimulatedReplyPattern.FindStringIndex(content); loc != nil && loc[0] > 0 {
		return loc[0]
	}
	return -1
}

type d5StreamChunk struct {
	Provider string `json:"provider"`
	Choices  []struct {
		FinishReason *string `json:"finish_reason"`
		Delta        struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *d5Usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// d5StreamCompletion streams one completion into Go. It cancels the call when no content
// arrives within firstTokenTimeout, and when cut reports a cut point it truncates the
// content there and stops reading.
func d5StreamCompletion(ctx context.Context, apiKey, sessionID string, req d5Request, firstTokenTimeout time.Duration, cut func(string) int) (d5CallResult, error) {
	res := d5CallResult{}
	start := time.Now()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var gotToken atomic.Bool
	timer := time.AfterFunc(firstTokenTimeout, func() {
		if !gotToken.Load() {
			cancel()
		}
	})
	defer timer.Stop()

	resp, err := d5Post(ctx, apiKey, sessionID, req)
	if err != nil {
		res.Elapsed = time.Since(start)
		return res, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		res.Elapsed = time.Since(start)
		return res, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var content strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), maxBodySize)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue // SSE comments such as ": OPENROUTER PROCESSING"
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk d5StreamChunk
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if chunk.Error != nil {
			res.Content = content.String()
			res.Elapsed = time.Since(start)
			return res, errors.New(chunk.Error.Message)
		}
		if chunk.Provider != "" {
			res.Provider = chunk.Provider
		}
		if chunk.Usage != nil {
			res.Usage = *chunk.Usage
		}
		for _, c := range chunk.Choices {
			if c.Delta.Content != "" {
				if !gotToken.Load() {
					gotToken.Store(true)
					res.TTFT = time.Since(start)
				}
				content.WriteString(c.Delta.Content)
			}
			if c.FinishReason != nil && *c.FinishReason != "" {
				res.Finish = *c.FinishReason
			}
		}
		if cut != nil {
			if idx := cut(content.String()); idx >= 0 {
				text := content.String()[:idx]
				content.Reset()
				content.WriteString(text)
				res.Cut = true
				break
			}
		}
	}
	res.Content = content.String()
	res.Elapsed = time.Since(start)
	if err := scanner.Err(); err != nil && !res.Cut {
		if !gotToken.Load() {
			return res, fmt.Errorf("no content within %s: %w", firstTokenTimeout, err)
		}
		return res, err
	}
	return res, nil
}

// d5Complete runs one non-streamed completion.
func d5Complete(ctx context.Context, apiKey, sessionID string, req d5Request) (d5CallResult, error) {
	res := d5CallResult{}
	start := time.Now()
	resp, err := d5Post(ctx, apiKey, sessionID, req)
	if err != nil {
		res.Elapsed = time.Since(start)
		return res, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	res.Elapsed = time.Since(start)
	if err != nil {
		return res, err
	}
	if resp.StatusCode != http.StatusOK {
		return res, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw[:min(len(raw), 512)])))
	}
	var out struct {
		Provider string `json:"provider"`
		Choices  []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage d5Usage `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return res, err
	}
	if out.Error != nil {
		return res, errors.New(out.Error.Message)
	}
	res.Provider, res.Usage = out.Provider, out.Usage
	if len(out.Choices) > 0 {
		res.Content = out.Choices[0].Message.Content
		res.Finish = out.Choices[0].FinishReason
	}
	return res, nil
}

// runInterviewerCall makes the live call with the §9 policy: 12 s overall, 5 s to the
// first token, and one retry only when the first attempt failed fast without content.
func runInterviewerCall(apiKey, sessionID string, req d5Request) (d5CallResult, error) {
	return runInterviewerCallWithin(apiKey, sessionID, req, d5InterviewerTotalTimeout)
}

func runInterviewerCallWithin(apiKey, sessionID string, req d5Request, total time.Duration) (d5CallResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), total)
	defer cancel()
	var res d5CallResult
	var err error
	for attempt := 1; attempt <= 2; attempt++ {
		started := time.Now()
		res, err = d5StreamCompletion(ctx, apiKey, sessionID, req, d5InterviewerFirstTokenTimeout, d5SimulatedReplyCut)
		res.Attempts = attempt
		if err == nil && strings.TrimSpace(res.Content) != "" {
			return res, nil
		}
		if err == nil {
			err = errors.New("empty reply")
		}
		if res.TTFT > 0 || time.Since(started) >= d5InterviewerRetryWindow || ctx.Err() != nil {
			break
		}
	}
	return res, err
}

// runBackgroundCall makes a non-streamed call with a per-attempt timeout and up to
// attempts tries.
func runBackgroundCall(apiKey, sessionID string, req d5Request, timeout time.Duration, attempts int) (d5CallResult, error) {
	var res d5CallResult
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		res, err = d5Complete(ctx, apiKey, sessionID, req)
		cancel()
		res.Attempts = attempt
		if err == nil && strings.TrimSpace(res.Content) != "" {
			return res, nil
		}
		if err == nil {
			err = errors.New("empty reply")
		}
	}
	return res, err
}
