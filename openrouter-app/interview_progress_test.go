package main

import (
	"strings"
	"testing"
)

func TestIsFollowUpAssessmentTurn(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeBug,
	}
	if isFollowUpAssessmentTurn(state) {
		t.Fatal("expected false before opening served")
	}

	state.ModeOpeningServed = true
	if isFollowUpAssessmentTurn(state) {
		t.Fatal("expected false before user answered")
	}

	state.ModeUserAnsweredSinceOpening = true
	if !isFollowUpAssessmentTurn(state) {
		t.Fatal("expected follow-up turn after user answered opening")
	}
}

func TestForwardMissingSyncHandoffOnFollowUp(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:            phaseAssessmentInProgress,
		ActiveMode:                   modeBug,
		ModeInterviewStep:            interviewStepFollowUp,
		ModeOpeningServed:            true,
		ModeUserAnsweredSinceOpening: true,
	}
	handoff := buildUnifiedCorrectiveHandoff(state, assessmentViolations{MissingSync: true})
	if strings.Contains(handoff, "Re-present the SAME concrete scenario") {
		t.Fatalf("follow-up missing sync should forward, not rewind: %q", handoff)
	}
	if !strings.Contains(handoff, "do NOT re-present the opening scenario") {
		t.Fatalf("expected forward guidance, got %q", handoff)
	}
	if !strings.Contains(handoff, "debugging-process follow-up") {
		t.Fatalf("expected bug follow-up guidance, got %q", handoff)
	}
}

func TestRewindMissingSyncHandoffOnOpeningTurn(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeBug,
		ModeInterviewStep: interviewStepOpening,
	}
	handoff := buildUnifiedCorrectiveHandoff(state, assessmentViolations{MissingSync: true})
	if !strings.Contains(handoff, "Re-present the SAME concrete scenario") {
		t.Fatalf("opening missing sync should rewind, got %q", handoff)
	}
}

func TestContentViolationOnFollowUpMovesForward(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:            phaseAssessmentInProgress,
		ActiveMode:                   modeCode,
		ModeOpeningServed:            true,
		ModeUserAnsweredSinceOpening: true,
	}
	handoff := buildUnifiedCorrectiveHandoff(state, assessmentViolations{Content: true, MissingSync: true})
	if strings.Contains(handoff, "Re-present the SAME concrete scenario") {
		t.Fatalf("content violation after the student answered must not rewind, got %q", handoff)
	}
	if !strings.Contains(handoff, "Do NOT re-present the opening scenario") {
		t.Fatalf("expected forward guidance, got %q", handoff)
	}
}

func TestContentViolationOnOpeningRewinds(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeConceptual,
		ModeInterviewStep: interviewStepOpening,
	}
	handoff := buildUnifiedCorrectiveHandoff(state, assessmentViolations{Content: true, SevereContent: true})
	if !strings.Contains(handoff, "Re-present the SAME concrete scenario") {
		t.Fatalf("content violation on the opening should rewind, got %q", handoff)
	}
}

func TestCorrectiveHandoffsSayTheyAreNotFromTheStudent(t *testing.T) {
	opening := &AgentSessionState{ConversationPhase: phaseAssessmentInProgress, ActiveMode: modeConceptual}
	followUp := &AgentSessionState{
		ConversationPhase:            phaseAssessmentInProgress,
		ActiveMode:                   modeConceptual,
		ModeOpeningServed:            true,
		ModeUserAnsweredSinceOpening: true,
	}
	cases := map[string]string{
		"rewind":             buildUnifiedCorrectiveHandoff(opening, assessmentViolations{MissingSync: true}),
		"forward content":    buildUnifiedCorrectiveHandoff(followUp, assessmentViolations{Content: true, SevereContent: true}),
		"forward sync":       buildUnifiedCorrectiveHandoff(followUp, assessmentViolations{MissingSync: true}),
		"closing sync":       buildUnifiedCorrectiveHandoff(followUp, assessmentViolations{MissingSync: true, MissingSyncClosing: true}),
		"complete no bucket": buildUnifiedCorrectiveHandoff(followUp, assessmentViolations{CompleteWithoutBucket: true}),
	}
	for name, handoff := range cases {
		if !strings.Contains(handoff, correctiveNoteRule) {
			t.Errorf("%s handoff missing server-note rule: %q", name, handoff)
		}
	}
}

func TestAdvanceInterviewProgressOnUserAnswer(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeCode,
		ModeInterviewStep: interviewStepAwaitingAnswer,
		ModeOpeningServed: true,
	}
	advanceInterviewProgressOnUserAnswer(state)
	if !state.ModeUserAnsweredSinceOpening {
		t.Fatal("expected userAnsweredSinceOpening")
	}
	if state.ModeInterviewStep != interviewStepDecompositionAnswered {
		t.Fatalf("step = %q, want %q", state.ModeInterviewStep, interviewStepDecompositionAnswered)
	}
}

func TestUpdateInterviewProgressAfterAssistantOpening(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeBug,
	}
	assistant := "What would you check first?\n\n```_ipyintervu\n{\"bugAssessmentPhase\": \"in_progress\"}\n```"
	updateInterviewProgressAfterAssistant(state, assistant)
	if !state.ModeOpeningServed {
		t.Fatal("expected opening served")
	}
	if state.ModeInterviewStep != interviewStepAwaitingAnswer {
		t.Fatalf("step = %q, want %q", state.ModeInterviewStep, interviewStepAwaitingAnswer)
	}
}

func TestCodeStepAdvancesToAwaitingCodeFromAssistant(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:            phaseAssessmentInProgress,
		ActiveMode:                   modeCode,
		ModeInterviewStep:            interviewStepDecompositionAnswered,
		ModeOpeningServed:            true,
		ModeUserAnsweredSinceOpening: true,
	}
	assistant := "Thanks. Please paste your Python code here.\n\n```_ipyintervu\n{\"codeAssessmentPhase\": \"in_progress\"}\n```"
	updateInterviewProgressAfterAssistant(state, assistant)
	if state.ModeInterviewStep != interviewStepAwaitingCode {
		t.Fatalf("step = %q, want %q", state.ModeInterviewStep, interviewStepAwaitingCode)
	}
}

func TestResetModeInterviewProgressOnModeTransition(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:            phaseAssessmentInProgress,
		ActiveMode:                   modeConceptual,
		ConceptualAssessmentPhase:    assessmentPhaseComplete,
		ConceptualAssessmentBucket:   bucketCompetent,
		ModeInterviewStep:            interviewStepInterviewing,
		ModeOpeningServed:            true,
		ModeUserAnsweredSinceOpening: true,
	}
	if !applyAutomaticModeTransitions(state) {
		t.Fatal("expected mode continuation into code")
	}
	if state.ActiveMode != modeCode {
		t.Fatalf("activeMode = %q, want %q", state.ActiveMode, modeCode)
	}
	if state.ModeInterviewStep != interviewStepOpening || state.ModeOpeningServed {
		t.Fatalf("expected reset progress, got step=%q openingServed=%v", state.ModeInterviewStep, state.ModeOpeningServed)
	}
}

func TestInterviewProgressSnapshotIncludedForAssessment(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeCode,
		ModeInterviewStep: interviewStepDecompositionAnswered,
		ModeOpeningServed: true,
	}
	snap := state.snapshotForPrompt()
	progress, ok := snap["interviewProgress"].(map[string]any)
	if !ok {
		t.Fatalf("expected interviewProgress in snapshot, got %#v", snap["interviewProgress"])
	}
	if progress["step"] != interviewStepDecompositionAnswered {
		t.Fatalf("step = %v", progress["step"])
	}
}

func TestPostProcessUpdatesProgressOnSuccess(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeConceptual,
		CurrentWeekNumber: 8,
	}
	assistant := "Got it. What would happen if the list were empty?\n\n```_ipyintervu\n{\"conceptualAssessmentPhase\": \"in_progress\"}\n```"
	postProcessAssistantTurnWithGuard(state, assistant, false, nil)
	if !state.ModeOpeningServed {
		t.Fatal("expected progress updated on successful turn")
	}
}

func TestPostProcessDoesNotUpdateProgressBeforeCorrectiveRetry(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeBug,
	}
	assistant := "Got it."
	followUp := postProcessAssistantTurnWithGuard(state, assistant, false, nil)
	if followUp.Kind != "corrective_retry" {
		t.Fatalf("expected corrective_retry, got %+v", followUp)
	}
	if state.ModeOpeningServed {
		t.Fatal("should not mark opening served before corrective retry succeeds")
	}
}

const alexInputQuestion = "What would you identify as the input before any processing begins?"

// Replays a reported session: after the student answered Alex's input question, Julia
// re-introduced herself, re-presented the scenario, and asked for the input again.
func TestJuliaReaskingAnsweredInputQuestionTriggersForwardRetry(t *testing.T) {
	state := week1FollowUpState()
	state.ModeQuestionsAsked = []string{alexInputQuestion}
	state.LastUserMessageRaw = "the three buffer solutions, three beakers, the ph meter to be calibrated."
	assistant := strings.Join([]string{
		"Hello, I'm Julia. Welcome to ChemCore Diagnostics. I work in the analytical chemistry lab here.",
		"",
		"Here's the scenario: we need to calibrate the pH meter using the three buffer solutions and the three beakers.",
		"",
		"Before any processing starts, what would you identify as the input for this calibration task?",
		"",
		"```_ipyintervu",
		`{"conceptualAssessmentPhase": "in_progress"}`,
		"```",
	}, "\n")

	if v := detectAssessmentViolations(state, assistant); !v.RepeatedQuestion {
		t.Fatalf("expected repeated question violation, got %+v", v)
	}
	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "corrective_retry" {
		t.Fatalf("expected corrective_retry, got %+v", followUp)
	}
	if strings.Contains(followUp.Handoff, "Re-present the SAME concrete scenario") {
		t.Fatalf("retry must move forward, got %q", followUp.Handoff)
	}
	if !strings.Contains(followUp.Handoff, alexInputQuestion) {
		t.Fatalf("retry should name the answered question, got %q", followUp.Handoff)
	}
}

func TestIsRepeatedQuestion(t *testing.T) {
	cases := []struct {
		next string
		want bool
	}{
		{"Before any processing starts, what would you identify as the input for this calibration task?", true},
		{"What would you identify as the input?", false},
		{"What would you identify as the output?", false},
		{"What steps would you include in the process between the inputs and the final report?", false},
		{"Which of those buffers would you measure first, and why?", false},
	}
	for _, c := range cases {
		if got := isRepeatedQuestion(alexInputQuestion, c.next); got != c.want {
			t.Errorf("isRepeatedQuestion(%q) = %v, want %v", c.next, got, c.want)
		}
	}
}

func TestNextDecompositionQuestionIsNotARepeat(t *testing.T) {
	state := week1FollowUpState()
	state.ModeQuestionsAsked = []string{alexInputQuestion}
	state.LastUserMessageRaw = "the three buffer solutions"
	assistant := "Julia here, QA lead at ChemCore. What steps would you include in the process?\n\n```_ipyintervu\n{\"conceptualAssessmentPhase\": \"in_progress\"}\n```"

	if v := detectAssessmentViolations(state, assistant); v.RepeatedQuestion {
		t.Fatalf("asking about the process is not a repeat, got %+v", v)
	}
}

func TestRewordingAfterClarificationRequestIsAllowed(t *testing.T) {
	state := week1FollowUpState()
	state.ModeQuestionsAsked = []string{alexInputQuestion}
	state.LastUserMessageRaw = "what do you mean by input?"
	assistant := "Before any processing starts, what would you identify as the input for this calibration task?"

	if v := detectAssessmentViolations(state, assistant); v.RepeatedQuestion {
		t.Fatalf("rewording after a clarification request is allowed, got %+v", v)
	}
}

func TestDeliveredReplyRecordsLastQuestion(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeConceptual,
		CurrentWeekNumber: 1,
	}
	assistant := "I'm Alex, a process analyst at ChemCore Diagnostics. Here's the scenario: calibrate the pH meter.\n\n" + alexInputQuestion + "\n\n```_ipyintervu\n{\"conceptualAssessmentPhase\": \"in_progress\"}\n```"

	updateInterviewProgressAfterAssistant(state, assistant)
	if len(state.ModeQuestionsAsked) != 1 || state.ModeQuestionsAsked[0] != alexInputQuestion {
		t.Fatalf("expected question recorded, got %q", state.ModeQuestionsAsked)
	}
	resetModeInterviewProgress(state)
	if state.ModeQuestionsAsked != nil {
		t.Fatal("mode reset should clear the last question")
	}
}

func TestSystemPromptStatesAnsweredQuestionOnFollowUp(t *testing.T) {
	state := week1FollowUpState()
	state.ModeQuestionsAsked = []string{alexInputQuestion}

	prompt, _, _, err := buildSystemPrompt(state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "THIS TURN:") || !strings.Contains(prompt, alexInputQuestion) {
		t.Fatal("follow-up prompt should state the answered question")
	}

	opening := &AgentSessionState{ConversationPhase: phaseAssessmentInProgress, ActiveMode: modeConceptual, CurrentWeekNumber: 1}
	prompt, _, _, err = buildSystemPrompt(opening)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, "THIS TURN:") {
		t.Fatal("opening turn should not carry the follow-up directive")
	}
}

func withSync(text string) string {
	return text + "\n\n```_ipyintervu\n{\"conceptualAssessmentPhase\": \"in_progress\"}\n```"
}

// Replays a reported Week 1 session end to end through the state machine. The model asked
// for the input three times and, after input, process, and output were all answered, kept
// interviewing instead of closing with a bucket.
func TestWeek1ReplayClosesAfterInputProcessOutput(t *testing.T) {
	state := newAgentSessionState()
	state.ConversationPhase = phaseAwaitingKeyConcept
	state.StudentMajor = "mathematics"
	applyPreChatUserUpdate(state, "week 1")
	if state.ActiveMode != modeConceptual || !state.isProblemDecompositionWeek() {
		t.Fatalf("expected week 1 conceptual, got mode=%q week=%d", state.ActiveMode, state.CurrentWeekNumber)
	}

	deliver := func(assistant, student string) {
		t.Helper()
		if f := postProcessAssistantTurn(state, withSync(assistant), false, nil); f.Kind != "" || f.ContinueTurn {
			t.Fatalf("expected %q to be delivered, got %+v", assistant, f)
		}
		applyPreChatUserUpdate(state, student)
	}

	deliver("Great choice. I'm Alex, a quantitative analyst at Stratum Analytics. Here's the scenario: run a population growth simulation.\n\nWhat would you identify as the input in this scenario — the information or materials needed before any processing begins?",
		"population size, growth rate, time period, a computer, and the modeling software.")
	deliver("Got it. Now, what would you identify as the process — the steps you'd follow to go from those inputs to a result?",
		"I would launch the software, enter a set of inputs, and record the result, repeating for each scenario.")
	deliver("Got it. And once a result is recorded for a set of inputs, how would you know the result was correct?",
		"I would do a hand validation by calculating it myself.")

	// Turn 4 in the report asked for the input again.
	reask := withSync("Got it. Now, thinking back to the original task itself — what would you identify as the input, before any processing begins?")
	if f := postProcessAssistantTurn(state, reask, false, nil); f.Kind != "corrective_retry" || !strings.Contains(f.Handoff, "do not ask any of them again") {
		t.Fatalf("expected forward retry for re-asked input question, got %+v", f)
	}
	deliver("Got it. What would you consider the output of the task — what would you have at the end that shows the work is done?",
		"A single report consisting of each scenario paired with its result.")

	if !conceptualClosingDue(state) {
		t.Fatalf("closing should be due after input, process, and output; answered=%v", decompositionPartsAnswered(state))
	}
	prompt, _, _, err := buildSystemPrompt(state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "THIS TURN MUST CLOSE THE INTERVIEW") {
		t.Fatal("prompt should require the closing reply")
	}

	// The reported reply asked for the input yet again instead of closing.
	loop := withSync("Got it. Thanks, that gives me the output clearly. What would you identify as the input to this task — before any processing begins, what do you have on hand?")
	f := postProcessAssistantTurn(state, loop, false, nil)
	if f.Kind != "corrective_retry" || !strings.Contains(f.Handoff, "the interview is finished") {
		t.Fatalf("expected closing retry, got %+v", f)
	}

	closing := "Thanks, that completes our discussion today.\n\n```_ipyintervu\n{\"conceptualAssessmentPhase\": \"complete\", \"conceptualAssessmentBucket\": \"Competent\"}\n```"
	f = postProcessAssistantTurn(state, closing, true, nil)
	if f.Kind != "server_results" || !strings.Contains(f.DirectAssistant, "Overall Rating: Competent") {
		t.Fatalf("expected server results after closing, got %+v", f)
	}
}

func TestDecompositionPart(t *testing.T) {
	cases := map[string]string{
		"What would you identify as the input in this scenario?":                            "input",
		"What would you identify as the process — the steps from those inputs to a result?": "process",
		"What steps would you follow?":                                                      "process",
		"What would you consider the output of the task?":                                   "output",
		"How would you know the result was correct?":                                        "",
	}
	for q, want := range cases {
		if got := decompositionPart(q); got != want {
			t.Errorf("decompositionPart(%q) = %q, want %q", q, got, want)
		}
	}
}

func TestClosingNotDueBeforeAllPartsOrCap(t *testing.T) {
	state := week1FollowUpState()
	state.ModeQuestionsAsked = []string{alexInputQuestion, "What steps belong in the process?"}
	if conceptualClosingDue(state) {
		t.Fatal("closing should not be due before the output is asked")
	}
	state.CurrentWeekNumber = 3
	state.SelectedKeyConcept = "Week 3 - Input & Type Casting"
	state.ModeQuestionsAsked = []string{"a?", "b?", "c?", "d?"}
	if conceptualClosingDue(state) {
		t.Fatal("closing should not be due before the question cap")
	}
	state.ModeQuestionsAsked = append(state.ModeQuestionsAsked, "e?")
	if !conceptualClosingDue(state) {
		t.Fatal("closing should be due at the question cap")
	}
}

func withCodeSync(text string) string {
	return text + "\n\n```_ipyintervu\n{\"codeAssessmentPhase\": \"in_progress\"}\n```"
}

// Replays a reported Week 2 code phase: after the paste and explain-code and AI-use
// answers, the model kept interviewing and then asked for the code a second time.
func TestCodeReplayClosesAfterExplainAndAIUse(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:  phaseAssessmentInProgress,
		ActiveMode:         modeCode,
		CurrentWeekNumber:  2,
		SelectedKeyConcept: "Week 2 - Variables",
	}
	deliver := func(assistant, student string) {
		t.Helper()
		if f := postProcessAssistantTurn(state, withCodeSync(assistant), false, nil); f.Kind != "" || f.ContinueTurn {
			t.Fatalf("expected %q to be delivered, got %+v", assistant, f)
		}
		applyPreChatUserUpdate(state, student)
	}
	code := "price = 12.50\nshipping = 2.00\nquantity = 150\n\ncost_per_unit = price + shipping\ntotal_value = cost_per_unit * quantity\n\nprint(f\"Cost per unit: ${cost_per_unit:.2f}\")"

	deliver("I'm Morgan, a software developer here at CodeVault Systems. Each mouse costs $12.50, shipping is $2.00 per unit, and there are 150 units. Can you break this problem down into the specific steps your program would follow?",
		"add the product cost plus the shipping cost then multiply this sum by the number of units")
	deliver("Perfect start. Now please paste your Python code for the task.", code)
	if state.ModeInterviewStep != interviewStepCodeSubmitted {
		t.Fatalf("step = %q, want %q", state.ModeInterviewStep, interviewStepCodeSubmitted)
	}
	if codeClosingDue(state) {
		t.Fatal("closing should not be due before any question about the code")
	}
	deliver("Thanks for pasting your code. What value does cost_per_unit hold after cost_per_unit = price + shipping executes?",
		"the cost_per_unit is a float that is the total for an individual mouse.")

	// Re-requesting code after the paste is a repeat, even with a new wording.
	reask := withCodeSync("Got it. Please paste the Python code you wrote for this task.")
	if f := postProcessAssistantTurn(state, reask, false, nil); f.Kind != "corrective_retry" {
		t.Fatalf("expected retry for re-requested code, got %+v", f)
	}

	deliver("When you were putting together that script, did you use any AI tools, and how did you verify what it produced?",
		"I did. I read through it and ran it before giving it to you")

	if !codeClosingDue(state) {
		t.Fatalf("closing should be due after explain-code and AI-use; asked=%q", postCodeQuestions(state))
	}
	prompt, _, _, err := buildSystemPrompt(state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "THIS TURN MUST CLOSE THE INTERVIEW") || !strings.Contains(prompt, `"codeAssessmentPhase": "complete"`) {
		t.Fatal("prompt should require the code closing reply")
	}

	loop := withCodeSync("Got it. Since you have a working script, please paste the Python code you wrote for this task.")
	if f := postProcessAssistantTurn(state, loop, false, nil); f.Kind != "corrective_retry" || !strings.Contains(f.Handoff, "the interview is finished") {
		t.Fatalf("expected closing retry, got %+v", f)
	}
}

func TestCodeRequestPattern(t *testing.T) {
	for text, want := range map[string]bool{
		"please paste the python code you wrote for this task.": true,
		"can you share your script?":                            true,
		"thanks for pasting your code.":                         false,
		"walk me through the code you pasted.":                  false,
	} {
		if got := looksLikeCodeRequest(text); got != want {
			t.Errorf("looksLikeCodeRequest(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestIsVagueAnswer(t *testing.T) {
	for msg, want := range map[string]bool{
		"idk":                             true,
		"I don't know.":                   true,
		"not sure, maybe print something": true,
		"print statements":                true,
		"":                                true,
		"I'm not sure of the cause, but I would print the running total before and after the addition and compare it to the intended total": false,
		"I would reread the intended behavior, then print the total on each pass to see where it drifts":                                    false,
		"what do you mean by narrowing it down?": false,
		"I'm confused":                           false,
	} {
		if got := isVagueAnswer(msg); got != want {
			t.Errorf("isVagueAnswer(%q) = %v, want %v", msg, got, want)
		}
	}
}

func withBugSync(text string) string {
	return text + "\n\n```_ipyintervu\n{\"bugAssessmentPhase\": \"in_progress\"}\n```"
}

func bugFollowUpState() *AgentSessionState {
	return &AgentSessionState{
		ConversationPhase:            phaseAssessmentInProgress,
		ActiveMode:                   modeBug,
		CurrentWeekNumber:            6,
		SelectedKeyConcept:           "Week 6 - Loops",
		BugAssessmentPhase:           assessmentPhaseInProgress,
		ModeOpeningServed:            true,
		ModeUserAnsweredSinceOpening: true,
		ModeInterviewStep:            interviewStepFollowUp,
	}
}

// Two vague answers end the mode: the reported loop was the interviewer re-approaching the
// same ground after each hedge instead of scoring what it already had.
func TestBugModeClosesAfterTwoVagueAnswers(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:  phaseAssessmentInProgress,
		ActiveMode:         modeBug,
		CurrentWeekNumber:  6,
		SelectedKeyConcept: "Week 6 - Loops",
		BugAssessmentPhase: assessmentPhaseInProgress,
	}
	deliver := func(assistant, student string) {
		t.Helper()
		if f := postProcessAssistantTurn(state, withBugSync(assistant), false, nil); f.Kind != "" || f.ContinueTurn {
			t.Fatalf("expected %q to be delivered, got %+v", assistant, f)
		}
		applyPreChatUserUpdate(state, student)
	}

	deliver("I'm Riley, a QA engineer here at Northwind Tools. Our internal report script prints a running total that ends up one short of the intended amount every run. How would you go about finding where that happens?",
		"idk")
	if state.ModeVagueAnswers != 1 {
		t.Fatalf("vague answers = %d, want 1", state.ModeVagueAnswers)
	}
	if modeClosingDue(state) {
		t.Fatal("one vague answer should not end the mode")
	}

	deliver("Understood. What would you want in front of you before changing anything in that script?",
		"not sure")
	if !modeClosingDue(state) {
		t.Fatalf("closing should be due after %d vague answers", state.ModeVagueAnswers)
	}
	prompt, _, _, err := buildSystemPrompt(state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "THIS TURN MUST CLOSE THE INTERVIEW") || !strings.Contains(prompt, "answered vaguely") {
		t.Fatal("prompt should require the closing reply and name the vague answers")
	}

	narrower := withBugSync("Let's simplify. What is the very first value you would inspect in that script?")
	if f := postProcessAssistantTurn(state, narrower, false, nil); f.Kind != "corrective_retry" || !strings.Contains(f.Handoff, "the interview is finished") {
		t.Fatalf("expected closing retry instead of another narrower question, got %+v", f)
	}

	closing := "Thanks — that's everything I needed for this portion.\n\n```_ipyintervu\n{\"bugAssessmentPhase\": \"complete\", \"bugAssessmentBucket\": \"Not Ready Yet\"}\n```"
	if f := postProcessAssistantTurn(state, closing, true, nil); f.Kind != "server_results" {
		t.Fatalf("expected server results after the bug closing reply, got %+v", f)
	}
}

func TestBugModeQuestionCap(t *testing.T) {
	state := bugFollowUpState()
	state.LastUserMessageRaw = "I would print the running total on each pass and compare it with the intended amount."
	state.ModeQuestionsAsked = []string{"How would you find it?", "What would you check first?", "What if that showed nothing?"}
	if bugClosingDue(state) {
		t.Fatal("closing should not be due before the bug question cap")
	}
	state.ModeQuestionsAsked = append(state.ModeQuestionsAsked, "Which assumption would you test next?")
	if !bugClosingDue(state) {
		t.Fatalf("closing should be due at %d questions", maxBugQuestions)
	}
}

// A re-ask that slips past the corrective retry is the second and last ask of that
// question: the mode closes rather than circling it again.
func TestSimilarQuestionAsksLimitedToTwo(t *testing.T) {
	state := bugFollowUpState()
	state.ModeQuestionsAsked = []string{"How would you find where that running total goes wrong?"}
	state.LastUserMessageRaw = "I would print the running total before and after the addition."

	reask := withBugSync("Got it. How would you find where that running total goes wrong?")
	if f := postProcessAssistantTurn(state, reask, false, nil); f.Kind != "corrective_retry" {
		t.Fatalf("expected a corrective retry for the re-ask, got %+v", f)
	}
	if state.ModeSimilarQuestionAsks != 0 {
		t.Fatalf("a rejected reply the student never saw should not count, got %d", state.ModeSimilarQuestionAsks)
	}

	if f := postProcessAssistantTurn(state, reask, true, nil); f.Kind != "" {
		t.Fatalf("expected the post-retry reply to be delivered, got %+v", f)
	}
	if state.ModeSimilarQuestionAsks != 1 {
		t.Fatalf("similar asks = %d, want 1", state.ModeSimilarQuestionAsks)
	}
	if !modeClosingDue(state) {
		t.Fatal("closing should be due once a question has been asked twice")
	}
}

func TestRewordingAfterClarificationDoesNotCountAsSimilarAsk(t *testing.T) {
	state := bugFollowUpState()
	state.ModeQuestionsAsked = []string{"How would you find where that running total goes wrong?"}
	state.LastUserMessageRaw = "can you rephrase that?"

	reworded := withBugSync("Sure. How would you find where that running total goes wrong — what would you look at?")
	if f := postProcessAssistantTurn(state, reworded, false, nil); f.Kind != "" {
		t.Fatalf("expected the rewording to be delivered, got %+v", f)
	}
	if state.ModeSimilarQuestionAsks != 0 {
		t.Fatalf("a rewording the student asked for should not count, got %d", state.ModeSimilarQuestionAsks)
	}
}

func TestCodeModeLimitsWaitForThePaste(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:            phaseAssessmentInProgress,
		ActiveMode:                   modeCode,
		CurrentWeekNumber:            2,
		SelectedKeyConcept:           "Week 2 - Variables",
		ModeOpeningServed:            true,
		ModeUserAnsweredSinceOpening: true,
		ModeInterviewStep:            interviewStepAwaitingCode,
		ModeVagueAnswers:             3,
		ModeSimilarQuestionAsks:      2,
	}
	if modeClosingDue(state) {
		t.Fatal("code mode cannot close before the student pastes code")
	}
	state.ModeInterviewStep = interviewStepCodeSubmitted
	if !modeClosingDue(state) {
		t.Fatal("after the paste the vagueness and repetition caps apply")
	}
}
