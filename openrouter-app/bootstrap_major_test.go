package main

import "testing"

// A late bootstrap completion (second tab sharing the session cookie, or a
// slow bootstrap call) must not send a session that already has a major back
// to AwaitingMajor.
func TestBootstrapAfterMajorDoesNotReaskMajor(t *testing.T) {
	state := newAgentSessionState()
	applyPreChatUserUpdate(state, "Biology")
	if state.StudentMajor != "Biology" || state.ConversationPhase != phaseAwaitingKeyConcept {
		t.Fatalf("setup: major=%q phase=%q", state.StudentMajor, state.ConversationPhase)
	}

	applyBootstrapState(state, "Welcome to IPyIntervu. What's your major?")

	if state.ConversationPhase == phaseAwaitingMajor {
		t.Fatalf("bootstrap reset phase to %q although major %q was already recorded", state.ConversationPhase, state.StudentMajor)
	}

	applyPreChatUserUpdate(state, "week 3")
	if state.ConversationPhase != phaseAssessmentInProgress {
		t.Fatalf("after week selection phase = %q, want %q (session is stuck asking for major)", state.ConversationPhase, phaseAssessmentInProgress)
	}
}
