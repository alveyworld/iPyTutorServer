package main

import "strings"

// Setup-phase messages are server-authored so the welcome and the weekly list are always
// exact; the model is only called once a key concept is selected.

const setupWelcomeMessage = "Welcome to IPyIntervu 👋\n\n" +
	"I conduct brief, interview-style assessments for introductory Python concepts. " +
	"Each session focuses on one weekly key concept only, and we won't track history across concepts—just today's focus.\n\n" +
	"Before we begin, what's your major?"

const setupReaskMajorMessage = "Before we begin, what's your major?"

func weeklyKeyConceptList() string {
	lines := make([]string, len(weeklyKeyConceptSelections))
	for i, sel := range weeklyKeyConceptSelections {
		lines[i] = "- " + sel.SelectedKeyConcept
	}
	return strings.Join(lines, "\n")
}

func setupMajorAcknowledgedMessage(major string) string {
	return "Thanks - I have your major as " + major + ".\n\n" +
		weeklyKeyConceptList() + "\n\n" +
		"Please choose one of these key concepts for us to assess today."
}

func setupInvalidWeekSelectionMessage() string {
	return "That isn't one of the listed key concepts.\n\n" +
		weeklyKeyConceptList() + "\n\n" +
		"Please choose one of these key concepts for us to assess today."
}

// serverSetupReply returns the fixed reply for a setup-phase user turn (after
// applyPreChatUserUpdate), or false once the turn has started the assessment.
func serverSetupReply(phaseBefore string, state *AgentSessionState) (string, bool) {
	switch state.ConversationPhase {
	case phaseAwaitingMajor:
		return setupReaskMajorMessage, true
	case phaseAwaitingKeyConcept:
		if phaseBefore == phaseAwaitingMajor {
			return setupMajorAcknowledgedMessage(state.StudentMajor), true
		}
		return setupInvalidWeekSelectionMessage(), true
	default:
		return "", false
	}
}
