package main

import (
	"strings"
	"testing"
)

func TestNormalizeAssessmentBucket(t *testing.T) {
	tests := map[string]string{
		"Looking Good":  bucketCompetent,
		"Not Yet Ready": bucketNotReady,
		"Looks Great":   bucketExceptional,
		"Competent":     bucketCompetent,
		"Strong":        bucketCompetent,
		"Good":          bucketCompetent,
	}
	for input, want := range tests {
		if got := normalizeAssessmentBucket(input); got != want {
			t.Fatalf("normalizeAssessmentBucket(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeAssessmentPhase(t *testing.T) {
	tests := map[string]string{
		"in_progress": assessmentPhaseInProgress,
		"in progress": assessmentPhaseInProgress,
		"complete":    assessmentPhaseComplete,
		"done":        assessmentPhaseComplete,
	}
	for input, want := range tests {
		if got := normalizeAssessmentPhase(input); got != want {
			t.Fatalf("normalizeAssessmentPhase(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseAssistantStateSyncConceptualBucketFromProseStrong(t *testing.T) {
	state := &AgentSessionState{ActiveMode: modeConceptual}
	assistant := "Thanks. Your conceptual assessment is Strong."

	parseAssistantStateSync(state, assistant)

	if state.ConceptualAssessmentBucket != bucketCompetent {
		t.Fatalf("ConceptualAssessmentBucket = %q, want %q", state.ConceptualAssessmentBucket, bucketCompetent)
	}
	if state.ConceptualAssessmentPhase != assessmentPhaseComplete {
		t.Fatalf("ConceptualAssessmentPhase = %q, want %q", state.ConceptualAssessmentPhase, assessmentPhaseComplete)
	}
}

func TestInProgressWithBucketDoesNotCompleteMode(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeConceptual,
		CurrentWeekNumber: 8,
	}
	assistant := "Next question.\n\n```_ipyintervu\n{\"conceptualAssessmentPhase\": \"in_progress\", \"conceptualAssessmentBucket\": \"Competent\"}\n```"

	parseAssistantStateSync(state, assistant)

	if state.ConceptualAssessmentBucket != "" {
		t.Fatalf("ConceptualAssessmentBucket = %q, want empty during in_progress", state.ConceptualAssessmentBucket)
	}
	if state.ConceptualAssessmentPhase != assessmentPhaseInProgress {
		t.Fatalf("ConceptualAssessmentPhase = %q, want %q", state.ConceptualAssessmentPhase, assessmentPhaseInProgress)
	}
	if applyAutomaticModeTransitions(state) {
		t.Fatal("expected no mode transition during in_progress")
	}
}

func TestCompletePhaseWithBucketTransitions(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeConceptual,
		CurrentWeekNumber: 8,
	}
	assistant := "Thanks.\n\n```_ipyintervu\n{\"conceptualAssessmentPhase\": \"complete\", \"conceptualAssessmentBucket\": \"Competent\"}\n```"

	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "mode" || !followUp.ContinueTurn {
		t.Fatalf("expected mode continuation, got %+v", followUp)
	}
	if state.ActiveMode != modeCode {
		t.Fatalf("ActiveMode = %q, want %q", state.ActiveMode, modeCode)
	}
}

func TestAssessmentSyncRetryOnMissingIPyBlock(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeConceptual,
		CurrentWeekNumber: 8,
	}
	assistant := "Thanks for walking through that menu flow."

	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "corrective_retry" || !followUp.ContinueTurn {
		t.Fatalf("expected corrective_retry follow-up, got %+v", followUp)
	}
}

func TestQuestionWithoutIPyBlockMarksOpeningServed(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeConceptual,
		CurrentWeekNumber: 8,
	}
	assistant := "What would you do if the user enters an invalid menu choice?"

	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "" || followUp.ContinueTurn {
		t.Fatalf("expected reply to be delivered, got %+v", followUp)
	}
	if !state.ModeOpeningServed {
		t.Fatal("opening without a sync block should still count as served")
	}
}

func TestPostProcessBucketSyncThenModeTransition(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeConceptual,
		CurrentWeekNumber: 8,
	}
	wrapUp := "Thanks. Let's move on to the coding portion."

	syncFollowUp := postProcessAssistantTurn(state, wrapUp, false, nil)
	if syncFollowUp.Kind != "corrective_retry" {
		t.Fatalf("expected corrective_retry for missing block, got %+v", syncFollowUp)
	}

	bucketReply := "Understood.\n\n```_ipyintervu\n{\"conceptualAssessmentPhase\": \"complete\", \"conceptualAssessmentBucket\": \"Competent\"}\n```"
	modeFollowUp := postProcessAssistantTurn(state, bucketReply, true, nil)
	if modeFollowUp.Kind != "mode" || !modeFollowUp.ContinueTurn {
		t.Fatalf("expected mode continuation, got %+v", modeFollowUp)
	}
	if state.ActiveMode != modeCode {
		t.Fatalf("ActiveMode = %q, want %q", state.ActiveMode, modeCode)
	}
}

func TestWeek1CompleteBucketTransitionsToResultsContinuation(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:  phaseAssessmentInProgress,
		ActiveMode:         modeConceptual,
		CurrentWeekNumber:  1,
		SelectedKeyConcept: "Week 1 - Problem Decomposition",
	}
	assistant := "Understood.\n\n```_ipyintervu\n{\"conceptualAssessmentPhase\": \"complete\", \"conceptualAssessmentBucket\": \"Competent\"}\n```"

	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "server_results" {
		t.Fatalf("expected server_results, got %+v", followUp)
	}
	if followUp.ContinueTurn {
		t.Fatal("server results should not schedule another model call")
	}
	if state.ConversationPhase != phaseAssessmentResults {
		t.Fatalf("ConversationPhase = %q, want %q", state.ConversationPhase, phaseAssessmentResults)
	}
	if state.FinalRating != bucketCompetent {
		t.Fatalf("FinalRating = %q, want %q", state.FinalRating, bucketCompetent)
	}
}

func TestPostProcessResultsContinuationAfterBug(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:          phaseAssessmentInProgress,
		ActiveMode:                 modeBug,
		CurrentWeekNumber:          7,
		SelectedKeyConcept:         "Week 7 - while Loops & Menus",
		ConceptualAssessmentBucket: bucketCompetent,
		CodeAssessmentBucket:       bucketCompetent,
	}
	assistant := "Thanks.\n\n```_ipyintervu\n{\"bugAssessmentPhase\": \"complete\", \"bugAssessmentBucket\": \"Competent\"}\n```"

	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "server_results" {
		t.Fatalf("expected server_results, got %+v", followUp)
	}
	if followUp.ContinueTurn {
		t.Fatal("server results should not schedule another model call")
	}
	if state.ConversationPhase != phaseAssessmentResults {
		t.Fatalf("ConversationPhase = %q, want %q", state.ConversationPhase, phaseAssessmentResults)
	}
}

// A wrap-up that keeps the phase in_progress leaves the student nothing to answer and the
// session stuck, so it is retried; the retry continues the interview (or closes it with a
// bucket) rather than rewinding to the scenario.
func TestWrapUpProseWithInProgressRetriesForward(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:            phaseAssessmentInProgress,
		ActiveMode:                   modeConceptual,
		CurrentWeekNumber:            1,
		ModeOpeningServed:            true,
		ModeUserAnsweredSinceOpening: true,
	}
	assistant := strings.Join([]string{
		"I think that covers our decomposition well. Let me transition us toward the next part of the assessment.",
		"",
		"```_ipyintervu",
		"{\"conceptualAssessmentPhase\": \"in_progress\"}",
		"```",
	}, "\n")

	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "corrective_retry" || strings.Contains(followUp.Handoff, "Re-present the SAME concrete scenario") {
		t.Fatalf("question-less wrap-up should retry forward, got %+v", followUp)
	}
	if state.ConversationPhase != phaseAssessmentInProgress {
		t.Fatalf("ConversationPhase = %q, want %q", state.ConversationPhase, phaseAssessmentInProgress)
	}
}

func TestParseAssistantStateSyncConceptualBucketFromProse(t *testing.T) {
	state := &AgentSessionState{ActiveMode: modeConceptual}
	assistant := "Thanks for walking through those concepts. Your conceptual assessment is Competent."

	parseAssistantStateSync(state, assistant)

	if state.ConceptualAssessmentBucket != bucketCompetent {
		t.Fatalf("ConceptualAssessmentBucket = %q, want %q", state.ConceptualAssessmentBucket, bucketCompetent)
	}
}

func TestParseAssistantStateSyncPartialIPyJSONBucket(t *testing.T) {
	state := &AgentSessionState{ActiveMode: modeConceptual}
	assistant := "I think we have a solid picture.\n\n```_ipy\n{\"conceptualAssessmentPhase\": \"complete\", \"conceptualAssessmentBucket\": \"Competent\"}"

	parseAssistantStateSync(state, assistant)

	if state.ConceptualAssessmentBucket != bucketCompetent {
		t.Fatalf("ConceptualAssessmentBucket = %q, want %q", state.ConceptualAssessmentBucket, bucketCompetent)
	}
}

func TestApplyPostChatStateUpdatePartialIPyTriggersCodeMode(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeConceptual,
		CurrentWeekNumber: 8,
	}
	assistant := "Let me wrap up this portion.\n\n```_ipy\n{\"conceptualAssessmentPhase\": \"complete\", \"conceptualAssessmentBucket\": \"Competent\"}"

	if !applyPostChatStateUpdate(state, "student answer", assistant) {
		t.Fatal("expected automatic continuation into code mode")
	}
	if state.ActiveMode != modeCode {
		t.Fatalf("ActiveMode = %q, want %q", state.ActiveMode, modeCode)
	}
}

func TestApplyAutomaticModeTransitionsConceptualToCode(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:          phaseAssessmentInProgress,
		ActiveMode:                 modeConceptual,
		CurrentWeekNumber:          8,
		ConceptualAssessmentBucket: bucketCompetent,
		ConceptualAssessmentPhase:  assessmentPhaseComplete,
	}

	if !applyAutomaticModeTransitions(state) {
		t.Fatal("expected continuation after conceptual bucket assignment")
	}
	if state.ActiveMode != modeCode {
		t.Fatalf("ActiveMode = %q, want %q", state.ActiveMode, modeCode)
	}
	if state.PendingQuestion != "codeAssessment" {
		t.Fatalf("PendingQuestion = %q, want codeAssessment", state.PendingQuestion)
	}
}

func TestApplyAutomaticModeTransitionsInProgressDoesNotTransition(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:          phaseAssessmentInProgress,
		ActiveMode:                 modeConceptual,
		CurrentWeekNumber:          8,
		ConceptualAssessmentBucket: bucketCompetent,
		ConceptualAssessmentPhase:  assessmentPhaseInProgress,
	}

	if applyAutomaticModeTransitions(state) {
		t.Fatal("expected no transition while phase is in_progress")
	}
}

func TestCoachingEnteredBeforeResultsPersistsThroughFinalResults(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:            phaseAssessmentInProgress,
		ActiveMode:                   modeBug,
		CurrentWeekNumber:            8,
		ConceptualAssessmentBucket:   bucketCompetent,
		CodeAssessmentBucket:         bucketCompetent,
		BugAssessmentBucket:          bucketCompetent,
		BugAssessmentPhase:           assessmentPhaseComplete,
		CoachingEnteredBeforeResults: true,
	}

	if applyAutomaticModeTransitions(state) {
		t.Fatal("expected no continuation after final bug bucket assignment")
	}
	if state.ConversationPhase != phaseAssessmentResults {
		t.Fatalf("ConversationPhase = %q, want %q", state.ConversationPhase, phaseAssessmentResults)
	}
	if !state.CoachingEnteredBeforeResults {
		t.Fatal("CoachingEnteredBeforeResults should remain true for the session once set")
	}
}

func TestModeContinuationUserMessage(t *testing.T) {
	state := &AgentSessionState{ActiveMode: modeCode}
	msg := modeContinuationUserMessage(state)
	if msg == "" {
		t.Fatal("expected handoff message for code mode")
	}
}

func TestClampAssessmentModeForwardAfterConceptualComplete(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:          phaseAssessmentInProgress,
		ActiveMode:                 modeConceptual,
		CurrentWeekNumber:          8,
		ConceptualAssessmentBucket: bucketCompetent,
		ConceptualAssessmentPhase:  assessmentPhaseComplete,
		ModesCompleted:             []string{modeConceptual},
	}

	clampAssessmentModeForward(state)

	if state.ActiveMode != modeCode {
		t.Fatalf("ActiveMode = %q, want %q", state.ActiveMode, modeCode)
	}
	if state.PendingQuestion != "codeAssessment" {
		t.Fatalf("PendingQuestion = %q, want codeAssessment", state.PendingQuestion)
	}
}

func TestResumeAssessmentAfterCoaching(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:          phaseAssessmentInProgress,
		ActiveMode:                 modeCoaching,
		CoachingRequested:          true,
		CurrentWeekNumber:          8,
		ConceptualAssessmentBucket: bucketCompetent,
		ConceptualAssessmentPhase:  assessmentPhaseComplete,
		ModesCompleted:             []string{modeConceptual},
	}

	applyPreChatUserUpdate(state, "Let's continue with the coding part.")

	if state.ActiveMode != modeCode {
		t.Fatalf("ActiveMode = %q, want %q after leaving coaching", state.ActiveMode, modeCode)
	}
}

func TestCompletedModeInProgressSyncIgnored(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:          phaseAssessmentInProgress,
		ActiveMode:                 modeCode,
		CurrentWeekNumber:          8,
		ConceptualAssessmentBucket: bucketCompetent,
		ConceptualAssessmentPhase:  assessmentPhaseComplete,
		ModesCompleted:             []string{modeConceptual},
	}

	applyModeSyncFromTail(state, modeConceptual, assessmentPhaseInProgress, bucketCompetent)

	if state.ConceptualAssessmentPhase != assessmentPhaseComplete {
		t.Fatalf("ConceptualAssessmentPhase = %q, want complete (unchanged)", state.ConceptualAssessmentPhase)
	}
}

func TestSelectPromptFilesCoachingDoesNotSuppressCodeMode(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeCode,
		CoachingRequested: true,
		CurrentWeekNumber: 8,
	}

	files, bundleID := selectPromptFiles(state)

	hasCode := false
	hasCoaching := false
	for _, f := range files {
		if f.displayName == "IPyIntervu-modes-code.md" {
			hasCode = true
		}
		if f.displayName == "IPyIntervu-modes-coaching.md" {
			hasCoaching = true
		}
	}
	if !hasCode {
		t.Fatal("expected code mode instructions when activeMode is Code")
	}
	if hasCoaching {
		t.Fatal("coaching instructions should not load when activeMode is Code")
	}
	if !strings.Contains(bundleID, "mode-code") {
		t.Fatalf("bundleID = %q, want mode-code", bundleID)
	}
}

func TestSelectPromptFilesCoachingModeLoadsCoachingOnly(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeCoaching,
		CoachingRequested: true,
		CurrentWeekNumber: 8,
	}

	files, bundleID := selectPromptFiles(state)

	hasCoaching := false
	hasCode := false
	for _, f := range files {
		if f.displayName == "IPyIntervu-modes-coaching.md" {
			hasCoaching = true
		}
		if f.displayName == "IPyIntervu-modes-code.md" {
			hasCode = true
		}
	}
	if !hasCoaching {
		t.Fatal("expected coaching instructions when activeMode is Coaching")
	}
	if hasCode {
		t.Fatal("code mode instructions should not load during coaching")
	}
	if !strings.Contains(bundleID, "coaching") {
		t.Fatalf("bundleID = %q, want coaching", bundleID)
	}
}

func TestCodeModeIntroDoesNotForceAssessmentSync(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeCode,
		CurrentWeekNumber: 8,
	}
	assistant := "Hi, I'm Taylor, a software developer at ChemCore Diagnostics. Morgan and I will run the code problem portion of today's interview.\n\nBefore you write any code, break this problem into smaller steps and explain your approach.\n\n```_ipyintervu\n{\"codeAssessmentPhase\": \"in_progress\"}\n```"

	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind == "corrective_retry" {
		t.Fatalf("code mode intro should not force corrective retry, got %+v", followUp)
	}
	if state.CodeAssessmentPhase != assessmentPhaseInProgress {
		t.Fatalf("CodeAssessmentPhase = %q, want in_progress", state.CodeAssessmentPhase)
	}
}

// Announcing the bug portion while the code phase is still in_progress asks nothing and
// cannot advance, so it is retried forward.
func TestCodeModePrematureBugTransitionRetriesForward(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:            phaseAssessmentInProgress,
		ActiveMode:                   modeCode,
		CurrentWeekNumber:            8,
		ModeOpeningServed:            true,
		ModeUserAnsweredSinceOpening: true,
		ModeInterviewStep:            interviewStepCodeSubmitted,
	}
	assistant := "Thanks for that. Let's move on to the debugging portion now.\n\n```_ipyintervu\n{\"codeAssessmentPhase\": \"in_progress\"}\n```"

	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "corrective_retry" || !strings.Contains(followUp.Handoff, "explain-code") {
		t.Fatalf("premature transition should retry with code-phase guidance, got %+v", followUp)
	}
}
