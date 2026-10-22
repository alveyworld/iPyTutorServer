package main

import (
	"strings"
	"testing"
)

func TestLooksLikeSelfAnsweredQuestionConceptual(t *testing.T) {
	visible := "What is the difference between append and extend on a list? Append adds a single element to the end, while extend adds each element from an iterable."
	if !looksLikeSelfAnsweredQuestion(visible) {
		t.Fatal("expected self-answered conceptual question to be detected")
	}
}

func TestLooksLikeSelfAnsweredQuestionSimulatedStudent(t *testing.T) {
	visible := "How would you find this bug?\n\nStudent: I would add print statements near the input parsing."
	if !looksLikeSelfAnsweredQuestion(visible) {
		t.Fatal("expected simulated student reply to be detected")
	}
}

func TestLooksLikeSelfAnsweredQuestionMultipleQuestionsOnly(t *testing.T) {
	visible := "What is append? What is extend?"
	if looksLikeSelfAnsweredQuestion(visible) {
		t.Fatal("expected multiple questions without answer to pass")
	}
}

func TestLooksLikeSelfAnsweredQuestionShortClosing(t *testing.T) {
	visible := "What would you check first in this snippet?"
	if looksLikeSelfAnsweredQuestion(visible) {
		t.Fatal("expected question-only reply to pass")
	}
}

func TestGuardQuestionOnlyResponseTruncatesAnswer(t *testing.T) {
	raw := "What is the difference between append and extend? Append adds a single element to the end, while extend adds each element from an iterable."
	got := guardQuestionOnlyResponse(raw)
	want := "What is the difference between append and extend?"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestGuardQuestionOnlyResponsePreservesBugSnippetBeforeQuestion(t *testing.T) {
	raw := strings.Join([]string{
		"Here is a snippet:",
		"```python",
		"nums = [1, 2]",
		"print(nums[2])",
		"```",
		"Expected behavior: print the third item.",
		"How would you find the bug?",
	}, "\n")
	got := guardQuestionOnlyResponse(raw)
	if got != raw {
		t.Fatalf("expected bug snippet presentation to remain unchanged, got %q", got)
	}
}

func TestClientVisibleAssistantContentGuardedDuringAssessment(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeConceptual,
		CurrentWeekNumber: 8,
	}
	raw := "What is slicing? Slicing lets you access a portion of a sequence using start and stop indices.\n\n```_ipyintervu\n{\"conceptualAssessmentPhase\": \"in_progress\"}\n```"
	got := clientVisibleAssistantContentGuarded(raw, state)
	if strings.Contains(got, "Slicing lets you") {
		t.Fatalf("expected answer portion to be stripped, got %q", got)
	}
	if !strings.HasSuffix(strings.TrimSpace(got), "?") {
		t.Fatalf("expected truncated reply to end on the question, got %q", got)
	}
}

func TestClientVisibleAssistantContentGuardedDisabledInCoaching(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeCoaching,
		CoachingRequested: true,
		CurrentWeekNumber: 8,
	}
	raw := "What is slicing? Slicing lets you access a portion of a sequence."
	got := clientVisibleAssistantContentGuarded(raw, state)
	if got != raw {
		t.Fatalf("coaching replies should not be truncated, got %q", got)
	}
}

func TestLooksLikeCompositeAssessmentReply(t *testing.T) {
	visible := "What would you identify as the input? What would you identify as the process?"
	if !looksLikeCompositeAssessmentReply(visible) {
		t.Fatal("expected composite multi-question reply to be detected")
	}
}

func TestScenarioPlusOneQuestionNotComposite(t *testing.T) {
	visible := strings.Join([]string{
		"I'm Alex at Acme Research. We help education clients analyze service quality.",
		"",
		"Here is today's task: a district wants weekly satisfaction scores from parents who completed a school visit. Individual survey responses arrive before anyone builds the final report.",
		"",
		"What would you identify as the input — the information that has to be available before any processing begins?",
	}, "\n")
	if looksLikeCompositeAssessmentReply(visible) {
		t.Fatal("scenario plus one student-directed question should not be composite")
	}
}

func TestGuardQuestionOnlyResponseTruncatesForExampleAfterQuestion(t *testing.T) {
	raw := "What are the three inputs? For example, research question and audience."
	got := guardQuestionOnlyResponse(raw)
	want := "What are the three inputs?"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestGuardAssessmentContentResponseKeepsFirstQuestionOnly(t *testing.T) {
	raw := "What would you identify as the input? What would you identify as the process?"
	got := guardAssessmentContentResponse(raw)
	want := "What would you identify as the input?"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLooksLikeSelfAnsweredQuestionSimulatedStudentLine(t *testing.T) {
	visible := strings.Join([]string{
		"What would you consider the final output of this process to be?",
		"a table where each row was a region and an amount",
		"Good point — how might you handle invalid region codes?",
	}, "\n")
	if !looksLikeSelfAnsweredQuestion(visible) {
		t.Fatal("expected simulated student answer line after question to be detected")
	}
}

func TestGuardQuestionOnlyResponseSimulatedStudentLine(t *testing.T) {
	raw := strings.Join([]string{
		"What would you consider the final output of this process to be?",
		"a table where each row was a region and an amount",
		"Good point — how might you handle invalid region codes?",
	}, "\n")
	got := guardQuestionOnlyResponse(raw)
	want := "What would you consider the final output of this process to be?"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestGuardStackedMiniInterviewWithSimulatedAnswers(t *testing.T) {
	visible := strings.Join([]string{
		"Got it. At QuantCore, we often work with exact integer results. If you had meters = 7 and pieces = 2, what would the expression parts = meters // pieces produce, and what data type would it be?",
		"the type would be integer. The value would be 3.",
		"Got it. After that runs, y would still be 7 because Python evaluated y = x + 2 using the value of x at that moment (5), and reassigning x later does not retroactively update y.",
		"",
		"Now, at QuantCore we often work with area calculations. If a rectangle has width = 4 and height = 7, what expression would you write to compute its area, and what data type would the result be?",
	}, "\n")
	if !looksLikeSelfAnsweredQuestion(visible) {
		t.Fatal("expected stacked mini-interview with simulated answers to be detected")
	}
	if !looksLikeCompositeAssessmentReply(visible) {
		t.Fatal("expected multiple interview questions to be detected as composite")
	}
	if !looksLikeSevereCompositeReply(visible) {
		t.Fatal("expected severe composite reply")
	}
	got := guardAssessmentContentResponse(visible)
	if strings.Contains(got, "the type would be integer") {
		t.Fatalf("expected simulated student answer stripped, got %q", got)
	}
	if strings.Contains(got, "y would still be 7") {
		t.Fatalf("expected self-answered follow-up stripped, got %q", got)
	}
	if strings.Contains(got, "rectangle has width") {
		t.Fatalf("expected second stacked question stripped, got %q", got)
	}
	if !strings.Contains(got, "meters // pieces") {
		t.Fatalf("expected first question preserved, got %q", got)
	}
}

func TestModeHandoffCodeIntroGuardBehavior(t *testing.T) {
	handoff := "Thanks. That covers our conceptual questions on lists. I have what I need for this portion."
	codeIncomplete := strings.Join([]string{
		"I'll pass things over to my colleagues for the coding portion.",
		"",
		"Hi, I'm Taylor, a quantitative developer at QuantCore Analytics. I work with data pipelines and analysis tools every day. My colleague Morgan will also be joining us.",
		"",
		"Here's the scenario: We have a list of exam scores from a recent training module—for example, [78, 92, 85, 70, 95, 88, 76, 91]. We need a program that calculates and prints the average score.",
		"",
		"Before we get to writing code, could you walk me through how you'd break this problem down? What steps would you identify as the input, the",
	}, "\n")
	codeTwoQuestions := strings.Replace(codeIncomplete,
		"What steps would you identify as the input, the",
		"What steps would you identify as the input, the process, and the output?", 1)
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeCode,
		CurrentWeekNumber: 8,
	}
	for name, code := range map[string]string{"incomplete": codeIncomplete, "twoQuestions": codeTwoQuestions} {
		withSync := code + "\n\n```_ipyintervu\n{\"codeAssessmentPhase\": \"in_progress\"}\n```"
		v := detectAssessmentViolations(state, withSync)
		guarded := clientVisibleAssistantContentGuarded(handoff+"\n\n"+code, state)
		t.Logf("%s: content=%v composite=%v questions=%d guarded_suffix=%q",
			name, v.Content, looksLikeCompositeAssessmentReply(code), countStudentDirectedQuestions(code),
			guarded[len(guarded)-100:])
	}
}
