package main

import (
	"fmt"
	"strings"
)

// d5MaxCodeRequests closes Code mode when the candidate has not pasted code after this
// many requests; grading then treats correctness as Not Ready (grading-rules.md).
const d5MaxCodeRequests = 3

const (
	d5ClosingInterview   = "Thanks — that completes the interview."
	d5CoachingAfterwards = "Coaching opens once the interview is finished."
	d5ResultsReminder    = "Your results are above. If you'd like feedback on how to improve, type 'switch to coach mode'."
	d5FallbackLeadIn     = "Thanks for walking me through that."
)

// d5RecordAnswer records a candidate message as an answer in the active mode: transcript,
// counters, the Go-set label for a vague answer (grading-rules.md, Step 1), and the code
// paste. It returns the rubric dimension the answered question targeted.
func d5RecordAnswer(state *AgentSessionState, sess *d5Session, msg string) string {
	mode := state.ActiveMode
	question := strings.TrimSpace(lastQuestionSentence(sess.lastInterviewerMessage()))
	if question == "" {
		question = sess.lastInterviewerMessage()
	}
	sess.AnswerIndex++
	sess.Transcript = append(sess.Transcript, d5Message{Role: "user", Content: msg, Mode: mode})
	state.ModeUserAnsweredSinceOpening = true
	target := sess.LastAskTarget
	noAI := target == dimAIUse && d5SaysNoAI(msg)
	sess.mu.Lock()
	sess.Answers[sess.AnswerIndex] = d5Answer{Mode: mode, Question: question, Answer: msg, Target: target}
	if noAI {
		sess.NoAIAnswers[sess.AnswerIndex] = true
	}
	sess.mu.Unlock()
	if noAI {
		// Students are expected to use AI, so not using it caps AI use at Competent: Code
		// cannot be Exceptional, but this alone never causes Not Ready Yet. Like a vague
		// answer's label, it stands over any labeller's.
		sess.addLabels(mode, []gradeLabel{{Dimension: dimAIUse, Level: levelCompetent, AnswerIndex: sess.AnswerIndex, Source: labelSourceVague}})
		return target
	}

	if mode == modeCode && !sess.CodePasted && d5LooksLikeCodeSubmission(msg) {
		sess.CodePasted = true
		state.ModeInterviewStep = interviewStepCodeSubmitted
		state.ModeQuestionsBeforeCode = len(state.ModeQuestionsAsked)
		// The vague and repetition limits count from the paste (B8 in the rules checklist).
		state.ModeVagueAnswers = 0
		state.ModeSimilarQuestionAsks = 0
		return target
	}
	if d5IsVagueAnswer(msg) {
		state.ModeVagueAnswers++
		sess.VagueAnswers[sess.AnswerIndex] = true
		if target != "" {
			sess.addLabels(mode, []gradeLabel{{Dimension: target, Level: levelNotReady, AnswerIndex: sess.AnswerIndex, Source: labelSourceVague}})
		}
	}
	return target
}

// d5ClosingDue reports whether the active mode must close now, and why (design §6).
func d5ClosingDue(state *AgentSessionState, sess *d5Session, brief *d5Brief) (bool, string) {
	mode := state.ActiveMode
	asked := len(state.ModeQuestionsAsked)
	limitsApply := mode != modeCode || sess.CodePasted
	if limitsApply && state.ModeVagueAnswers >= maxVagueAnswers {
		return true, "two vague answers"
	}
	if limitsApply && state.ModeSimilarQuestionAsks >= maxSimilarQuestionAsks-1 {
		return true, "a question was asked twice"
	}
	switch mode {
	case modeConceptual:
		if state.isProblemDecompositionWeek() && asked > 0 && len(decompositionPartsRemaining(state)) == 0 {
			return true, "input, process and output covered"
		}
		if asked >= maxConceptualQuestions {
			return true, "question cap"
		}
	case modeCode:
		if !sess.CodePasted && sess.CodeRequests >= d5MaxCodeRequests {
			return true, "no code after repeated requests"
		}
		if sess.CodePasted {
			post := postCodeQuestions(state)
			if len(post) >= maxPostCodeQuestions || (len(post) >= 2 && askedAboutAIUse(post)) {
				return true, "code explained and AI use covered"
			}
		}
	case modeBug:
		if asked >= maxBugQuestions {
			return true, "question cap"
		}
	}
	if brief != nil && brief.Mode == mode && brief.Recommend == "close" {
		if mode == modeConceptual && asked < 3 {
			return false, ""
		}
		if mode == modeCode && !sess.CodePasted {
			return false, ""
		}
		return true, "evaluator recommends closing"
	}
	return false, ""
}

func briefNext(brief *d5Brief, mode string) string {
	if brief != nil && brief.Mode == mode {
		return brief.Next
	}
	return ""
}

// d5ChooseMove picks the move for a candidate message in the assessment (design §6).
// clarification means the message asked about the question rather than answering it; in
// that case it was not recorded as an answer.
func d5ChooseMove(state *AgentSessionState, sess *d5Session, brief *d5Brief, msg string, clarification bool) d5Move {
	mode := state.ActiveMode
	if clarification {
		return d5Move{Kind: moveClarify, Target: sess.LastAskTarget, MaxTokens: d5ReplyMaxTokens,
			Instruction: "They asked for clarification. Restate your last question more simply, without giving the answer or adding a new question."}
	}
	if due, reason := d5ClosingDue(state, sess, brief); due {
		return d5Move{Kind: moveCloseMode, Reason: reason}
	}
	if d5IsVagueAnswer(msg) && !sess.RedirectUsed[mode] && !(mode == modeCode && state.ModeInterviewStep == interviewStepAwaitingCode) {
		return d5Move{Kind: moveRedirectVague, Target: sess.LastAskTarget, MaxTokens: d5ReplyMaxTokens,
			Instruction: "Their answer was too general to assess. Warmly ask for one concrete detail about the situation."}
	}

	switch mode {
	case modeCode:
		if !sess.CodePasted {
			instruction := "Respond to how they would break the problem down, then ask them to write the Python code for this task and paste it here. Tell them they're encouraged to use AI tools to help write it."
			if state.ModeInterviewStep == interviewStepAwaitingCode {
				instruction = "They haven't pasted their code yet. Respond briefly to what they said, then ask again for their Python code for this task, pasted here. They're encouraged to use AI tools."
			}
			return d5Move{Kind: moveRequestCode, Target: dimCorrectness, MaxTokens: d5ReplyMaxTokens, Instruction: instruction}
		}
		if post := postCodeQuestions(state); len(post) >= 1 && !askedAboutAIUse(post) {
			return d5Move{Kind: moveCodeFollowUp, Target: dimAIUse, MaxTokens: d5ReplyMaxTokens,
				Instruction: "Ask whether and how they used AI tools for this code, and how they checked what it produced."}
		}
		return d5Move{Kind: moveCodeFollowUp, Target: dimUnderstanding, MaxTokens: d5ReplyMaxTokens,
			Instruction: "Their code is in their latest message. Ask about one specific line or choice in their code and why they wrote it that way."}
	case modeBug:
		return d5Move{Kind: moveFollowUp, Target: dimStrategy, MaxTokens: d5ReplyMaxTokens,
			Instruction: followUpInstruction(brief, mode) + " Ask about their debugging process only (how they would find or narrow down the problem); never ask for fixed or corrected code."}
	default:
		instruction := followUpInstruction(brief, mode) + " The question must be about " + weekFocus(state) + ", in the context of the scenario; never drift to unrelated topics such as statistics or client communication. Keep it conceptual; no code-level details."
		if state.isProblemDecompositionWeek() {
			instruction = "Respond to their answer, then ask one follow-up question about the same scenario; never present a new one."
			if remaining := decompositionPartsRemaining(state); len(remaining) > 0 {
				instruction += " Ask about the part of the breakdown not yet discussed: " + remaining[0] + "."
			}
		}
		return d5Move{Kind: moveFollowUp, Target: dimConceptual, MaxTokens: d5ReplyMaxTokens, Instruction: instruction}
	}
}

func followUpInstruction(brief *d5Brief, mode string) string {
	if briefNext(brief, mode) != "" {
		return "Respond to their answer, then ask one follow-up question in the direction of your colleague's notes."
	}
	return "Respond to their answer, then ask one follow-up question that explores new ground about the scenario."
}

// d5OpeningMove is the move that opens a mode with a live Interviewer call.
func d5OpeningMove(state *AgentSessionState, sess *d5Session, mode string) d5Move {
	kind := moveOpenMode
	needsCompany := mode == modeConceptual && sess.CompanyName == ""
	if needsCompany {
		kind = moveOpenFirst
	}
	maxTokens := d5OpeningMaxTokens
	if mode == modeBug {
		maxTokens = d5BugOpeningMaxTokens
	}
	target := map[string]string{modeConceptual: dimConceptual, modeCode: dimDecomposition, modeBug: dimStrategy}[mode]
	return d5Move{Kind: kind, Target: target, MaxTokens: maxTokens, Instruction: openingInstruction(state, mode, needsCompany)}
}

// d5ApplyInterviewerReply records a delivered Interviewer reply: transcript, the question
// it asked, repetition counting and the mode step.
func d5ApplyInterviewerReply(state *AgentSessionState, sess *d5Session, move d5Move, reply string) {
	mode := state.ActiveMode
	sess.Transcript = append(sess.Transcript, d5Message{Role: "assistant", Content: reply, Mode: mode})
	sess.LastMove = string(move.Kind)
	if move.Target != "" {
		sess.LastAskTarget = move.Target
	}
	if move.Kind == moveRedirectVague {
		sess.RedirectUsed[mode] = true
	}
	if move.isOpening() {
		state.ModeOpeningServed = true
		state.ModeInterviewStep = interviewStepAwaitingAnswer
	}
	if move.Kind == moveRequestCode {
		state.ModeInterviewStep = interviewStepAwaitingCode
		sess.CodeRequests++
	}
	if move.Kind == moveClarify {
		return // a restated question is not a new question
	}
	if q := lastQuestionSentence(reply); q != "" {
		if !move.isOpening() && move.Kind != moveRequestCode && repeatsRecordedQuestion(state, q) {
			state.ModeSimilarQuestionAsks++
		}
		state.ModeQuestionsAsked = append(state.ModeQuestionsAsked, q)
	}
}

// d5FinalMode reports whether closing the active mode ends the interview.
func d5FinalMode(state *AgentSessionState) bool {
	return state.ActiveMode == modeBug || (state.ActiveMode == modeConceptual && state.isProblemDecompositionWeek())
}

// d5AdvanceMode moves to the next mode after the active one closes (forward only).
func d5AdvanceMode(state *AgentSessionState) string {
	appendModeCompleted(state, state.ActiveMode)
	next := modeCode
	if state.ActiveMode == modeCode {
		next = modeBug
	}
	state.ActiveMode = next
	state.PendingQuestion = pendingQuestionForMode(next)
	resetModeInterviewProgress(state)
	return next
}

// d5FinishAssessment moves to the results phase once the final mode closes.
func d5FinishAssessment(state *AgentSessionState) {
	appendModeCompleted(state, state.ActiveMode)
	state.ActiveMode = ""
	state.ConversationPhase = phaseAssessmentResults
	state.PendingQuestion = "assessmentResults"
	state.WaitingForUserResponse = true
	markAssessmentEnded(state)
}

// d5FallbackQuestion is the question sent when the Interviewer fails or asks nothing:
// the brief's FALLBACK when it fits the mode, otherwise a fixed one for the move.
func d5FallbackQuestion(state *AgentSessionState, sess *d5Session, brief *d5Brief, move d5Move) string {
	mode := state.ActiveMode
	switch move.Kind {
	case moveClarify:
		if q := lastQuestionSentence(sess.lastInterviewerMessage()); q != "" {
			return "Let me put it another way: " + q
		}
	case moveRequestCode:
		return "When you're ready, please write the Python code for this task and paste it here. You're encouraged to use AI tools to help write it."
	case moveRedirectVague:
		return "Could you give me one concrete detail about how you'd handle this situation?"
	}
	if brief != nil && brief.Mode == mode && brief.Fallback != "" && move.Kind != moveCodeFollowUp {
		return brief.Fallback
	}
	switch {
	case move.Kind == moveCodeFollowUp && move.Target == dimAIUse:
		return "Did you use any AI tools while writing this code, and if so, how did you check what they produced?"
	case move.Kind == moveCodeFollowUp:
		return "Could you walk me through one line of your code and why you wrote it that way?"
	case mode == modeBug:
		return "What's the first thing you would check to narrow down where the problem is?"
	case state.isProblemDecompositionWeek():
		return fmt.Sprintf("What would you need to work out next to handle the %s part of this problem?", firstOr(decompositionPartsRemaining(state), "next"))
	default:
		return "Could you tell me a bit more about how you'd approach that?"
	}
}

func firstOr(items []string, fallback string) string {
	if len(items) > 0 {
		return items[0]
	}
	return fallback
}

// d5PendingQuestion is the interviewer's open question, repeated after a deferred
// coaching request.
func d5PendingQuestion(sess *d5Session) string {
	return strings.TrimSpace(lastQuestionSentence(sess.lastInterviewerMessage()))
}
