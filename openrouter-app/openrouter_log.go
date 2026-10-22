package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"syscall"
)

// openRouterLogCtx correlates log lines for one chat/upstream operation.
type openRouterLogCtx struct {
	sessionID string
	turnID    string
	operation string // bootstrap, chat_body
	turn      int
}

func newOpenRouterLogCtx(sessionID, turnID, operation string, turn int) openRouterLogCtx {
	return openRouterLogCtx{sessionID: sessionID, turnID: turnID, operation: operation, turn: turn}
}

func (c openRouterLogCtx) sessionShort() string {
	if len(c.sessionID) <= 8 {
		return c.sessionID
	}
	return c.sessionID[:8]
}

func classifyOpenRouterError(err error, sentToClient bool) string {
	if err == nil {
		return "none"
	}
	if errors.Is(err, context.Canceled) {
		return "context_canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "openrouter_timeout"
	}
	if errors.Is(err, io.EOF) {
		return "upstream_eof"
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return "upstream_unexpected_eof"
	}
	if errors.Is(err, syscall.EPIPE) {
		return "client_broken_pipe"
	}
	if errors.Is(err, syscall.ECONNRESET) {
		if sentToClient {
			return "client_connection_reset"
		}
		return "upstream_connection_reset"
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			if sentToClient {
				return "client_timeout"
			}
			return "openrouter_timeout"
		}
		return "network_error"
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return "network_op_error"
	}

	return "unknown_error"
}

func logOpenRouterStart(c openRouterLogCtx, attempt int) {
	log.Printf("[openrouter] start session=%s turn_id=%s operation=%s mode_turn=%d attempt=%d/%d",
		c.sessionShort(), truncateTurnID(c.turnID), c.operation, c.turn, attempt+1, openRouterMaxAttempts)
}

func logOpenRouterConnected(c openRouterLogCtx, attempt int, statusCode int, contentType string, elapsedMs int64) {
	log.Printf("[openrouter] connected session=%s turn_id=%s operation=%s mode_turn=%d attempt=%d status=%d content_type=%q elapsed_ms=%d",
		c.sessionShort(), truncateTurnID(c.turnID), c.operation, c.turn, attempt+1, statusCode, contentType, elapsedMs)
}

func logOpenRouterFailure(c openRouterLogCtx, attempt int, failurePoint string, err error, sentToClient bool, extra string) {
	failureClass := classifyOpenRouterError(err, sentToClient)
	log.Printf("[openrouter] failure session=%s turn_id=%s operation=%s mode_turn=%d attempt=%d failure_point=%s failure_class=%s sent_to_client=%v extra=%s err=%q",
		c.sessionShort(), truncateTurnID(c.turnID), c.operation, c.turn, attempt+1, failurePoint, failureClass, sentToClient, extra, err.Error())
}

func logOpenRouterRetry(c openRouterLogCtx, attempt int, reason string) {
	log.Printf("[openrouter] retry session=%s turn_id=%s operation=%s mode_turn=%d next_attempt=%d/%d reason=%s",
		c.sessionShort(), truncateTurnID(c.turnID), c.operation, c.turn, attempt+2, openRouterMaxAttempts, reason)
}

func logOpenRouterStatusRetry(c openRouterLogCtx, attempt int, statusCode int, bodyPreview string) {
	log.Printf("[openrouter] status_retry session=%s turn_id=%s operation=%s mode_turn=%d attempt=%d status=%d body_preview=%q",
		c.sessionShort(), truncateTurnID(c.turnID), c.operation, c.turn, attempt+1, statusCode, truncateForLog(bodyPreview, 200))
}

func logChatRequestDone(sessionID, turnID string, elapsedMs int64, modeContinuations int) {
	log.Printf("[openrouter] chat_done session=%s turn_id=%s elapsed_ms=%d mode_continuations=%d",
		truncateSessionID(sessionID), truncateTurnID(turnID), elapsedMs, modeContinuations)
}

func truncateSessionID(sessionID string) string {
	if len(sessionID) <= 8 {
		return sessionID
	}
	return sessionID[:8]
}

func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
