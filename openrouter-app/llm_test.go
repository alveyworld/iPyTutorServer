package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"
)

// providerFake streams reply(ignore) for each request and records the providers each
// request asked OpenRouter to skip.
type providerFake struct {
	mu      sync.Mutex
	ignores [][]string
	reply   func(attempt int, ignore []string) (provider, content string)
}

func (f *providerFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider struct {
			Ignore []string `json:"ignore"`
		} `json:"provider"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	f.mu.Lock()
	f.ignores = append(f.ignores, req.Provider.Ignore)
	attempt := len(f.ignores)
	f.mu.Unlock()
	provider, content := f.reply(attempt, req.Provider.Ignore)
	chunk, _ := json.Marshal(map[string]any{"provider": provider, "choices": []map[string]any{{"delta": map[string]string{"content": content}}}})
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
}

const endOfSentence = "<｜end▁of▁sentence｜>"

var testSpec = callSpec{Name: "test", MaxTokens: 50, Timeout: 10 * time.Second}

func TestEmptyReplyRetriesOnAnotherProvider(t *testing.T) {
	fake := &providerFake{reply: func(_ int, ignore []string) (string, string) {
		if slices.Contains(ignore, "DeepInfra") {
			return "Baidu", "TITLE: A real problem"
		}
		return "DeepInfra", endOfSentence
	}}
	server := httptest.NewServer(fake)
	defer server.Close()

	res, err := llmClient{apiKey: "k", url: server.URL}.call(context.Background(), testSpec, []chatMessage{{Role: "system", Content: "Write a problem."}})
	if err != nil || res.Content != "TITLE: A real problem" || res.Provider != "Baidu" || res.Attempts != 2 {
		t.Fatalf("call = %+v, %v", res, err)
	}
	if len(fake.ignores) != 2 || fake.ignores[0] != nil || !slices.Equal(fake.ignores[1], []string{"DeepInfra"}) {
		t.Fatalf("ignored providers per request = %q", fake.ignores)
	}
}

func TestRepeatedEmptyRepliesGiveUp(t *testing.T) {
	fake := &providerFake{reply: func(attempt int, _ []string) (string, string) {
		return fmt.Sprintf("Provider%d", attempt), endOfSentence
	}}
	server := httptest.NewServer(fake)
	defer server.Close()

	_, err := llmClient{apiKey: "k", url: server.URL}.call(context.Background(), testSpec, []chatMessage{{Role: "user", Content: "Hi"}})
	if !errors.Is(err, errEmptyReply) {
		t.Fatalf("err = %v, want errEmptyReply", err)
	}
	want := [][]string{nil, {"Provider1"}, {"Provider1", "Provider2"}}
	if len(fake.ignores) != llmMaxAttempts {
		t.Fatalf("made %d attempts, want %d", len(fake.ignores), llmMaxAttempts)
	}
	for i := range want {
		if !slices.Equal(fake.ignores[i], want[i]) {
			t.Fatalf("attempt %d ignored %q, want %q", i+1, fake.ignores[i], want[i])
		}
	}
}

func TestControlTokensAreStripped(t *testing.T) {
	for in, want := range map[string]string{
		"Sure, here it is." + endOfSentence: "Sure, here it is.",
		endOfSentence:                       "",
		"Done<|im_end|>":                    "Done",
		"<|endoftext|>":                     "",
		"if a <| b: use <b>bold</b>":        "if a <| b: use <b>bold</b>",
		"<reply>\nHello\n</reply>":          "<reply>\nHello\n</reply>",
	} {
		if got := specialToken.ReplaceAllString(in, ""); got != want {
			t.Errorf("strip(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEveryRequestEndsWithAStudentMessage(t *testing.T) {
	system := chatMessage{Role: "system", Content: "Teach."}
	for _, msgs := range [][]chatMessage{
		{system},
		{system, {Role: "assistant", Content: "Here is the problem."}},
	} {
		got := testSpec.request(msgs, nil).Messages
		if last := got[len(got)-1]; last.Role != "user" || len(got) != len(msgs)+1 {
			t.Errorf("request(%v) ends with %+v", msgs, last)
		}
	}
	withUser := []chatMessage{system, {Role: "user", Content: "Hint please"}}
	if got := testSpec.request(withUser, nil).Messages; len(got) != 2 || got[1].Content != "Hint please" {
		t.Errorf("a conversation already ending with the student changed: %+v", got)
	}
	if p := testSpec.request(withUser, []string{"DeepInfra"}).Provider["ignore"]; !slices.Equal(p.([]string), []string{"DeepInfra"}) {
		t.Errorf("provider ignore = %v", p)
	}
}
