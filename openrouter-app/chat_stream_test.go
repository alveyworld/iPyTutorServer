package main

import (
	"strings"
	"testing"
)

func TestHandoffVisibleIncludesPriorModeClosing(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeBug,
		CurrentWeekNumber: 8,
	}
	codeClose := "Thanks. That covers the code portion.\n\n```_ipyintervu\n{\"codeAssessmentPhase\": \"complete\", \"codeAssessmentBucket\": \"Competent\"}\n```"
	bugIntro := strings.Join([]string{
		"Hi, I'm Riley, a quality analyst at QuantCore Analytics.",
		"Here is the snippet:",
		"```python",
		"tickets = [3, 7, 2]",
		"```",
		"What would you check first?",
		"```_ipyintervu",
		"{\"bugAssessmentPhase\": \"in_progress\"}",
		"```",
	}, "\n")
	combined := buildDisplayAssistantRaw([]string{codeClose}, bugIntro)
	visible := clientVisibleAssistantContentGuarded(combined, state)
	if !strings.Contains(visible, "Thanks. That covers the code portion.") {
		t.Fatalf("expected code closing in combined visible, got %q", visible)
	}
	if !strings.Contains(visible, "Hi, I'm Riley") {
		t.Fatalf("expected bug intro in combined visible, got %q", visible)
	}
	if strings.Contains(visible, "_ipyintervu") {
		t.Fatalf("expected sync blocks stripped, got %q", visible)
	}
}

func TestBuildUnifiedCorrectiveHandoffForbidsRePresentMeta(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase: phaseAssessmentInProgress,
		ActiveMode:        modeBug,
	}
	handoff := buildUnifiedCorrectiveHandoff(state, assessmentViolations{MissingSync: true})
	if !strings.Contains(handoff, "Do NOT mention re-presenting") {
		t.Fatalf("expected corrective handoff to forbid re-present meta, got %q", handoff)
	}
	if !strings.Contains(handoff, "Re-present the SAME concrete scenario") {
		t.Fatalf("opening turn should still rewind scenario, got %q", handoff)
	}
}
