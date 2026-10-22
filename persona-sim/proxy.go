package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const openRouterUpstream = "https://openrouter.ai/api/v1/chat/completions"

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type upstreamCall struct {
	Seq               int             `json:"seq"`
	Time              time.Time       `json:"time"`
	SessionID         string          `json:"sessionId"`
	Model             string          `json:"model"`
	SystemPromptSHA   string          `json:"systemPromptSha,omitempty"`
	SystemPromptChars int             `json:"systemPromptChars"`
	Messages          []chatMessage   `json:"messages"`
	Handoffs          []string        `json:"handoffs,omitempty"`
	Status            int             `json:"status"`
	LatencyMs         int64           `json:"latencyMs"`
	Response          string          `json:"response"`
	FinishReason      string          `json:"finishReason,omitempty"`
	Usage             json.RawMessage `json:"usage,omitempty"`
	Error             string          `json:"error,omitempty"`
}

// recordingProxy forwards the server's OpenRouter calls and keeps a copy of each
// request/response, keyed by the session ID the server sends in x-custom-header.
type recordingProxy struct {
	apiURL     string
	promptsDir string
	client     *http.Client

	mu        sync.Mutex
	seq       int
	bySession map[string][]upstreamCall
	prompts   map[string]bool
}

func startRecordingProxy(promptsDir string) (*recordingProxy, error) {
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &recordingProxy{
		apiURL:     "http://" + ln.Addr().String() + "/api/v1/chat/completions",
		promptsDir: promptsDir,
		client:     &http.Client{Timeout: 150 * time.Second},
		bySession:  map[string][]upstreamCall{},
		prompts:    map[string]bool{},
	}
	go http.Serve(ln, p)
	return p, nil
}

func (p *recordingProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqBody, _ := io.ReadAll(r.Body)
	call := upstreamCall{Time: time.Now(), SessionID: r.Header.Get("x-custom-header")}
	p.parseRequest(&call, reqBody)

	upReq, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, openRouterUpstream, bytes.NewReader(reqBody))
	for _, h := range []string{"Authorization", "Content-Type", "x-custom-header", "HTTP-Referer", "X-Title"} {
		if v := r.Header.Get(h); v != "" {
			upReq.Header.Set(h, v)
		}
	}
	started := time.Now()
	resp, err := p.client.Do(upReq)
	call.LatencyMs = time.Since(started).Milliseconds()
	if err != nil {
		call.Error = err.Error()
		p.record(call)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	call.Status = resp.StatusCode
	if err != nil {
		call.Error = err.Error()
	}
	parseResponse(&call, respBody)
	p.record(call)

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

func (p *recordingProxy) parseRequest(call *upstreamCall, body []byte) {
	var req struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		call.Error = "unparseable request: " + err.Error()
		return
	}
	call.Model = req.Model
	for _, m := range req.Messages {
		text := contentText(m.Content)
		if m.Role == "system" {
			call.SystemPromptSHA = p.savePrompt(text)
			call.SystemPromptChars = len(text)
			continue
		}
		call.Messages = append(call.Messages, chatMessage{Role: m.Role, Content: text})
		if strings.Contains(text, "[System") {
			call.Handoffs = append(call.Handoffs, text)
		}
	}
}

// contentText accepts both a plain string and an array of {type,text} parts.
func contentText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var b strings.Builder
		for _, part := range parts {
			b.WriteString(part.Text)
		}
		return b.String()
	}
	return string(raw)
}

func parseResponse(call *upstreamCall, body []byte) {
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Choices) == 0 {
		if call.Error == "" {
			call.Error = "no choices: " + truncate(string(body), 500)
		}
		return
	}
	call.Response = resp.Choices[0].Message.Content
	call.FinishReason = resp.Choices[0].FinishReason
	call.Usage = resp.Usage
}

func (p *recordingProxy) savePrompt(text string) string {
	sum := sha256.Sum256([]byte(text))
	id := hex.EncodeToString(sum[:])[:16]
	p.mu.Lock()
	seen := p.prompts[id]
	p.prompts[id] = true
	p.mu.Unlock()
	if !seen {
		_ = os.WriteFile(filepath.Join(p.promptsDir, id+".md"), []byte(text), 0o644)
	}
	return id
}

func (p *recordingProxy) record(call upstreamCall) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seq++
	call.Seq = p.seq
	p.bySession[call.SessionID] = append(p.bySession[call.SessionID], call)
}

func (p *recordingProxy) count(sessionID string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.bySession[sessionID])
}

func (p *recordingProxy) calls(sessionID string) []upstreamCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]upstreamCall(nil), p.bySession[sessionID]...)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
