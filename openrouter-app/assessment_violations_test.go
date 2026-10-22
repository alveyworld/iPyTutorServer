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

func TestDetectAssessmentViolationsMissingSync(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeConceptual,
		CurrentWeekNumber: 8,
	}
	assistant := "Thanks for walking through that menu flow."

	v := detectAssessmentViolations(state, assistant)
	if !v.MissingSync {
		t.Fatal("expected missing sync violation")
	}
	if !v.NeedsCorrectiveRetry() {
		t.Fatal("expected corrective retry for missing sync")
	}
}

func TestDetectAssessmentViolationsQuestionWithoutSyncIsImplicitInProgress(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeConceptual,
		CurrentWeekNumber: 8,
	}
	assistant := "What would you do if the user enters an invalid menu choice?"

	if v := detectAssessmentViolations(state, assistant); v.Any() {
		t.Fatalf("question without sync block should not be a violation, got %+v", v)
	}
}

func TestDetectAssessmentViolationsContentWithSyncTruncateOnly(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeConceptual,
		CurrentWeekNumber: 9,
	}
	assistant := "What is append? Append adds a single element to the end of a list.\n\n```_ipyintervu\n{\"conceptualAssessmentPhase\": \"in_progress\"}\n```"

	v := detectAssessmentViolations(state, assistant)
	if !v.Content {
		t.Fatal("expected content violation")
	}
	if v.MissingSync {
		t.Fatal("sync block is present")
	}
	if !v.NeedsTruncateOnly() {
		t.Fatal("expected truncate-only handling")
	}
	if v.NeedsCorrectiveRetry() {
		t.Fatal("content with valid sync should not retry")
	}
}

func TestPostProcessUnifiedCorrectiveRetry(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeConceptual,
		CurrentWeekNumber: 8,
	}
	assistant := "Thanks for walking through that menu flow."

	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "corrective_retry" || !followUp.ContinueTurn {
		t.Fatalf("expected corrective_retry, got %+v", followUp)
	}
	if !strings.Contains(followUp.Handoff, "_ipyintervu") {
		t.Fatalf("expected sync requirement in handoff, got %q", followUp.Handoff)
	}
}

func TestPostProcessContentWithSyncNoRetry(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeConceptual,
		CurrentWeekNumber: 9,
	}
	assistant := "What is append? Append adds a single element to the end of a list.\n\n```_ipyintervu\n{\"conceptualAssessmentPhase\": \"in_progress\"}\n```"

	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.ContinueTurn {
		t.Fatalf("expected no retry for truncate-only content issue, got %+v", followUp)
	}
}

func TestPostProcessSevereCompositeWithSyncCorrectiveRetry(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeConceptual,
		CurrentWeekNumber: 2,
	}
	assistant := strings.Join([]string{
		"Got it. At QuantCore, we often work with exact integer results. If you had meters = 7 and pieces = 2, what would the expression parts = meters // pieces produce, and what data type would it be?",
		"the type would be integer. The value would be 3.",
		"Got it. After that runs, y would still be 7 because Python evaluated y = x + 2 using the value of x at that moment (5), and reassigning x later does not retroactively update y.",
		"",
		"Now, at QuantCore we often work with area calculations. If a rectangle has width = 4 and height = 7, what expression would you write to compute its area, and what data type would the result be?",
		"",
		"```_ipyintervu",
		"{\"conceptualAssessmentPhase\": \"in_progress\"}",
		"```",
	}, "\n")

	v := detectAssessmentViolations(state, assistant)
	if !v.SevereContent {
		t.Fatal("expected severe composite violation")
	}
	if !v.NeedsCorrectiveRetry() {
		t.Fatal("expected corrective retry for severe composite even with sync")
	}

	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "corrective_retry" || !followUp.ContinueTurn {
		t.Fatalf("expected corrective_retry, got %+v", followUp)
	}
}

func TestPostProcessSimulatedStudentLineCorrectiveRetry(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeConceptual,
		CurrentWeekNumber: 1,
	}
	assistant := strings.Join([]string{
		"What would you consider the final output of this process to be?",
		"a table where each row was a region and an amount",
		"Good point — how might you handle invalid region codes?",
	}, "\n")

	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "corrective_retry" || !followUp.ContinueTurn {
		t.Fatalf("expected corrective_retry, got %+v", followUp)
	}
}

func TestPostProcessDeliversRetryReplyWithResidualViolation(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeConceptual,
		CurrentWeekNumber: 8,
	}
	assistant := "Thanks for walking through that menu flow."

	followUp := postProcessAssistantTurn(state, assistant, true, nil)
	if followUp.Kind != "" || followUp.ContinueTurn {
		t.Fatalf("expected retry reply to be delivered, got %+v", followUp)
	}
	if !state.ModeOpeningServed {
		t.Fatal("delivered reply should mark the mode opening as served")
	}
}

func TestPostProcessFailClosedAfterCorrectiveRetry(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeConceptual,
		CurrentWeekNumber: 8,
	}
	assistant := "  \n"

	followUp := postProcessAssistantTurn(state, assistant, true, nil)
	if followUp.Kind != "fail_closed" {
		t.Fatalf("expected fail_closed after corrective retry, got %+v", followUp)
	}
	if followUp.DirectAssistant == "" {
		t.Fatal("expected direct failure message")
	}
}

func TestWeek1CompleteBucketUsesServerResults(t *testing.T) {
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
		t.Fatal("server results should not continue model turn")
	}
	if !strings.Contains(followUp.DirectAssistant, "Overall Rating: Competent") {
		t.Fatalf("expected server-rendered results, got %q", followUp.DirectAssistant)
	}
	if state.ConversationPhase != phaseAssessmentResults {
		t.Fatalf("ConversationPhase = %q, want %q", state.ConversationPhase, phaseAssessmentResults)
	}
}

func TestBuildServerAssessmentResultsMessageWeek1(t *testing.T) {
	state := &AgentSessionState{
		SelectedKeyConcept:         "Week 1 - Problem Decomposition",
		ConceptualAssessmentBucket: bucketCompetent,
		FinalRating:                bucketCompetent,
	}
	state.CurrentWeekNumber = 1

	got := buildServerAssessmentResultsMessage(state)
	if !strings.Contains(got, "Code Assessment: N/A") {
		t.Fatalf("expected N/A code bucket, got %q", got)
	}
	if !strings.Contains(got, "Overall Rating: Competent") {
		t.Fatalf("expected final rating, got %q", got)
	}
}

func week1FollowUpState() *AgentSessionState {
	return &AgentSessionState{
		ConversationPhase:            phaseAssessmentInProgress,
		ActiveMode:                   modeConceptual,
		CurrentWeekNumber:            1,
		SelectedKeyConcept:           "Week 1 - Problem Decomposition",
		ConceptualAssessmentPhase:    assessmentPhaseInProgress,
		ModeOpeningServed:            true,
		ModeUserAnsweredSinceOpening: true,
		ModeInterviewStep:            interviewStepInterviewing,
	}
}

func TestWeek1ClosingWithoutSyncRetriesForCompleteNotNewScenario(t *testing.T) {
	state := week1FollowUpState()
	assistant := "Thanks. That covers how you would break this task down."

	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "corrective_retry" {
		t.Fatalf("expected corrective_retry, got %+v", followUp)
	}
	if !strings.Contains(followUp.Handoff, "no new scenario") || !strings.Contains(followUp.Handoff, "\"conceptualAssessmentPhase\": \"complete\"") {
		t.Fatalf("expected closing handoff asking for complete, got %q", followUp.Handoff)
	}
	if strings.Contains(followUp.Handoff, "new conceptual interview question") {
		t.Fatalf("closing handoff must not push a new question, got %q", followUp.Handoff)
	}
}

func TestWeek1FollowUpQuestionWithoutSyncIsDelivered(t *testing.T) {
	state := week1FollowUpState()
	assistant := "Got it. What steps would you put in the process?"

	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "" || followUp.ContinueTurn {
		t.Fatalf("expected reply to be delivered, got %+v", followUp)
	}
}

func TestWeek1ForwardMissingSyncHandoffUsesSameScenarioGuidance(t *testing.T) {
	state := week1FollowUpState()

	handoff := buildUnifiedCorrectiveHandoff(state, assessmentViolations{MissingSync: true})
	if !strings.Contains(handoff, "never present a new scenario") {
		t.Fatalf("expected week 1 same-scenario guidance, got %q", handoff)
	}
}

// Replays a reported session: the student answered the input and process questions, then
// a two-persona reply stacking questions was retried with "re-present the scenario" and the
// interview restarted with a new scenario.
func TestWeek1StackedFollowUpRetryDoesNotRestartScenario(t *testing.T) {
	state := week1FollowUpState()
	state.StudentMajor = "chemistry"
	assistant := strings.Join([]string{
		"I'm Julia Ramirez, a lab specialist at ChemCore Diagnostics.",
		"",
		"Which of those steps would you do first if the acetate had to be weighed? And what would you consider the output of this process?",
		"",
		"```_ipyintervu",
		"{\"conceptualAssessmentPhase\": \"in_progress\"}",
		"```",
	}, "\n")

	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "corrective_retry" {
		t.Fatalf("expected corrective_retry for stacked questions, got %+v", followUp)
	}
	if strings.Contains(followUp.Handoff, "Re-present the SAME concrete scenario") {
		t.Fatalf("follow-up retry must not rewind to the scenario, got %q", followUp.Handoff)
	}
	for _, want := range []string{"Do NOT re-present the opening scenario", "never present a new scenario", correctiveNoteRule} {
		if !strings.Contains(followUp.Handoff, want) {
			t.Errorf("handoff missing %q: %q", want, followUp.Handoff)
		}
	}
}

func TestWeek1CompleteInUnlabeledFenceShowsResults(t *testing.T) {
	state := week1FollowUpState()
	assistant := "Thanks.\n\n```json\n{\"conceptualAssessmentPhase\": \"complete\", \"conceptualAssessmentBucket\": \"Not Ready Yet\"}\n```"

	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "server_results" {
		t.Fatalf("expected server_results, got %+v", followUp)
	}
	if state.FinalRating != bucketNotReady {
		t.Fatalf("FinalRating = %q, want %q", state.FinalRating, bucketNotReady)
	}
}

func TestWeek1CompletedInMalformedJSONShowsResults(t *testing.T) {
	state := week1FollowUpState()
	assistant := "Thanks.\n\n```_ipyintervu\n{\"conceptualAssessmentBucket\": \"Exceptional\", \"conceptualAssessmentPhase\": \"completed\",}\n```"

	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "server_results" {
		t.Fatalf("expected server_results, got %+v", followUp)
	}
	if state.FinalRating != bucketExceptional {
		t.Fatalf("FinalRating = %q, want %q", state.FinalRating, bucketExceptional)
	}
}

// Replays a reported code-mode reply: after the student answered "it ends up being 90.0",
// the model explained the answer with invented values, graded it, and staged another turn
// ("Got it — that's exactly right.") before its only question.
func TestExplainedAndGradedReplyTriggersForwardRetry(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:            phaseAssessmentInProgress,
		ActiveMode:                   modeCode,
		CurrentWeekNumber:            2,
		CodeAssessmentPhase:          assessmentPhaseInProgress,
		ModeOpeningServed:            true,
		ModeUserAnsweredSinceOpening: true,
		ModeInterviewStep:            interviewStepCodeSubmitted,
		LastUserMessageRaw:           "it ends up being 90.0",
	}
	assistant := strings.Join([]string{
		"You're right that the computation runs, but I want to dig a little deeper. Let's step through it: we start with length = 10 and width = 9, then we compute area = length * width using those initial values. After that, we reassign length = 12 — so now length holds 12. Since area was already computed with the old values (10 and 9), the variable area still holds the result from that earlier calculation, which is 90.0 (a float, since the multiplication produced a decimal value).",
		"",
		"Got it — that's exactly right. Now let's look at the code line by line. Point me to the line you'd change if you wanted area to reflect the updated values of length = 12 and width = 9, and what would happen after that single change?",
		"",
		"```_ipyintervu",
		`{"codeAssessmentPhase": "in_progress"}`,
		"```",
	}, "\n")

	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "corrective_retry" {
		t.Fatalf("expected corrective_retry, got %+v", followUp)
	}
	if strings.Contains(followUp.Handoff, "Re-present the SAME concrete scenario") {
		t.Fatalf("follow-up retry must move forward, got %q", followUp.Handoff)
	}
	if !strings.Contains(followUp.Handoff, "do NOT praise") {
		t.Fatalf("retry should forbid praise and explanation, got %q", followUp.Handoff)
	}
}

func TestLooksLikeEvaluationOrStagedTurn(t *testing.T) {
	cases := map[string]bool{
		"Got it — that's exactly right. What would you change next?":                                true,
		"You're right that it runs. What happens to area?":                                          true,
		"Let's step through it: area is 90.0. Which line would you change?":                         true,
		"The scenario is set.\n\nGot it. Which line would you change?":                              true,
		"Exactly. Which line would you change?":                                                     true,
		"Got it. What happens to area when length * width runs with those two float values?":        false,
		"Thanks. Please paste your Python code.":                                                    false,
		"I'm Alex at ChemCore.\n\nHere's the scenario: we track sample weights. What is the input?": false,
		"Our goal is to get the right result for every sample. What steps belong in the process?":   false,
	}
	for text, want := range cases {
		if got := looksLikeEvaluationOrStagedTurn(text); got != want {
			t.Errorf("looksLikeEvaluationOrStagedTurn(%q) = %v, want %v", text, got, want)
		}
	}
}

const week2ExplanationReply = "Got it. A variable is like a named box that holds a value. For example, price = 10 and quantity = 3. You can then write total = price * quantity, and the expression price * quantity uses the variables to compute a new value.\n\n```_ipyintervu\n{\"conceptualAssessmentPhase\": \"in_progress\"}\n```"

// Replays a reported Week 2 first reply: an explanation with examples and no question.
func TestExplanationWithoutQuestionRedoesOpening(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:  phaseAssessmentInProgress,
		ActiveMode:         modeConceptual,
		CurrentWeekNumber:  2,
		SelectedKeyConcept: "Week 2 - Variables & Expressions",
	}

	v := detectAssessmentViolations(state, week2ExplanationReply)
	if !v.NoQuestion || !v.SevereContent {
		t.Fatalf("expected no-question and explanation violations, got %+v", v)
	}
	followUp := postProcessAssistantTurn(state, week2ExplanationReply, false, nil)
	if followUp.Kind != "corrective_retry" || !strings.Contains(followUp.Handoff, "did not open this part of the interview") {
		t.Fatalf("expected opening redo, got %+v", followUp)
	}
	if state.ModeOpeningServed {
		t.Fatal("a rejected reply must not count as the phase's first reply")
	}
}

// The first retry can break the rules too (it did in the report); the server now makes a
// second corrective call before showing anything, and only then delivers.
func TestSecondCorrectiveRetryBeforeDelivery(t *testing.T) {
	var calls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		content := week2ExplanationReply
		if n == 3 {
			content = "I'm Alex, a data analyst at Northwind Traders. An order has a price of 4.50 and a quantity of 3. What expression would you write to compute the order total?\n\n```_ipyintervu\n{\"conceptualAssessmentPhase\": \"in_progress\"}\n```"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": content}}},
		})
	}))
	defer upstream.Close()
	saved := openRouterURL
	openRouterURL = upstream.URL
	defer func() { openRouterURL = saved }()

	states := newAgentStateStore()
	state := newAgentSessionState()
	state.ConversationPhase = phaseAwaitingKeyConcept
	state.StudentMajor = "computer science"
	states.set("session-1", state)

	body, _ := json.Marshal(chatCompletionRequest{Messages: []chatMessage{{Role: "user", Content: "week 2"}}})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(string(body)))
	req.Header.Set(turnIDHeader, "turn-1")
	req = req.WithContext(context.WithValue(req.Context(), sessionIDContextKey, "session-1"))
	rec := httptest.NewRecorder()
	handleChat("test-key", states, newTurnStore())(rec, req)

	if n := atomic.LoadInt32(&calls); n != 1+maxCorrectiveRetries {
		t.Fatalf("expected %d model calls, got %d", 1+maxCorrectiveRetries, n)
	}
	if got := extractAssistantContent(rec.Body.Bytes()); !strings.HasSuffix(got, "compute the order total?") {
		t.Fatalf("expected the valid second retry to be shown, got %q", got)
	}
}

func TestAsksStudentSomething(t *testing.T) {
	cases := map[string]bool{
		"What expression would you write?": true,
		"Please paste your Python code.":   true,
		"Before you write any code, break this problem into smaller steps and explain your approach.": true,
		"Walk me through how you would find the bug.":                                                 true,
		"A variable is like a named box that holds a value.":                                          false,
		"Let's move on to the debugging portion now.":                                                 false,
	}
	for text, want := range cases {
		if got := asksStudentSomething(text); got != want {
			t.Errorf("asksStudentSomething(%q) = %v, want %v", text, got, want)
		}
	}
}

// Replays a reported Week 2 first reply: it asked a persona to begin instead of asking the
// student anything, and left "[companyName]" unfilled.
func TestPersonaHandoffWithPlaceholderRedoesOpening(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:  phaseAssessmentInProgress,
		ActiveMode:         modeConceptual,
		CurrentWeekNumber:  2,
		SelectedKeyConcept: "Week 2 - Variables & Expressions",
	}
	assistant := "I appreciate you confirming we're starting with Week 2: Variables & Expressions. Let me bring in my colleague Alex to begin the interview. Alex is part of the engineering team here at [companyName], and they'll walk us through today's scenario.\n\nAlex, would you like to begin?\n\n```_ipyintervu\n{\"conceptualAssessmentPhase\": \"in_progress\"}\n```"

	v := detectAssessmentViolations(state, assistant)
	if !v.NoQuestion || !v.UnfilledPlaceholder {
		t.Fatalf("expected no-question and placeholder violations, got %+v", v)
	}
	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "corrective_retry" || !strings.Contains(followUp.Handoff, "never to another interviewer") || !strings.Contains(followUp.Handoff, "[companyName]") {
		t.Fatalf("expected opening redo naming both problems, got %+v", followUp)
	}
}

func TestPersonaAddressedQuestionsDoNotCount(t *testing.T) {
	cases := map[string]bool{
		"Alex, would you like to begin?":                                false,
		"Would you like to take it from here, Julia?":                   false,
		"So, Taylor — can you share the task?":                          false,
		"Alex here. What would you identify as the input?":              true,
		"I work with Julia. What expression would you write for total?": true,
	}
	for text, want := range cases {
		if got := asksStudentSomething(text); got != want {
			t.Errorf("asksStudentSomething(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestHasUnfilledPlaceholder(t *testing.T) {
	cases := map[string]bool{
		"I work at [companyName].":                                             true,
		"Welcome to [Company Name] labs.":                                      true,
		"See the [style guide](https://example.com) for details.":              false,
		"What does `items[idx]` hold?":                                         false,
		"```python\nprices = [3, 4]\nfirst = prices[idx]\n```\nWhat is first?": false,
		"I work at Northwind Traders.":                                         false,
	}
	for text, want := range cases {
		if got := hasUnfilledPlaceholder(text); got != want {
			t.Errorf("hasUnfilledPlaceholder(%q) = %v, want %v", text, got, want)
		}
	}
}

// Replays a reported Week 2 phase change: the conceptual closing reply carried the model's
// grading notes, and the first code reply greeted "Taylor" as if the student were Taylor.
func TestPhaseChangeHidesGradingNotesAndRetriesPersonaGreeting(t *testing.T) {
	replies := []string{
		"The student answered the final follow-up question.\n\nLooking at the rubric:\n\nThis aligns with Competent.\n\nI'll now close the conceptual portion with a brief neutral closing sentence and the sync block.That covers everything we needed for this scenario.\n\n```_ipyintervu\n{\"conceptualAssessmentPhase\": \"complete\", \"conceptualAssessmentBucket\": \"Competent\"}\n```",
		"Hey Taylor! Morgan here — I’m on the engineering side with you today.\n\nThe marketing team at Sphere Analytics stores weekly visitor totals in variables. What’s the first thing you’d want to store or figure out?\n\n```_ipyintervu\n{\"codeAssessmentPhase\": \"in_progress\"}\n```",
		"Hi, I'm Taylor, a software developer at Sphere Analytics. The marketing team stores the weekly visitor total in a variable. How would you break the task of computing the average daily visitors into steps?\n\n```_ipyintervu\n{\"codeAssessmentPhase\": \"in_progress\"}\n```",
	}
	var calls int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": replies[min(int(n), len(replies))-1]}}},
		})
	}))
	defer upstream.Close()
	saved := openRouterURL
	openRouterURL = upstream.URL
	defer func() { openRouterURL = saved }()

	states := newAgentStateStore()
	state := &AgentSessionState{
		ConversationPhase:            phaseAssessmentInProgress,
		ActiveMode:                   modeConceptual,
		CurrentWeekNumber:            2,
		SelectedKeyConcept:           "Week 2 - Variables & Expressions",
		StudentMajor:                 "computer science",
		ConceptualAssessmentPhase:    assessmentPhaseInProgress,
		ModeOpeningServed:            true,
		ModeUserAnsweredSinceOpening: true,
	}
	states.set("session-1", state)

	body, _ := json.Marshal(chatCompletionRequest{Messages: []chatMessage{{Role: "user", Content: `I would use print and put the variables in the string, like "pay for <name>: <total>"`}}})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(string(body)))
	req.Header.Set(turnIDHeader, "turn-1")
	req = req.WithContext(context.WithValue(req.Context(), sessionIDContextKey, "session-1"))
	rec := httptest.NewRecorder()
	handleChat("test-key", states, newTurnStore())(rec, req)

	got := extractAssistantContent(rec.Body.Bytes())
	for _, leaked := range []string{"The student", "rubric", "sync block", "Hey Taylor"} {
		if strings.Contains(got, leaked) {
			t.Errorf("shown reply contains %q: %q", leaked, got)
		}
	}
	if !strings.HasPrefix(got, phaseClosingMessage) || !strings.Contains(got, "Hi, I'm Taylor") {
		t.Fatalf("expected fixed closing line then the clean code intro, got %q", got)
	}
	if state.ConceptualAssessmentBucket != bucketCompetent || state.ActiveMode != modeCode {
		t.Fatalf("rating must still be recorded from the hidden closing reply: bucket=%q mode=%q", state.ConceptualAssessmentBucket, state.ActiveMode)
	}
}

func TestAddressesPersonaAndLeaksReasoning(t *testing.T) {
	persona := map[string]bool{
		"Hey Taylor! Morgan here. What would you store first?":              true,
		"Thanks, Alex. What would you store first?":                         true,
		"Alex, would you like to begin?":                                    true,
		"Hi, I'm Taylor, a software developer. What would you store first?": false,
		"Morgan here — I work with Taylor. What would you store first?":     false,
		"Thanks. What would you store first?":                               false,
	}
	for text, want := range persona {
		if got := addressesPersona(text); got != want {
			t.Errorf("addressesPersona(%q) = %v, want %v", text, got, want)
		}
	}
	leaks := map[string]bool{
		"The student answered the final follow-up question.": true,
		"Looking at the rubric, this is Competent.":          true,
		"I'll now close the conceptual portion.":             true,
		"What would you do if a student record was missing?": false,
		"Which value would you store first?":                 false,
	}
	for text, want := range leaks {
		if got := leaksReasoning(text); got != want {
			t.Errorf("leaksReasoning(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestConfirmedLinesUpIsPraise(t *testing.T) {
	if !looksLikeEvaluationOrStagedTurn("Confirmed — that decomposition lines up with the prompt.\n\nPlease paste your Python code for the task.") {
		t.Fatal("\"Confirmed — … lines up with\" grades the answer and should be caught")
	}
}

func TestSpeaksAsInstructor(t *testing.T) {
	chemistry := &AgentSessionState{StudentMajor: "chemistry"}
	education := &AgentSessionState{StudentMajor: "Elementary Education"}
	cases := []struct {
		state *AgentSessionState
		text  string
		want  bool
	}{
		// From a reported code-phase reply.
		{chemistry, "Let’s talk about the task itself: we need to compute a weekly average, but we’ll only use what we know from week 1 and 2 — variables, expressions, data types, and print().", true},
		{chemistry, "I'm Alex, your CSE 110 instructor. What would you identify as the input?", true},
		{chemistry, "Using what you've learned so far, how would you compute the total?", true},
		{chemistry, "I'm Alex, a teacher at Lincoln Elementary. What would you identify as the input?", true},
		{education, "I'm Alex, a teacher at Lincoln Elementary. What would you identify as the input?", false},
		{education, "Using what you've learned in class, how would you grade this?", true},
		{chemistry, "I'm Alex, a process analyst at ChemCore Diagnostics. Our lab logs a sample's weight and volume. What would you compute first?", false},
		{chemistry, "Our weekly report totals seven daily readings. How would you store each day's value?", false},
	}
	for _, c := range cases {
		if got := speaksAsInstructor(c.state, c.text); got != c.want {
			t.Errorf("speaksAsInstructor(%s, %q) = %v, want %v", c.state.StudentMajor, c.text, got, c.want)
		}
	}
}

func TestInstructorFramingRetriesForward(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:            phaseAssessmentInProgress,
		ActiveMode:                   modeCode,
		CurrentWeekNumber:            2,
		StudentMajor:                 "computer science",
		ModeOpeningServed:            true,
		ModeUserAnsweredSinceOpening: true,
		ModeInterviewStep:            interviewStepDecompositionAnswered,
	}
	assistant := "Got it. Using only what you've learned in weeks 1 and 2, please paste your Python code for the task.\n\n```_ipyintervu\n{\"codeAssessmentPhase\": \"in_progress\"}\n```"
	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "corrective_retry" || !strings.Contains(followUp.Handoff, "never as instructors") || !strings.Contains(followUp.Handoff, "paste their Python code") {
		t.Fatalf("expected a forward retry as job interviewers, got %+v", followUp)
	}
}

// recordingUpstream serves replies in order and records every request's messages.
func recordingUpstream(t *testing.T, replies ...string) (*[][]chatMessage, func()) {
	t.Helper()
	var requests [][]chatMessage
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if isCachePrimeRequest(req) {
			return
		}
		requests = append(requests, req.Messages)
		content := replies[min(len(requests), len(replies))-1]
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": content}}},
		})
	}))
	saved := openRouterURL
	openRouterURL = upstream.URL
	return &requests, func() { openRouterURL = saved; upstream.Close() }
}

// isCachePrimeRequest reports the background cache-priming call (primeCacheWithPrompt),
// which is not part of the interview the tests check.
func isCachePrimeRequest(req chatCompletionRequest) bool {
	return len(req.Messages) == 2 && req.Messages[1].Role == "user" && req.Messages[1].Content == "Ready."
}

func runChatTurn(t *testing.T, states *agentStateStore, messages []chatMessage) string {
	t.Helper()
	body, _ := json.Marshal(chatCompletionRequest{Messages: messages})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(string(body)))
	req.Header.Set(turnIDHeader, "turn-1")
	req = req.WithContext(context.WithValue(req.Context(), sessionIDContextKey, "session-1"))
	rec := httptest.NewRecorder()
	handleChat("test-key", states, newTurnStore())(rec, req)
	return extractAssistantContent(rec.Body.Bytes())
}

// assertConversationUnchanged checks a follow-up model call carried the student's
// conversation as-is (only the system prompt differs) and returns that system prompt.
func assertConversationUnchanged(t *testing.T, first, later []chatMessage) string {
	t.Helper()
	if len(later) != len(first) {
		t.Fatalf("follow-up call has %d messages, first call had %d: %+v", len(later), len(first), later)
	}
	for i := 1; i < len(first); i++ {
		if later[i] != first[i] {
			t.Fatalf("message %d changed: %+v -> %+v", i, first[i], later[i])
		}
	}
	for _, m := range later[1:] {
		if strings.Contains(m.Content, "[System") {
			t.Fatalf("server note leaked into the conversation as %s message: %q", m.Role, m.Content)
		}
	}
	return later[0].Content
}

// Replays a reported Week 2 start: the retry used to append the rejected reply and the
// correction note as a user message, and the model thanked the student and assessed its
// own rejected explanation as the student's answer.
func TestCorrectiveRetryKeepsStudentConversation(t *testing.T) {
	good := "I'm Alex, a data analyst at DataForge Analytics. Our dashboard stores a day's page views and the number of visitors. What expression would you write to compute views per visitor?\n\n```_ipyintervu\n{\"conceptualAssessmentPhase\": \"in_progress\"}\n```"
	requests, done := recordingUpstream(t, week2ExplanationReply, good)
	defer done()

	states := newAgentStateStore()
	state := newAgentSessionState()
	state.ConversationPhase = phaseAwaitingKeyConcept
	state.StudentMajor = "computer science"
	states.set("session-1", state)

	got := runChatTurn(t, states, []chatMessage{
		{Role: "assistant", Content: setupMajorAcknowledgedMessage("computer science")},
		{Role: "user", Content: "week 2"},
	})
	if len(*requests) != 2 {
		t.Fatalf("expected 2 model calls, got %d", len(*requests))
	}
	system := assertConversationUnchanged(t, (*requests)[0], (*requests)[1])
	if last := (*requests)[1][len((*requests)[1])-1]; last.Role != "user" || last.Content != "week 2" {
		t.Fatalf("retry must end with the student's real message, got %+v", last)
	}
	if !strings.HasPrefix(system, "SERVER INSTRUCTION FOR THIS REPLY") || !strings.Contains(system, "<<<\nGot it. A variable is like a named box") {
		t.Fatalf("retry system prompt should carry the correction and the rejected draft, got %.400q", system)
	}
	if strings.Contains((*requests)[0][0].Content, "SERVER INSTRUCTION") {
		t.Fatal("first call must not carry a server instruction")
	}
	if !strings.HasPrefix(got, "I'm Alex") {
		t.Fatalf("expected the valid retry to be shown, got %q", got)
	}
}

func TestPhaseHandoffKeepsStudentConversation(t *testing.T) {
	closing := "That covers it.\n\n```_ipyintervu\n{\"conceptualAssessmentPhase\": \"complete\", \"conceptualAssessmentBucket\": \"Competent\"}\n```"
	intro := "Hi, I'm Taylor, a software developer at DataForge Analytics. Our dashboard stores total page views and days in variables. How would you break the task of computing average daily views into steps?\n\n```_ipyintervu\n{\"codeAssessmentPhase\": \"in_progress\"}\n```"
	requests, done := recordingUpstream(t, closing, intro)
	defer done()

	states := newAgentStateStore()
	states.set("session-1", &AgentSessionState{
		ConversationPhase:            phaseAssessmentInProgress,
		ActiveMode:                   modeConceptual,
		CurrentWeekNumber:            2,
		StudentMajor:                 "computer science",
		ConceptualAssessmentPhase:    assessmentPhaseInProgress,
		ModeOpeningServed:            true,
		ModeUserAnsweredSinceOpening: true,
	})
	got := runChatTurn(t, states, []chatMessage{
		{Role: "assistant", Content: "What expression would you write to compute views per visitor?"},
		{Role: "user", Content: "views / visitors"},
	})
	if len(*requests) != 2 {
		t.Fatalf("expected 2 model calls, got %d", len(*requests))
	}
	system := assertConversationUnchanged(t, (*requests)[0], (*requests)[1])
	if !strings.Contains(system, "already handled by the previous part") || !strings.Contains(system, "activeMode is now CodeProblem") {
		t.Fatalf("handoff should be in the system prompt, got %.400q", system)
	}
	if !strings.HasPrefix(got, phaseClosingMessage) || !strings.Contains(got, "Hi, I'm Taylor") {
		t.Fatalf("unexpected display %q", got)
	}
}
