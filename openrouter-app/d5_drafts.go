package main

import (
	"log"
	"strings"
	"time"
)

// Phase 2: pre-drafted openings. When a part's opening is delivered, a background job
// drafts the next part's opening with reasoning on, which the live path cannot afford,
// so the transition can use it with no live call (D5-interviewer-evaluator-design.md §7).

const (
	d5DraftAttempts = 3
	// d5DraftWait is how long a transition waits for a draft still in flight.
	d5DraftWait = 2 * time.Second
)

// d5PreparedOpening is a pre-drafted opening for one part.
type d5PreparedOpening struct {
	// Content is the model's reply, including the hidden DEFECT line for Bug.
	Content string
	// Verified is the defect check's description of the actual bug (Bug only).
	Verified string
	Problems []string
	Attempts int
}

// d5NextMode is the part after mode, or "" when mode is the last one.
func d5NextMode(state *AgentSessionState, mode string) string {
	if state.isProblemDecompositionWeek() {
		return ""
	}
	switch mode {
	case modeConceptual:
		return modeCode
	case modeCode:
		return modeBug
	}
	return ""
}

// draftRequest is a non-streamed opening call with reasoning, for background drafting.
func draftRequest(model string, messages []chatMessage, maxTokens int) d5Request {
	return withReasoningBudget(d5Request{
		Model:     resolveChatModel(model),
		Messages:  messages,
		MaxTokens: maxTokens,
		Stop:      d5Stops,
		Usage:     map[string]any{"include": true},
	})
}

// d5StartDraft starts drafting mode's opening in the background. The prompt is built now,
// while the turn holds the session; the job itself only makes calls and runs checks, so it
// never reads session state the turn may be changing.
func d5StartDraft(p chatRunParams, sess *d5Session, mode string) {
	if mode == "" {
		return
	}
	state := p.state
	move := d5OpeningMove(state, sess, mode)
	system := d5InterviewerSystemPrompt(state, sess, mode, move, nil)
	base := d5InterviewerMessages(system, sess, mode, true)
	jobParams := chatRunParams{state: state, apiKey: p.apiKey, sessionID: p.sessionID, req: p.req}

	done := make(chan struct{})
	sess.mu.Lock()
	if sess.draftDone == nil {
		sess.draftDone = map[string]chan struct{}{}
		sess.Drafts = map[string]*d5PreparedOpening{}
	}
	sess.draftDone[mode] = done
	delete(sess.Drafts, mode)
	sess.mu.Unlock()

	go func() {
		defer close(done)
		started := time.Now()
		var best *d5PreparedOpening
		feedback := ""
		for attempt := 1; attempt <= d5DraftAttempts; attempt++ {
			msgs := append([]chatMessage(nil), base...)
			if feedback != "" {
				msgs[0].Content += "\n" + feedback
			}
			res, err := runBackgroundCall(jobParams.apiKey, jobParams.sessionID, draftRequest(jobParams.req.Model, msgs, move.MaxTokens), d5DraftTimeout, 1)
			if err != nil || strings.TrimSpace(res.Content) == "" {
				log.Printf("[d5] draft_attempt_failed session=%s mode=%s attempt=%d err=%v", truncateSessionID(jobParams.sessionID), mode, attempt, err)
				continue
			}
			problems, verified := d5OpeningProblems(jobParams, mode, res.Content, nil, true)
			candidate := &d5PreparedOpening{Content: res.Content, Verified: verified, Problems: problems, Attempts: attempt}
			if best == nil || len(problems) < len(best.Problems) {
				best = candidate
			}
			if len(problems) == 0 {
				break
			}
			feedback = strings.Join(problems, "\n")
		}
		problems := -1
		if best != nil {
			problems = len(best.Problems)
		}
		log.Printf("[d5] draft_done session=%s mode=%s ms=%d attempts=%d problems=%d",
			truncateSessionID(jobParams.sessionID), mode, time.Since(started).Milliseconds(), func() int {
				if best == nil {
					return d5DraftAttempts
				}
				return best.Attempts
			}(), problems)
		sess.mu.Lock()
		sess.Drafts[mode] = best
		sess.mu.Unlock()
	}()
}

// d5TakePrepared returns mode's pre-drafted opening when one passed every check, waiting
// up to d5DraftWait for a draft still in flight. It returns nil (use the live path) when
// there is no draft, it is late, or it still has problems.
func d5TakePrepared(p chatRunParams, sess *d5Session, mode string) *d5PreparedOpening {
	sess.mu.Lock()
	done := sess.draftDone[mode]
	sess.mu.Unlock()
	if done == nil {
		return nil
	}
	ready := true
	select {
	case <-done:
	case <-time.After(d5DraftWait):
		ready = false
	}
	sess.mu.Lock()
	draft := sess.Drafts[mode]
	delete(sess.Drafts, mode)
	delete(sess.draftDone, mode)
	sess.mu.Unlock()
	usable := ready && draft != nil && len(draft.Problems) == 0
	log.Printf("[d5] draft_used session=%s mode=%s ready=%v usable=%v", truncateSessionID(p.sessionID), mode, ready, usable)
	if !usable {
		return nil
	}
	return draft
}
