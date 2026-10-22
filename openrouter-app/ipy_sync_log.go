package main

import (
	"log"
	"strings"
)

// ipySyncLogContext correlates assessment sync log lines with a chat request.
type ipySyncLogContext struct {
	sessionID  string
	turnID     string
	modeTurn   int
	activeMode string
	phase      string
}

func normalizeSyncPortionBody(raw string) string {
	jsonText := strings.TrimSpace(raw)
	jsonText = strings.TrimSuffix(jsonText, "```")
	return strings.TrimSpace(jsonText)
}

func collectIPyIntervuSyncPortions(assistant string) (complete []string, partial string) {
	for _, match := range ipyintervuBlockPattern.FindAllStringSubmatch(assistant, -1) {
		if len(match) >= 2 {
			body := normalizeSyncPortionBody(match[1])
			if body != "" {
				complete = append(complete, body)
			}
		}
	}
	if len(complete) > 0 {
		return complete, ""
	}
	if match := partialIPyIntervuOpenPattern.FindStringSubmatch(assistant); len(match) == 2 {
		partial = normalizeSyncPortionBody(match[1])
	}
	return complete, partial
}

func logIPyIntervuSyncPortions(ctx ipySyncLogContext, complete []string, partial string) {
	for i, body := range complete {
		log.Printf("[openrouter] assessment_sync_portion session=%s turn_id=%s mode_turn=%d conversation_phase=%s active_mode=%s kind=complete index=%d body=%q",
			truncateSessionID(ctx.sessionID),
			truncateTurnID(ctx.turnID),
			ctx.modeTurn,
			ctx.phase,
			ctx.activeMode,
			i,
			truncateForLog(body, 800),
		)
	}
	if partial != "" {
		log.Printf("[openrouter] assessment_sync_portion session=%s turn_id=%s mode_turn=%d conversation_phase=%s active_mode=%s kind=partial body=%q",
			truncateSessionID(ctx.sessionID),
			truncateTurnID(ctx.turnID),
			ctx.modeTurn,
			ctx.phase,
			ctx.activeMode,
			truncateForLog(partial, 800),
		)
	}
	if len(complete) == 0 && partial == "" &&
		ctx.phase == phaseAssessmentInProgress &&
		ctx.activeMode != "" && ctx.activeMode != modeCoaching {
		log.Printf("[openrouter] assessment_sync_portion session=%s turn_id=%s mode_turn=%d conversation_phase=%s active_mode=%s kind=absent",
			truncateSessionID(ctx.sessionID),
			truncateTurnID(ctx.turnID),
			ctx.modeTurn,
			ctx.phase,
			ctx.activeMode,
		)
	}
}
