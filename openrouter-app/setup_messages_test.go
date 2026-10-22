package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// Drives the real handlers through setup: the welcome, major acknowledgment, weekly list,
// and invalid-selection reply are server-authored, so OpenRouter is not called until a
// key concept is chosen.
func TestSetupRepliesAreServerAuthored(t *testing.T) {
	var upstreamCalls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if isCachePrimeRequest(req) {
			return
		}
		atomic.AddInt32(&upstreamCalls, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant",
				"content": "I'm Alex at ChemCore. What would you identify as the input?\n\n```_ipyintervu\n{\"conceptualAssessmentPhase\": \"in_progress\"}\n```"}}},
		})
	}))
	defer upstream.Close()
	saved := openRouterURL
	openRouterURL = upstream.URL
	defer func() { openRouterURL = saved }()

	states := newAgentStateStore()
	turns := newTurnStore()
	withSession := func(req *http.Request) *http.Request {
		return req.WithContext(context.WithValue(req.Context(), sessionIDContextKey, "session-1"))
	}

	rec := httptest.NewRecorder()
	handleBootstrap(states, "test-key")(rec, withSession(httptest.NewRequest(http.MethodPost, "/api/session/bootstrap", strings.NewReader(`{}`))))
	var boot bootstrapResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &boot); err != nil || boot.Assistant != setupWelcomeMessage {
		t.Fatalf("bootstrap = %q (err %v), want fixed welcome", boot.Assistant, err)
	}

	turn := 0
	chat := func(text string) string {
		t.Helper()
		turn++
		body, _ := json.Marshal(chatCompletionRequest{Messages: []chatMessage{{Role: "user", Content: text}}})
		req := withSession(httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(string(body))))
		req.Header.Set(turnIDHeader, "turn-"+string(rune('a'+turn)))
		rec := httptest.NewRecorder()
		handleChat("test-key", states, turns)(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("chat %q status %d: %s", text, rec.Code, rec.Body.String())
		}
		return extractAssistantContent(rec.Body.Bytes())
	}

	if got := chat("hi"); got != setupReaskMajorMessage {
		t.Fatalf("greeting reply = %q", got)
	}
	got := chat("chemistry")
	if !strings.HasPrefix(got, "Thanks - I have your major as chemistry.") || !strings.Contains(got, "- Week 9 - Lists and Files") {
		t.Fatalf("major reply = %q", got)
	}
	if got := chat("week 12"); !strings.HasPrefix(got, "That isn't one of the listed key concepts.") || !strings.Contains(got, "- Week 1 - Problem Decomposition") {
		t.Fatalf("invalid selection reply = %q", got)
	}
	if n := atomic.LoadInt32(&upstreamCalls); n != 0 {
		t.Fatalf("setup turns called OpenRouter %d times", n)
	}

	if got := chat("week 1"); !strings.Contains(got, "I'm Alex") {
		t.Fatalf("week selection should reach the model, got %q", got)
	}
	if n := atomic.LoadInt32(&upstreamCalls); n != 1 {
		t.Fatalf("expected 1 OpenRouter call after selecting a week, got %d", n)
	}
}
