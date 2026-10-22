package main

import (
	"strings"
	"testing"
)

func d5TestState(week int, mode string) (*AgentSessionState, *d5Session) {
	state := newAgentSessionState()
	state.ConversationPhase = phaseAssessmentInProgress
	state.CurrentWeekNumber = week
	state.ActiveMode = mode
	state.StudentMajor = "Nutrition Science"
	sess := newD5Session("test")
	return state, sess
}

func askAndAnswer(state *AgentSessionState, sess *d5Session, move d5Move, question, answer string) {
	d5ApplyInterviewerReply(state, sess, move, question)
	d5RecordAnswer(state, sess, answer)
}

func TestChooseMoveConceptualFollowUpThenCap(t *testing.T) {
	state, sess := d5TestState(5, modeConceptual)
	open := d5Move{Kind: moveOpenMode, Target: dimConceptual}
	askAndAnswer(state, sess, open, "A clinic sorts patients by risk. How would you decide which group a patient belongs in?", "I would compare their score against each threshold, starting from the highest risk band.")
	move := d5ChooseMove(state, sess, nil, "I would compare their score against each threshold, starting from the highest risk band.", false)
	if move.Kind != moveFollowUp || move.Target != dimConceptual {
		t.Fatalf("move = %+v, want FOLLOW_UP", move)
	}
	questions := []string{
		"Why does the order of those checks matter here?",
		"What happens to someone exactly on a boundary score?",
		"How would combining two conditions change the grouping?",
		"Which situation would need an else branch at the end?",
	}
	for _, q := range questions {
		askAndAnswer(state, sess, move, q, "A detailed answer that commits to a clear approach for the patients in this clinic scenario.")
	}
	if got := d5ChooseMove(state, sess, nil, "A detailed answer that commits to a clear approach.", false); got.Kind != moveCloseMode {
		t.Fatalf("after %d questions move = %+v, want CLOSE_MODE", len(state.ModeQuestionsAsked), got)
	}
}

func TestChooseMoveBriefCloseNeedsThreeConceptualQuestions(t *testing.T) {
	state, sess := d5TestState(5, modeConceptual)
	brief := &d5Brief{Mode: modeConceptual, Recommend: "close", Next: "x"}
	askAndAnswer(state, sess, d5Move{Kind: moveOpenMode, Target: dimConceptual}, "How would you group the patients?", "By comparing each score with the thresholds from highest to lowest.")
	if got := d5ChooseMove(state, sess, brief, "By comparing each score.", false); got.Kind == moveCloseMode {
		t.Fatal("brief close honoured before the 3rd question")
	}
	askAndAnswer(state, sess, d5Move{Kind: moveFollowUp, Target: dimConceptual}, "Why check the highest band first?", "Because a high score also passes the lower checks, so order matters.")
	askAndAnswer(state, sess, d5Move{Kind: moveFollowUp, Target: dimConceptual}, "What about a score exactly on the line?", "It depends on whether the comparison uses greater-or-equal, so I'd pick that deliberately.")
	if got := d5ChooseMove(state, sess, brief, "It depends on the comparison.", false); got.Kind != moveCloseMode {
		t.Fatalf("move = %+v, want CLOSE_MODE once 3 questions are asked", got)
	}
}

func TestChooseMoveVagueRedirectOnceThenClose(t *testing.T) {
	state, sess := d5TestState(5, modeBug)
	askAndAnswer(state, sess, d5Move{Kind: moveOpenMode, Target: dimStrategy}, "How would you find what's wrong here?", "idk")
	move := d5ChooseMove(state, sess, nil, "idk", false)
	if move.Kind != moveRedirectVague {
		t.Fatalf("first vague answer: move = %+v, want REDIRECT_VAGUE", move)
	}
	askAndAnswer(state, sess, move, "Could you give me one concrete step you'd take first?", "not sure")
	if got := d5ChooseMove(state, sess, nil, "not sure", false); got.Kind != moveCloseMode {
		t.Fatalf("second vague answer: move = %+v, want CLOSE_MODE", got)
	}
	labels := sess.labelsSnapshot()[modeBug]
	if len(labels) != 2 || labels[0].Level != levelNotReady || labels[0].Dimension != dimStrategy {
		t.Fatalf("vague answers must be labelled not_ready on strategy, got %+v", labels)
	}
}

func TestChooseMoveCodeSequence(t *testing.T) {
	state, sess := d5TestState(5, modeCode)
	askAndAnswer(state, sess, d5Move{Kind: moveOpenMode, Target: dimDecomposition}, "Data available: ... What's wanted: ... How would you break this problem down?", "First read the subtotal, then apply the discount if it is over 50, then add the fee, then print the total.")
	move := d5ChooseMove(state, sess, nil, "First read the subtotal...", false)
	if move.Kind != moveRequestCode || move.Target != dimCorrectness {
		t.Fatalf("after decomposition: %+v, want REQUEST_CODE", move)
	}
	brief := &d5Brief{Mode: modeCode, Recommend: "close", Next: "x"}
	askAndAnswer(state, sess, move, "Please write the Python code and paste it here. AI tools are fine.", "ok working on it, give me a sec")
	if got := d5ChooseMove(state, sess, brief, "ok working on it", false); got.Kind != moveRequestCode {
		t.Fatalf("no code yet: %+v, want REQUEST_CODE again (brief close ignored before the paste)", got)
	}
	code := "```python\nsubtotal = float(input('Subtotal: '))\nif subtotal > 50:\n    subtotal = subtotal * 0.9\nprint(subtotal)\n```"
	askAndAnswer(state, sess, move, "Please paste your Python code here.", code)
	if !sess.CodePasted {
		t.Fatal("code paste not detected")
	}
	move = d5ChooseMove(state, sess, nil, code, false)
	if move.Kind != moveCodeFollowUp || move.Target != dimUnderstanding {
		t.Fatalf("after paste: %+v, want line follow-up", move)
	}
	askAndAnswer(state, sess, move, "Why did you multiply by 0.9 on that line?", "Multiplying by 0.9 takes ten percent off, which is the discount the bakery wants.")
	move = d5ChooseMove(state, sess, nil, "Multiplying by 0.9 takes ten percent off.", false)
	if move.Target != dimAIUse {
		t.Fatalf("second post-code move: %+v, want AI-use question", move)
	}
	askAndAnswer(state, sess, move, "Did you use any AI tools for this, and how did you check them?", "I asked an AI to explain float(input()) and then tested with 40 and 60 dollar orders.")
	if got := d5ChooseMove(state, sess, nil, "I asked an AI...", false); got.Kind != moveCloseMode {
		t.Fatalf("after explain + AI questions: %+v, want CLOSE_MODE", got)
	}
}

func TestChooseMoveWeek1ClosesWhenAllPartsCovered(t *testing.T) {
	state, sess := d5TestState(1, modeConceptual)
	askAndAnswer(state, sess, d5Move{Kind: moveOpenMode, Target: dimConceptual}, "How would you start breaking down the weekly meal plan task?", "I'd start by listing what we know about each client and what they need from the plan.")
	move := d5ChooseMove(state, sess, nil, "I'd start by listing...", false)
	if !strings.Contains(move.Instruction, "input") {
		t.Fatalf("week 1 follow-up should steer to input, got %q", move.Instruction)
	}
	askAndAnswer(state, sess, move, "What inputs would you collect from each client?", "Their calorie goals, allergies and food preferences.")
	askAndAnswer(state, sess, move, "What process turns those inputs into a plan?", "Filter recipes by allergy, then pick meals that add up to the calorie goal for each day.")
	askAndAnswer(state, sess, move, "What output would the client receive?", "A weekly table of meals with calories per day.")
	if got := d5ChooseMove(state, sess, nil, "A weekly table of meals.", false); got.Kind != moveCloseMode {
		t.Fatalf("all parts covered: %+v, want CLOSE_MODE", got)
	}
	if !d5FinalMode(state) {
		t.Fatal("week 1 conceptual close must end the interview")
	}
}

func TestClarifyDoesNotCountAsQuestion(t *testing.T) {
	state, sess := d5TestState(5, modeConceptual)
	d5ApplyInterviewerReply(state, sess, d5Move{Kind: moveOpenMode, Target: dimConceptual}, "How would you group the patients?")
	move := d5ChooseMove(state, sess, nil, "what do you mean by group?", true)
	if move.Kind != moveClarify {
		t.Fatalf("move = %+v", move)
	}
	d5ApplyInterviewerReply(state, sess, move, "Put another way: how would you decide which risk band each patient falls into?")
	if len(state.ModeQuestionsAsked) != 1 || state.ModeSimilarQuestionAsks != 0 {
		t.Fatalf("clarification counted: asked=%v similar=%d", state.ModeQuestionsAsked, state.ModeSimilarQuestionAsks)
	}
}

func TestFallbackQuestionPrefersBrief(t *testing.T) {
	state, sess := d5TestState(5, modeConceptual)
	brief := &d5Brief{Mode: modeConceptual, Fallback: "What happens on the boundary?"}
	if got := d5FallbackQuestion(state, sess, brief, d5Move{Kind: moveFollowUp}); got != brief.Fallback {
		t.Fatalf("got %q", got)
	}
	other := &d5Brief{Mode: modeCode, Fallback: "code question"}
	if got := d5FallbackQuestion(state, sess, other, d5Move{Kind: moveFollowUp}); got == other.Fallback {
		t.Fatal("a brief from another mode must not supply the fallback")
	}
}
