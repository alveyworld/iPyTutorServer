package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"syscall"
	"time"
)

const (
	openRouterMaxAttempts = 3
	openRouterRetryBase   = 1500 * time.Millisecond
)

func isRetryableOpenRouterError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	return errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, context.DeadlineExceeded)
}

func isRetryableOpenRouterStatus(status int) bool {
	return status == http.StatusTooManyRequests ||
		status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable ||
		status == http.StatusGatewayTimeout
}

func openRouterRetryDelay(attempt int) time.Duration {
	return openRouterRetryBase * time.Duration(attempt+1)
}

func postOpenRouterOnce(ctx context.Context, apiKey string, payload []byte, sessionID string) (*http.Response, error) {
	upstream, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	openRouterHeaders(upstream, apiKey, sessionID)
	return openRouterClient.Do(upstream)
}

func waitOpenRouterRetry(ctx context.Context, attempt int, logCtx openRouterLogCtx) error {
	if attempt == 0 {
		return nil
	}
	delay := openRouterRetryDelay(attempt - 1)
	logOpenRouterRetry(logCtx, attempt-1, "backoff")
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

func primeCacheWithPrompt(ctx context.Context, apiKey string, prompt string) error {
	// Send prompt to prime OpenRouter's cache
	// This caches the prompt prefix for reuse in subsequent requests
	payload, err := json.Marshal(chatCompletionRequest{
		Model: resolveChatModel(""),
		Messages: []chatMessage{
			{Role: "system", Content: prompt},
			{Role: "user", Content: "Ready."},
		},
	})
	if err != nil {
		return err
	}

	resp, err := postOpenRouterOnce(ctx, apiKey, payload, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, _ = io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if resp.StatusCode != http.StatusOK {
		log.Printf("[cache] prime_failed status=%d", resp.StatusCode)
		return errors.New("cache prime failed")
	}
	log.Printf("[cache] primed_base_bundle status=%d", resp.StatusCode)
	return nil
}

func readOpenRouterBodyWithRetry(ctx context.Context, apiKey string, payload []byte, logCtx openRouterLogCtx) (body []byte, statusCode int, err error) {
	var lastErr error

	for attempt := 0; attempt < openRouterMaxAttempts; attempt++ {
		if err := waitOpenRouterRetry(ctx, attempt, logCtx); err != nil {
			return nil, 0, err
		}

		attemptStarted := time.Now()
		logOpenRouterStart(logCtx, attempt)

		resp, err := postOpenRouterOnce(ctx, apiKey, payload, logCtx.sessionID)
		if err != nil {
			lastErr = err
			logOpenRouterFailure(logCtx, attempt, "connect", err, false, "read_body")
			if attempt < openRouterMaxAttempts-1 && isRetryableOpenRouterError(err) {
				logOpenRouterRetry(logCtx, attempt, classifyOpenRouterError(err, false))
				continue
			}
			return nil, 0, err
		}

		statusCode = resp.StatusCode
		contentType := resp.Header.Get("Content-Type")
		logOpenRouterConnected(logCtx, attempt, statusCode, contentType, time.Since(attemptStarted).Milliseconds())

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			logOpenRouterFailure(logCtx, attempt, "upstream_read", readErr, false, "read_body")
			if attempt < openRouterMaxAttempts-1 && isRetryableOpenRouterError(readErr) {
				logOpenRouterRetry(logCtx, attempt, classifyOpenRouterError(readErr, false))
				continue
			}
			return nil, statusCode, readErr
		}

		if isRetryableOpenRouterStatus(statusCode) {
			lastErr = errors.New(string(body))
			logOpenRouterStatusRetry(logCtx, attempt, statusCode, string(body))
			if attempt < openRouterMaxAttempts-1 {
				continue
			}
			return body, statusCode, lastErr
		}

		log.Printf("[openrouter] body_ok session=%s operation=%s turn=%d attempt=%d body_bytes=%d elapsed_ms=%d",
			logCtx.sessionShort(), logCtx.operation, logCtx.turn, attempt+1, len(body), time.Since(attemptStarted).Milliseconds())
		return body, statusCode, nil
	}

	if lastErr != nil {
		return nil, 0, lastErr
	}
	return nil, 0, errors.New("openrouter request failed")
}
