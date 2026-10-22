package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

const defaultChatModel = "deepseek/deepseek-v4-flash-0731"

func resolveChatModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return defaultChatModel
	}
	return model
}

type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type bootstrapResponse struct {
	Assistant string `json:"assistant"`
}

type openRouterCompletion struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func lastUserMessage(messages []chatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

func prependSystemPrompt(body []byte, prompt string) ([]byte, error) {
	var req chatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	system := chatMessage{Role: "system", Content: prompt}
	if len(req.Messages) > 0 && req.Messages[0].Role == "system" {
		req.Messages[0] = system
	} else {
		req.Messages = append([]chatMessage{system}, req.Messages...)
	}

	return json.Marshal(req)
}

func openRouterHeaders(req *http.Request, apiKey string, sessionID string) {
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("x-custom-header", sessionID)
	}
	if referer := os.Getenv("OPENROUTER_HTTP_REFERER"); referer != "https://aalang.org" {
		req.Header.Set("HTTP-Referer", referer)
	}
	if title := os.Getenv("OPENROUTER_APP_TITLE"); title != "iPyInterVu" {
		req.Header.Set("X-Title", title)
	}
}

// handleBootstrap returns the fixed server-authored welcome; primes cache in background.
func handleBootstrap(states *agentStateStore, apiKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID, ok := sessionIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		state := states.getOrCreate(sessionID)
		applyBootstrapState(state, setupWelcomeMessage)
		states.set(sessionID, state)

		// Prime base bundle cache in background (async). The D5 engine sends no base bundle.
		if d5Enabled() {
			writeJSON(w, http.StatusOK, bootstrapResponse{Assistant: setupWelcomeMessage})
			return
		}
		go func() {
			baseBundle, err := buildBaseBundle()
			if err != nil {
				log.Printf("[cache] base_bundle_build_failed: %v", err)
				return
			}
			if err := primeCacheWithPrompt(context.Background(), apiKey, baseBundle); err != nil {
				log.Printf("[cache] base_bundle_prime_failed: %v", err)
			}
		}()

		writeJSON(w, http.StatusOK, bootstrapResponse{Assistant: setupWelcomeMessage})
	}
}

func prependSystemMessage(messages []chatMessage, prompt string) []chatMessage {
	system := chatMessage{Role: "system", Content: prompt}
	if len(messages) > 0 && messages[0].Role == "system" {
		out := make([]chatMessage, len(messages))
		copy(out, messages)
		out[0] = system
		return out
	}
	return append([]chatMessage{system}, messages...)
}

func extractAssistantContent(body []byte) string {
	var completion openRouterCompletion
	if err := json.Unmarshal(body, &completion); err != nil {
		return ""
	}
	if len(completion.Choices) == 0 {
		return ""
	}
	return completion.Choices[0].Message.Content
}

func handleSessionState(states *agentStateStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID, ok := sessionIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		state, ok := states.get(sessionID)
		if !ok {
			state = newAgentSessionState()
		}
		writeJSON(w, http.StatusOK, state)
	}
}
