package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func turnRoleName(role turnRole) string {
	switch role {
	case turnRoleLeader:
		return "leader"
	case turnRoleFollower:
		return "follower"
	case turnRoleReplayCompleted:
		return "replay_completed"
	case turnRoleReplayFailed:
		return "replay_failed"
	case turnRoleResumeLeader:
		return "resume_leader"
	default:
		return "unknown"
	}
}

func truncateTurnID(turnID string) string {
	if len(turnID) <= 12 {
		return turnID
	}
	return turnID[:12]
}

func handleChat(apiKey string, states *agentStateStore, turns *turnStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID, ok := sessionIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}

		var req chatCompletionRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		turnID := strings.TrimSpace(r.Header.Get(turnIDHeader))
		state := states.getOrCreate(sessionID)
		userMessage := lastUserMessage(req.Messages)

		var turnRec *chatTurnRecord
		var turnRole turnRole
		skipPreChat := false

		if turnID != "" {
			acquired := turns.acquire(sessionID, turnID)
			turnRec = acquired.record
			turnRole = acquired.role

			log.Printf("[openrouter] turn_acquire session=%s turn_id=%s role=%s status=%s",
				truncateSessionID(sessionID), truncateTurnID(turnID), turnRoleName(turnRole), turnRec.status)

			switch turnRole {
			case turnRoleFollower:
				if !turnRec.wait.wait(r.Context()) {
					return
				}
				writeTurnResponse(w, turnRec)
				return
			case turnRoleReplayCompleted, turnRoleReplayFailed:
				writeTurnResponse(w, turnRec)
				return
			case turnRoleResumeLeader:
				skipPreChat = true
			case turnRoleLeader:
				// apply pre-chat below
			}
		}

		if d5Enabled() {
			handleD5Chat(chatRunParams{
				sessionID:   sessionID,
				turnID:      turnID,
				turnRec:     turnRec,
				turnRole:    turnRole,
				turns:       turns,
				states:      states,
				state:       state,
				req:         req,
				userMessage: userMessage,
				apiKey:      apiKey,
				r:           r,
				w:           w,
			}, skipPreChat)
			return
		}

		phaseBefore := state.ConversationPhase
		weekNumberBefore := state.CurrentWeekNumber

		if !skipPreChat {
			applyPreChatUserUpdate(state, userMessage)
		}

		// Cache management: when week is first selected, prime week rubric cache
		if weekNumberBefore == 0 && state.CurrentWeekNumber > 0 {
			// Prime week rubric cache asynchronously
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				weekRubric, err := buildWeekRubric(state.CurrentWeekNumber)
				if err == nil && weekRubric != "" {
					_ = primeCacheWithPrompt(ctx, apiKey, weekRubric)
					log.Printf("[cache] primed_week_rubric session=%s week=%d", truncateSessionID(sessionID), state.CurrentWeekNumber)
				}
			}()
		}

		states.set(sessionID, state)

		log.Printf("[openrouter] chat_start session=%s turn_id=%s active_mode=%s phase=%s message_index=%d",
			truncateSessionID(sessionID), truncateTurnID(turnID), state.ActiveMode, state.ConversationPhase, state.MessageIndex)

		p := chatRunParams{
			sessionID:   sessionID,
			turnID:      turnID,
			turnRec:     turnRec,
			turnRole:    turnRole,
			turns:       turns,
			states:      states,
			state:       state,
			req:         req,
			userMessage: userMessage,
			apiKey:      apiKey,
			r:           r,
			w:           w,
		}
		if reply, ok := serverSetupReply(phaseBefore, state); ok {
			writeServerSetupReply(p, reply)
			return
		}
		runChatRequest(p)
	}
}

// writeServerSetupReply answers a setup-phase turn with fixed server text, shaped like an
// OpenRouter completion so the browser handles it the same way.
func writeServerSetupReply(p chatRunParams, content string) {
	body, err := json.Marshal(map[string]any{
		"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": content}}},
	})
	if err != nil {
		http.Error(p.w, "internal error", http.StatusInternalServerError)
		if p.managesTurn() {
			p.finalizeTurn(content, nil, http.StatusInternalServerError, true)
		}
		return
	}
	p.state.LastAssistantSummary = truncateSummary(content, 500)
	p.states.set(p.sessionID, p.state)
	writeChatResponse(p.w, body, http.StatusOK)
	if p.managesTurn() {
		p.finalizeTurn(content, body, http.StatusOK, false)
	}
	log.Printf("[openrouter] setup_reply session=%s turn_id=%s phase=%s",
		truncateSessionID(p.sessionID), truncateTurnID(p.turnID), p.state.ConversationPhase)
}

type chatRunParams struct {
	sessionID   string
	turnID      string
	turnRec     *chatTurnRecord
	turnRole    turnRole
	turns       *turnStore
	states      *agentStateStore
	state       *AgentSessionState
	req         chatCompletionRequest
	userMessage string
	apiKey      string
	r           *http.Request
	w           http.ResponseWriter
}

func (p *chatRunParams) managesTurn() bool {
	if p.turnID == "" || p.turnRec == nil {
		return false
	}
	return p.turnRole == turnRoleLeader || p.turnRole == turnRoleResumeLeader
}

func (p *chatRunParams) upstreamContext() context.Context {
	if p.managesTurn() && p.turnID != "" {
		return context.WithoutCancel(p.r.Context())
	}
	return p.r.Context()
}

func (p *chatRunParams) finalizeTurn(rawAssistant string, responseBody []byte, statusCode int, failed bool) {
	if !p.managesTurn() {
		return
	}
	visible := clientVisibleAssistantContentGuarded(rawAssistant, p.state)
	if failed {
		p.turns.failTurn(p.sessionID, p.turnID, rawAssistant, visible, responseBody, statusCode)
		log.Printf("[openrouter] turn_failed session=%s turn_id=%s visible_chars=%d",
			truncateSessionID(p.sessionID), truncateTurnID(p.turnID), len(visible))
		return
	}
	p.turns.completeTurn(p.sessionID, p.turnID, rawAssistant, visible, responseBody, statusCode)
	log.Printf("[openrouter] turn_completed session=%s turn_id=%s visible_chars=%d",
		truncateSessionID(p.sessionID), truncateTurnID(p.turnID), len(visible))
}

func (p *chatRunParams) displayRawAssistant(handoffParts []string, lastAssistant string) string {
	return buildDisplayAssistantRaw(handoffParts, lastAssistant)
}

// serverInstruction carries a corrective retry or phase handoff into the next model call's
// system prompt. It used to be appended to the conversation as a user message after the
// rejected reply, and the model took both for the student's words: it thanked the student
// for the note and assessed its own rejected draft as the student's answer.
type serverInstruction struct {
	kind          string // corrective_retry or mode
	note          string
	rejectedDraft string
}

func (s serverInstruction) systemBlock() string {
	if s.note == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("SERVER INSTRUCTION FOR THIS REPLY — from the server, not the student. The student has sent nothing new: their latest message is the last user message in the conversation.")
	if s.kind == "mode" {
		b.WriteString(" That message was already handled by the previous part of the interview; do not respond to or evaluate it.")
	}
	b.WriteString("\n")
	b.WriteString(s.note)
	if s.rejectedDraft != "" {
		b.WriteString("\nYour rejected draft, which the student never saw. It is not the student's words; do not thank anyone for it, assess it, or continue from it:\n<<<\n")
		b.WriteString(s.rejectedDraft)
		b.WriteString("\n>>>")
	}
	return b.String()
}

func (p *chatRunParams) tryScheduleFollowUpTurn(
	followUp assistantTurnFollowUp,
	assistant string,
	instruction *serverInstruction,
	modeContinuations *int,
	correctiveRetries *int,
) bool {
	if followUp.Kind == "server_results" || followUp.Kind == "fail_closed" {
		return false
	}
	if !followUp.ContinueTurn || followUp.Handoff == "" {
		return false
	}
	if followUp.Kind == "mode" || followUp.Kind == "results" {
		*modeContinuations++
	}
	if followUp.Kind == "corrective_retry" {
		*correctiveRetries++
		log.Printf("[openrouter] corrective_retry session=%s turn_id=%s active_mode=%s",
			truncateSessionID(p.sessionID), truncateTurnID(p.turnID), p.state.ActiveMode)
	}
	*instruction = serverInstruction{kind: followUp.Kind, note: followUp.Handoff}
	if followUp.Kind == "corrective_retry" {
		instruction.rejectedDraft = clientVisibleAssistantContent(assistant)
	}
	return true
}

func writeChatResponse(w http.ResponseWriter, body []byte, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
}

func runChatRequest(p chatRunParams) {
	chatStarted := time.Now()
	modeContinuations := 0
	correctiveRetries := 0

	// The student's conversation is sent unchanged on every internal call; retries and phase
	// handoffs travel in the system prompt (see serverInstruction).
	turnMessages := append([]chatMessage(nil), p.req.Messages...)
	var instruction serverInstruction
	var handoffParts []string
	var lastAssistant string
	responseStarted := false
	displayRaw := func() string {
		return p.displayRawAssistant(handoffParts, lastAssistant)
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[openrouter] chat_panic session=%s turn_id=%s panic=%v",
				truncateSessionID(p.sessionID), truncateTurnID(p.turnID), recovered)
			if p.managesTurn() && !responseStarted {
				p.finalizeTurn(displayRaw(), nil, http.StatusInternalServerError, true)
			}
		}
	}()

	for turn := 0; turn < maxChatInternalTurns; turn++ {
		// Use cached prompt if available; otherwise build it (for pre-assessment turns)
		prompt := p.state.CachedSystemPrompt
		if prompt == "" {
			var err error
			prompt, _, _, err = buildSystemPrompt(p.state)
			if err != nil {
				http.Error(p.w, "internal error", http.StatusInternalServerError)
				if p.managesTurn() {
					p.finalizeTurn(displayRaw(), nil, http.StatusInternalServerError, true)
				}
				return
			}
		}

		if block := instruction.systemBlock(); block != "" {
			prompt = block + "\n\n" + prompt
		}

		payload, err := json.Marshal(chatCompletionRequest{
			Model:    resolveChatModel(p.req.Model),
			Messages: prependSystemMessage(turnMessages, prompt),
		})
		if err != nil {
			http.Error(p.w, "internal error", http.StatusInternalServerError)
			if p.managesTurn() {
				p.finalizeTurn(displayRaw(), nil, http.StatusInternalServerError, true)
			}
			return
		}

		logCtx := newOpenRouterLogCtx(p.sessionID, p.turnID, "chat_body", turn)
		respBody, statusCode, err := readOpenRouterBodyWithRetry(p.upstreamContext(), p.apiKey, payload, logCtx)
		if err != nil {
			log.Printf("[openrouter] chat_body_failed session=%s turn_id=%s mode_turn=%d err=%q",
				truncateSessionID(p.sessionID), truncateTurnID(p.turnID), turn, err.Error())
			http.Error(p.w, "upstream error", http.StatusBadGateway)
			if p.managesTurn() {
				p.finalizeTurn(displayRaw(), nil, http.StatusBadGateway, true)
			}
			return
		}

		if statusCode != http.StatusOK {
			http.Error(p.w, string(respBody), statusCode)
			if p.managesTurn() {
				p.finalizeTurn(displayRaw(), respBody, statusCode, true)
			}
			return
		}

		assistant := extractAssistantContent(respBody)
		lastAssistant = assistant

		syncLog := &ipySyncLogContext{
			sessionID:  p.sessionID,
			turnID:     p.turnID,
			modeTurn:   turn,
			activeMode: p.state.ActiveMode,
			phase:      p.state.ConversationPhase,
		}
		followUp := postProcessAssistantTurnWithGuard(p.state, assistant, correctiveRetries >= maxCorrectiveRetries, syncLog)
		p.states.set(p.sessionID, p.state)

		if followUp.Kind == "server_results" || followUp.Kind == "fail_closed" {
			content := followUp.DirectAssistant
			if content == "" {
				content = buildAssessmentTurnFailureMessage()
			}
			if visible := clientVisibleAssistantContentGuarded(content, p.state); visible != "" {
				content = visible
			}
			strippedBody, err := replaceAssistantContent(respBody, content)
			if err != nil {
				strippedBody = respBody
			}
			writeChatResponse(p.w, strippedBody, statusCode)
			responseStarted = true
			if p.managesTurn() {
				p.finalizeTurn(content, strippedBody, statusCode, followUp.Kind == "fail_closed")
			}
			logChatRequestDone(p.sessionID, p.turnID, time.Since(chatStarted).Milliseconds(), modeContinuations)
			return
		}

		if followUp.ContinueTurn {
			if followUp.Kind == "mode" || followUp.Kind == "results" {
				// Show fixed text, not the model's phase-closing reply: that reply is where the
				// model grades the student, and its grading notes leaked into the display.
				handoffParts = append(handoffParts, phaseClosingMessage)
			}
		}

		if p.tryScheduleFollowUpTurn(followUp, assistant, &instruction, &modeContinuations, &correctiveRetries) {
			continue
		}

		strippedBody, err := replaceAssistantContent(respBody, stripIPyIntervuTail(displayRaw()))
		if err != nil {
			strippedBody = respBody
		}
		if visible := clientVisibleAssistantContentGuarded(displayRaw(), p.state); visible != "" {
			if rewritten, err := replaceAssistantContent(strippedBody, visible); err == nil {
				strippedBody = rewritten
			}
		}
		writeChatResponse(p.w, strippedBody, statusCode)
		responseStarted = true
		if p.managesTurn() {
			p.finalizeTurn(displayRaw(), strippedBody, statusCode, false)
		}
		logChatRequestDone(p.sessionID, p.turnID, time.Since(chatStarted).Milliseconds(), modeContinuations)
		return
	}

	if p.managesTurn() {
		p.finalizeTurn(displayRaw(), nil, http.StatusOK, false)
	}
	logChatRequestDone(p.sessionID, p.turnID, time.Since(chatStarted).Milliseconds(), modeContinuations)
}
