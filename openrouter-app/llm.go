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
	"os"
	"strings"
	"sync/atomic"
	"time"
)

const (
	defaultTutorModel    = "deepseek/deepseek-v4-flash-0731"
	defaultOpenRouterURL = "https://openrouter.ai/api/v1/chat/completions"
)

// llmClient calls OpenRouter's chat completions endpoint. url is a field, not a global, so
// tests (and OPENROUTER_URL for local runs) can point each tutor at its own fake upstream.
type llmClient struct {
	apiKey string
	url    string
}

func tutorModel() string {
	if m := strings.TrimSpace(os.Getenv("TUTOR_MODEL")); m != "" {
		return m
	}
	return defaultTutorModel
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// llmHTTPClient has no client-wide timeout: each call carries its own context deadline.
var llmHTTPClient = &http.Client{}

type llmRequest struct {
	Model     string         `json:"model"`
	Messages  []chatMessage  `json:"messages"`
	Stream    bool           `json:"stream"`
	MaxTokens int            `json:"max_tokens,omitempty"`
	Reasoning map[string]any `json:"reasoning,omitempty"`
	Provider  map[string]any `json:"provider,omitempty"`
	Usage     map[string]any `json:"usage,omitempty"`
}

type llmUsage struct {
	PromptTokens            int `json:"prompt_tokens"`
	CompletionTokens        int `json:"completion_tokens"`
	CompletionTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

type llmResult struct {
	Content  string
	Usage    llmUsage
	Provider string
	Finish   string
	TTFT     time.Duration
	Elapsed  time.Duration
	Attempts int
}

// callSpec sets the limits for one kind of model call.
type callSpec struct {
	Name      string
	MaxTokens int
	Timeout   time.Duration
	// Reasoning is "" for none, or an effort such as "low" or "medium". Reasoning makes
	// practice problems more reliable (a bug hunt whose code really has the bug) but slow,
	// so it is used for problems prepared in the background.
	Reasoning string
}

// Live calls answer the student directly; their output limits leave room for a lesson or a
// worked solution with code.
var (
	callLesson   = callSpec{Name: "lesson", MaxTokens: 1400, Timeout: 60 * time.Second}
	callChat     = callSpec{Name: "chat", MaxTokens: 900, Timeout: 45 * time.Second}
	callFeedback = callSpec{Name: "feedback", MaxTokens: 1000, Timeout: 50 * time.Second}
	callHint     = callSpec{Name: "hint", MaxTokens: 400, Timeout: 30 * time.Second}
	callSolution = callSpec{Name: "solution", MaxTokens: 1400, Timeout: 60 * time.Second}
	callProblem  = callSpec{Name: "problem", MaxTokens: 1600, Timeout: 50 * time.Second}
	// callPreparedProblem writes a practice problem ahead of time, off the live path.
	callPreparedProblem = callSpec{Name: "problem_prepared", MaxTokens: 1600, Timeout: 150 * time.Second, Reasoning: "medium"}
)

const (
	llmFirstTokenTimeout          = 12 * time.Second
	llmReasoningFirstTokenTimeout = 60 * time.Second
	// llmRetryWindow: an attempt that fails this fast, without content, is retried once.
	llmRetryWindow = 5 * time.Second
)

func (spec callSpec) request(messages []chatMessage) llmRequest {
	req := llmRequest{
		Model:     tutorModel(),
		Messages:  messages,
		Stream:    true,
		MaxTokens: spec.MaxTokens,
		Reasoning: map[string]any{"enabled": false},
		Provider:  map[string]any{"sort": "throughput", "require_parameters": true},
		Usage:     map[string]any{"include": true},
	}
	if spec.Reasoning != "" {
		req.Reasoning = map[string]any{"effort": spec.Reasoning}
		// Reasoning tokens count against max_tokens.
		req.MaxTokens += 6000
		req.Provider = map[string]any{"require_parameters": true}
	}
	return req
}

func openRouterHeaders(req *http.Request, apiKey string) {
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	if referer := os.Getenv("OPENROUTER_HTTP_REFERER"); referer != "" {
		req.Header.Set("HTTP-Referer", referer)
	}
	title := os.Getenv("OPENROUTER_APP_TITLE")
	if title == "" {
		title = "iPyTutor"
	}
	req.Header.Set("X-Title", title)
}

func (c llmClient) post(ctx context.Context, req llmRequest) (*http.Response, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	openRouterHeaders(httpReq, c.apiKey)
	return llmHTTPClient.Do(httpReq)
}

type llmStreamChunk struct {
	Provider string `json:"provider"`
	Choices  []struct {
		FinishReason *string `json:"finish_reason"`
		Delta        struct {
			Content   string `json:"content"`
			Reasoning string `json:"reasoning"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *llmUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// stream streams one completion. It cancels the call when nothing (content or reasoning)
// arrives within firstTokenTimeout.
func (c llmClient) stream(ctx context.Context, req llmRequest, firstTokenTimeout time.Duration) (llmResult, error) {
	res := llmResult{}
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

	resp, err := c.post(ctx, req)
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
		var chunk llmStreamChunk
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
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" || choice.Delta.Reasoning != "" {
				if !gotToken.Load() {
					gotToken.Store(true)
					res.TTFT = time.Since(start)
				}
			}
			content.WriteString(choice.Delta.Content)
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				res.Finish = *choice.FinishReason
			}
		}
	}
	res.Content = content.String()
	res.Elapsed = time.Since(start)
	if err := scanner.Err(); err != nil {
		if !gotToken.Load() {
			return res, fmt.Errorf("no reply within %s: %w", firstTokenTimeout, err)
		}
		return res, err
	}
	return res, nil
}

// call makes one call within spec.Timeout, retrying once when the first attempt failed
// fast without any content.
func (c llmClient) call(ctx context.Context, spec callSpec, messages []chatMessage) (llmResult, error) {
	ctx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()
	firstToken := llmFirstTokenTimeout
	if spec.Reasoning != "" {
		firstToken = llmReasoningFirstTokenTimeout
	}
	req := spec.request(messages)
	var res llmResult
	var err error
	for attempt := 1; attempt <= 2; attempt++ {
		started := time.Now()
		res, err = c.stream(ctx, req, firstToken)
		res.Attempts = attempt
		if err == nil && strings.TrimSpace(res.Content) != "" {
			return res, nil
		}
		if err == nil {
			err = errors.New("empty reply")
		}
		if res.TTFT > 0 || time.Since(started) >= llmRetryWindow || ctx.Err() != nil {
			break
		}
	}
	return res, err
}
