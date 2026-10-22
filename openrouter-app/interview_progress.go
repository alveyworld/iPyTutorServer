package main

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const (
	interviewStepOpening                 = "opening"
	interviewStepAwaitingAnswer          = "awaiting_answer"
	interviewStepInterviewing            = "interviewing"
	interviewStepDecompositionAnswered   = "decomposition_answered"
	interviewStepAwaitingCode            = "awaiting_code"
	interviewStepCodeSubmitted           = "code_submitted"
	interviewStepFollowUp                = "follow_up"
)

const week1ConceptualForwardPolicy = "Week 1 uses ONE scenario for the whole interview. Ask follow-ups about that same scenario only — never present a new scenario. Once the student's answers show their input, process, and output decomposition, finish with conceptualAssessmentPhase complete plus conceptualAssessmentBucket."

func resetModeInterviewProgress(state *AgentSessionState) {
	if state == nil {
		return
	}
	state.ModeInterviewStep = interviewStepOpening
	state.ModeOpeningServed = false
	state.ModeUserAnsweredSinceOpening = false
	state.ModeQuestionsAsked = nil
	state.ModeQuestionsBeforeCode = 0
	state.ModeVagueAnswers = 0
	state.ModeSimilarQuestionAsks = 0
}

func isFollowUpAssessmentTurn(state *AgentSessionState) bool {
	if state == nil || state.ConversationPhase != phaseAssessmentInProgress {
		return false
	}
	switch state.ActiveMode {
	case modeConceptual, modeCode, modeBug:
		return state.ModeOpeningServed && state.ModeUserAnsweredSinceOpening
	default:
		return false
	}
}

func advanceInterviewProgressOnUserAnswer(state *AgentSessionState) {
	if state == nil || !state.ModeOpeningServed {
		return
	}
	state.ModeUserAnsweredSinceOpening = true
	if isVagueAnswer(state.LastUserMessageRaw) {
		state.ModeVagueAnswers++
	}

	switch state.ActiveMode {
	case modeConceptual:
		if state.ModeInterviewStep == interviewStepOpening || state.ModeInterviewStep == interviewStepAwaitingAnswer {
			state.ModeInterviewStep = interviewStepInterviewing
		}
	case modeCode:
		switch state.ModeInterviewStep {
		case interviewStepOpening, interviewStepAwaitingAnswer:
			state.ModeInterviewStep = interviewStepDecompositionAnswered
		case interviewStepAwaitingCode:
			if looksLikeCodeSubmission(state.LastUserMessageRaw) {
				state.ModeInterviewStep = interviewStepCodeSubmitted
				state.ModeQuestionsBeforeCode = len(state.ModeQuestionsAsked)
			}
		}
	case modeBug:
		if state.ModeInterviewStep == interviewStepOpening || state.ModeInterviewStep == interviewStepAwaitingAnswer {
			state.ModeInterviewStep = interviewStepFollowUp
		}
	}
}

func updateInterviewProgressAfterAssistant(state *AgentSessionState, assistant string) {
	// Do not require a sync block: a delivered opening without one must still count as
	// served, or every later turn is treated as the opening and rewound to the scenario.
	if state == nil || state.ConversationPhase != phaseAssessmentInProgress || clientVisibleAssistantContent(assistant) == "" {
		return
	}
	switch state.ActiveMode {
	case modeConceptual, modeCode, modeBug:
		if !state.ModeOpeningServed {
			state.ModeOpeningServed = true
			state.ModeInterviewStep = interviewStepAwaitingAnswer
		}
		if q := lastQuestionSentence(clientVisibleAssistantContentGuarded(assistant, state)); q != "" {
			// A rephrasing the student asked for is not the interview circling the same ground.
			// Also, follow-up questions in the opening turn should not be counted as repeats of the opening
			// question, since asking "what's the value?" after asking "what's the type?" is elaboration, not repetition.
			if isFollowUpAssessmentTurn(state) && repeatsRecordedQuestion(state, q) && !studentAskedForClarification(state.LastUserMessageRaw) {
				state.ModeSimilarQuestionAsks++
			}
			state.ModeQuestionsAsked = append(state.ModeQuestionsAsked, q)
		}
		if state.ActiveMode == modeCode {
			advanceCodeInterviewStepFromAssistant(state, assistant)
		}
	}
}

func advanceCodeInterviewStepFromAssistant(state *AgentSessionState, assistant string) {
	visible := strings.ToLower(strings.TrimSpace(stripIPyIntervuTail(assistant)))
	switch state.ModeInterviewStep {
	case interviewStepDecompositionAnswered, interviewStepInterviewing:
		if looksLikeCodeRequest(visible) {
			state.ModeInterviewStep = interviewStepAwaitingCode
		}
	}
}

func looksLikeCodeRequest(text string) bool {
	for _, phrase := range []string{
		"paste your code",
		"paste the code",
		"share your code",
		"send your code",
		"submit your code",
		"provide your code",
		"paste your solution",
		"paste your python",
		"share your implementation",
		"paste it here",
	} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return codeRequestPattern.MatchString(text)
}

// codeRequestPattern catches wordings the phrase list misses, such as "please paste the
// Python code you wrote". "pasted"/"shared" do not match, so "the code you pasted" is not a request.
var codeRequestPattern = regexp.MustCompile(`(?i)\b(paste|share|send|submit|provide)\s+(me\s+)?(your|the|that|it)?\s*(python\s+)?(code|solution|script)\b`)

func looksLikeCodeSubmission(userMessage string) bool {
	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		return false
	}
	if strings.Contains(msg, "```") {
		return true
	}
	lower := strings.ToLower(msg)
	for _, token := range []string{"def ", "import ", "print(", "for ", "while ", "if ", "input("} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return len(msg) > 120 && strings.Count(msg, "\n") >= 2
}

func forwardInterviewMoveGuidance(state *AgentSessionState) string {
	if state == nil {
		return "Continue with one appropriate interview question for the active mode."
	}
	switch state.ActiveMode {
	case modeCode:
		switch state.ModeInterviewStep {
		case interviewStepDecompositionAnswered, interviewStepInterviewing, interviewStepFollowUp:
			return "The student already answered decomposition. Your next move MUST ask them to paste their Python code for the task (e.g. 'Please paste your Python code.'). Do not repeat the opening decomposition question and do not send complete until pasted code is received and assessed."
		case interviewStepAwaitingCode:
			return "You asked for code. Wait for pasted Python code if not yet received — do not repeat decomposition. After paste, evaluate code and ask explain-code or AI-use questions before complete."
		case interviewStepCodeSubmitted:
			return "The student already pasted their code — never ask for it again. Ask one explain-code, line-level, or AI-use reflection question — do not repeat decomposition. Once the student has explained the code and answered an AI-use question, finish with codeAssessmentPhase complete plus codeAssessmentBucket."
		default:
			return "Continue the code interview with the next step (code request or explain-code). Do not repeat the opening decomposition question."
		}
	case modeBug:
		if state.ModeInterviewStep == interviewStepFollowUp || state.ModeInterviewStep == interviewStepInterviewing {
			return "Continue bug hunting with one new debugging-process follow-up question. Do not repeat the opening scenario question verbatim."
		}
		return "Continue bug hunting with one debugging-process question. Do not repeat the opening scenario question verbatim."
	case modeConceptual:
		if state.isProblemDecompositionWeek() {
			if remaining := decompositionPartsRemaining(state); isFollowUpAssessmentTurn(state) && len(remaining) > 0 {
				return week1ConceptualForwardPolicy + " Parts the student has not been asked about yet: " + strings.Join(remaining, ", ") + "."
			}
			return week1ConceptualForwardPolicy
		}
		return "Continue with one new conceptual interview question. Do not repeat the previous question verbatim."
	default:
		return "Continue with one interview question appropriate to the active mode."
	}
}

func interviewProgressSnapshot(state *AgentSessionState) map[string]any {
	if state == nil || state.ConversationPhase != phaseAssessmentInProgress {
		return nil
	}
	switch state.ActiveMode {
	case modeConceptual, modeCode, modeBug:
	default:
		return nil
	}
	snap := map[string]any{
		"step":                      state.ModeInterviewStep,
		"openingServed":             state.ModeOpeningServed,
		"userAnsweredSinceOpening": state.ModeUserAnsweredSinceOpening,
	}
	if len(state.ModeQuestionsAsked) > 0 {
		snap["questionsAsked"] = state.ModeQuestionsAsked
	}
	snap["vagueAnswers"] = state.ModeVagueAnswers
	snap["similarQuestionAsks"] = state.ModeSimilarQuestionAsks
	snap["repetitionPolicy"] = "Do not cover ground an earlier question in this mode already covered: the same question, in any wording, may be put to the student at most twice in the whole mode. If the student answers vaguely twice, stop asking — close the mode with the bucket their answers support (vague or no strategy is Not Ready Yet) instead of rephrasing the question to fish for a better answer."
	if state.ActiveMode == modeConceptual && state.isProblemDecompositionWeek() {
		snap["decompositionPartsAnswered"] = decompositionPartsAnswered(state)
	}
	switch state.ActiveMode {
	case modeCode:
		if state.ModeInterviewStep == interviewStepCodeSubmitted {
			snap["forwardPolicy"] = "The student has pasted their code — never ask for it again. Ask explain-code and AI-use questions, then finish with codeAssessmentPhase complete plus codeAssessmentBucket."
		} else {
			snap["forwardPolicy"] = "After decomposition is answered, explicitly ask the student to paste their Python code — then evaluate pasted code before complete. Never repeat the opening decomposition ask or complete after decomposition alone."
		}
	case modeBug:
		snap["forwardPolicy"] = fmt.Sprintf("After the opening debug question is answered, ask follow-up debugging-process questions that open new ground (a different tool, assumption, or narrowing step) — never repeat the opening question or circle an earlier one. Ask at most %d questions in this mode including the opening one; %d have been asked. Then close with bugAssessmentPhase complete plus bugAssessmentBucket.", maxBugQuestions, len(state.ModeQuestionsAsked))
	case modeConceptual:
		if state.isProblemDecompositionWeek() {
			snap["forwardPolicy"] = week1ConceptualForwardPolicy
		} else {
			snap["forwardPolicy"] = "Ask new conceptual follow-ups; do not repeat the same question verbatim."
		}
	}
	return snap
}

// followUpTurnDirective states the interview position plainly for this turn. The
// interviewProgress flags alone did not stop the model from re-introducing a persona,
// re-asking answered questions, or interviewing past the point it should close.
func followUpTurnDirective(state *AgentSessionState) string {
	if !isFollowUpAssessmentTurn(state) {
		return ""
	}
	if modeClosingDue(state) {
		phaseField, bucketField, _ := currentModeSyncFields(state)
		return "THIS TURN MUST CLOSE THE INTERVIEW: " + closingDueReason(state) +
			". Ask no question, do not rephrase an earlier question, and do not ask for code. Send one brief neutral closing sentence and end with ```_ipyintervu``` containing \"" + phaseField + "\": \"complete\" and \"" +
			bucketField + "\" (Not Ready Yet, Competent, or Exceptional) based on all of the student's answers — vague or absent answers are Not Ready Yet.\n"
	}
	return "THIS TURN: the scenario is already on screen and the student's latest message answers your previous question. The student has already answered: " +
		quotedQuestions(state.ModeQuestionsAsked) +
		". Do not welcome the student again, restate the scenario, or ask any of those questions again in any wording (unless the student asked you to clarify one). A persona speaking for the first time gives at most one short clause of introduction. " +
		forwardInterviewMoveGuidance(state) + "\n"
}

func quotedQuestions(questions []string) string {
	if len(questions) == 0 {
		return "your previous question"
	}
	quoted := make([]string, len(questions))
	for i, q := range questions {
		quoted[i] = fmt.Sprintf("%q", q)
	}
	return strings.Join(quoted, "; ")
}

// maxConceptualQuestions is the top of the conceptual mode's 3–5 question guideline.
const maxConceptualQuestions = 5

var decompositionParts = []string{"input", "process", "output"}

// decompositionPart classifies a Week 1 question by the first part it names, so
// "what is the process — the steps from those inputs to a result?" counts as process.
func decompositionPart(question string) string {
	lower := strings.ToLower(question)
	best, bestIdx := "", -1
	for part, keywords := range map[string][]string{
		"input":   {"input"},
		"process": {"process", "steps"},
		"output":  {"output"},
	} {
		for _, kw := range keywords {
			if idx := strings.Index(lower, kw); idx >= 0 && (bestIdx < 0 || idx < bestIdx) {
				best, bestIdx = part, idx
			}
		}
	}
	return best
}

// decompositionPartsAnswered lists the Week 1 parts the student has been asked about. Every
// recorded question has been answered by the time a follow-up turn is built.
func decompositionPartsAnswered(state *AgentSessionState) []string {
	asked := map[string]bool{}
	for _, q := range state.ModeQuestionsAsked {
		asked[decompositionPart(q)] = true
	}
	var answered []string
	for _, part := range decompositionParts {
		if asked[part] {
			answered = append(answered, part)
		}
	}
	return answered
}

func decompositionPartsRemaining(state *AgentSessionState) []string {
	answered := strings.Join(decompositionPartsAnswered(state), " ")
	var remaining []string
	for _, part := range decompositionParts {
		if !strings.Contains(answered, part) {
			remaining = append(remaining, part)
		}
	}
	return remaining
}

// conceptualClosingDue reports that this conceptual turn must be the closing reply: Week 1
// once input, process, and output are all answered, any week after the question cap.
func conceptualClosingDue(state *AgentSessionState) bool {
	if state.ActiveMode != modeConceptual || !isFollowUpAssessmentTurn(state) || studentAskedForClarification(state.LastUserMessageRaw) {
		return false
	}
	if state.isProblemDecompositionWeek() && len(decompositionPartsRemaining(state)) == 0 {
		return true
	}
	return len(state.ModeQuestionsAsked) >= maxConceptualQuestions
}

// maxPostCodeQuestions caps the explain-code and AI-use questions asked after the paste.
const maxPostCodeQuestions = 3

// postCodeQuestions lists the questions asked since the student pasted their code.
func postCodeQuestions(state *AgentSessionState) []string {
	if state.ModeQuestionsBeforeCode > len(state.ModeQuestionsAsked) {
		return nil
	}
	return state.ModeQuestionsAsked[state.ModeQuestionsBeforeCode:]
}

var aiUsePattern = regexp.MustCompile(`(?i)\b(ai|a\.i\.|chatgpt|copilot|gemini|claude|llm)\b|artificial intelligence`)

func askedAboutAIUse(questions []string) bool {
	for _, q := range questions {
		if aiUsePattern.MatchString(q) {
			return true
		}
	}
	return false
}

// codeClosingDue reports that this code turn must be the closing reply: the student has
// pasted code and answered an explain-code question plus an AI-use question, or the cap.
// Without it the model kept interviewing and eventually asked for the code again.
func codeClosingDue(state *AgentSessionState) bool {
	if state.ActiveMode != modeCode || state.ModeInterviewStep != interviewStepCodeSubmitted ||
		!isFollowUpAssessmentTurn(state) || studentAskedForClarification(state.LastUserMessageRaw) {
		return false
	}
	asked := postCodeQuestions(state)
	return len(asked) >= maxPostCodeQuestions || (len(asked) >= 2 && askedAboutAIUse(asked))
}

// maxBugQuestions caps Bug Hunting at the opening scenario question plus three debugging
// follow-ups. Without a cap the mode had no finish condition at all, and the model kept
// circling the same snippet with slight variations of questions it had already asked.
const maxBugQuestions = 4

// bugClosingDue reports that this bug turn must be the closing reply: the student has
// answered the opening debug question and the mode has hit its question cap.
func bugClosingDue(state *AgentSessionState) bool {
	if state.ActiveMode != modeBug || !isFollowUpAssessmentTurn(state) || studentAskedForClarification(state.LastUserMessageRaw) {
		return false
	}
	return len(state.ModeQuestionsAsked) >= maxBugQuestions
}

// maxVagueAnswers: two answers with nothing to assess end the mode. Re-asking after a
// vague answer produced the reported loop — the student hedged, the interviewer rephrased,
// and neither the evidence nor the bucket changed. Two vague answers are themselves the
// evidence (see Bug assessment criteria: vague or no strategy → Not Ready Yet).
const maxVagueAnswers = 2

// maxSimilarQuestionAsks caps how many times the interview may put essentially the same
// question to the student: the first ask plus one re-ask. Once one has been asked twice,
// asking around it again yields nothing, so the mode closes on the answers it has.
const maxSimilarQuestionAsks = 2

// overfitLimitsApply gates the vagueness and repetition caps. Code Problem mode cannot
// close before the student pastes code — that paste is the mode's required evidence — so
// there the caps bind only after the submission.
func overfitLimitsApply(state *AgentSessionState) bool {
	if !isFollowUpAssessmentTurn(state) {
		return false
	}
	return state.ActiveMode != modeCode || state.ModeInterviewStep == interviewStepCodeSubmitted
}

func vagueAnswerLimitReached(state *AgentSessionState) bool {
	return overfitLimitsApply(state) && state.ModeVagueAnswers >= maxVagueAnswers
}

func similarQuestionLimitReached(state *AgentSessionState) bool {
	return overfitLimitsApply(state) && state.ModeSimilarQuestionAsks >= maxSimilarQuestionAsks-1
}

// modeClosingDue reports that the active mode's interview is finished and this turn must close it.
func modeClosingDue(state *AgentSessionState) bool {
	return vagueAnswerLimitReached(state) || similarQuestionLimitReached(state) ||
		conceptualClosingDue(state) || codeClosingDue(state) || bugClosingDue(state)
}

func closingDueReason(state *AgentSessionState) string {
	if vagueAnswerLimitReached(state) {
		return fmt.Sprintf("the student has answered vaguely %d times, so further questions will not produce better evidence", state.ModeVagueAnswers)
	}
	if similarQuestionLimitReached(state) {
		return "the same question has already been asked twice"
	}
	if state.ActiveMode == modeCode {
		return fmt.Sprintf("code pasted and %d questions about it answered", len(postCodeQuestions(state)))
	}
	if state.isProblemDecompositionWeek() && len(decompositionPartsRemaining(state)) == 0 {
		return "input, process, and output have all been answered"
	}
	return fmt.Sprintf("%d questions asked", len(state.ModeQuestionsAsked))
}

// lastQuestionSentence returns the final question sentence of visible assistant text.
func lastQuestionSentence(visible string) string {
	visible = strings.TrimSpace(stripCodeFences(visible))
	end := strings.LastIndex(visible, "?")
	if end < 0 {
		return ""
	}
	start := strings.LastIndexAny(visible[:end], ".!?\n")
	return strings.TrimSpace(visible[start+1 : end+1])
}

var questionStopWords = map[string]bool{
	"what": true, "would": true, "you": true, "your": true, "identify": true, "the": true, "this": true,
	"that": true, "these": true, "those": true, "for": true, "and": true, "any": true, "are": true,
	"can": true, "could": true, "how": true, "which": true, "when": true, "where": true, "why": true,
	"with": true, "from": true, "into": true, "about": true, "does": true, "did": true, "should": true,
	"might": true, "will": true, "there": true, "here": true, "have": true, "has": true, "our": true,
	"we": true, "consider": true, "think": true, "say": true, "describe": true, "explain": true,
	"begin": true, "begins": true, "start": true, "starts": true, "task": true, "then": true,
}

func questionContentWords(question string) map[string]bool {
	words := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(question), func(r rune) bool { return !unicode.IsLetter(r) }) {
		if len(w) < 3 || questionStopWords[w] {
			continue
		}
		for _, suffix := range []string{"ing", "es", "ed", "s"} {
			if len(w) > len(suffix)+3 && strings.HasSuffix(w, suffix) {
				w = strings.TrimSuffix(w, suffix)
				break
			}
		}
		words[w] = true
	}
	return words
}

// isRepeatedQuestion reports whether next asks essentially the same thing as prev: at least
// two shared content words covering most of the shorter question. "What is the input?" and
// "What is the output?" share none, so asking for the next decomposition part is not a repeat.
func isRepeatedQuestion(prev, next string) bool {
	a, b := questionContentWords(prev), questionContentWords(next)
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	shared := 0
	for w := range a {
		if b[w] {
			shared++
		}
	}
	smaller := len(a)
	if len(b) < smaller {
		smaller = len(b)
	}
	return shared >= 2 && float64(shared)/float64(smaller) >= 0.6
}

var clarificationPhrases = []string{"don't understand", "dont understand", "not sure what you mean", "confused", "clarify", "rephrase"}

func studentAskedForClarification(userMessage string) bool {
	lower := strings.ToLower(userMessage)
	if strings.Contains(lower, "?") {
		return true
	}
	return containsAny(lower, clarificationPhrases)
}

func containsAny(text string, phrases []string) bool {
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

// vagueAnswerPhrases mark an answer that declines to commit to anything assessable.
var vagueAnswerPhrases = []string{
	"i don't know", "i dont know", "don't know", "dont know", "no idea", "not sure",
	"unsure", "no clue", "idk", "dunno", "i guess", "maybe", "whatever", "beats me",
	"can't think", "cant think", "nothing comes to mind", "hard to say", "who knows",
}

// vagueHedgeWordLimit: a hedge inside a substantial answer ("I don't know the cause, but
// I would add a print before the loop and compare the counter to the intended total")
// is thinking out loud, not a non-answer — only short hedged replies count as vague.
const vagueHedgeWordLimit = 12

// vagueBareWordLimit: an answer this short ("print statements", "not really") states no
// strategy the rubric can score, whatever words it uses.
const vagueBareWordLimit = 3

// isVagueAnswer reports an answer that gives the interviewer nothing to assess. A request
// to have the question explained is not an answer at all, so it never counts as vague.
func isVagueAnswer(userMessage string) bool {
	trimmed := strings.TrimSpace(userMessage)
	if trimmed == "" {
		return true
	}
	if looksLikeCodeSubmission(trimmed) {
		return false
	}
	lower := strings.ToLower(trimmed)
	if containsAny(lower, clarificationPhrases) {
		return false
	}
	words := len(strings.Fields(lower))
	if words <= vagueBareWordLimit {
		return true
	}
	return words <= vagueHedgeWordLimit && containsAny(lower, vagueAnswerPhrases)
}

// repeatsAnsweredQuestion reports a follow-up reply that re-asks any question the student
// already answered in this mode (e.g. asking for the input again after the output).
func repeatsAnsweredQuestion(state *AgentSessionState, assistant string) bool {
	if state.ActiveMode == modeCode && state.ModeInterviewStep == interviewStepCodeSubmitted && isFollowUpAssessmentTurn(state) &&
		looksLikeCodeRequest(strings.ToLower(clientVisibleAssistantContent(assistant))) {
		return true
	}
	// Code mode legitimately re-asks for code the student has not pasted yet.
	if (state.ActiveMode == modeCode && state.ModeInterviewStep != interviewStepCodeSubmitted) || !isFollowUpAssessmentTurn(state) || len(state.ModeQuestionsAsked) == 0 || studentAskedForClarification(state.LastUserMessageRaw) {
		return false
	}
	candidates := studentDirectedInterviewQuestions(assistant)
	if next := lastQuestionSentence(clientVisibleAssistantContent(assistant)); next != "" {
		candidates = append(candidates, next)
	}
	for _, q := range candidates {
		if repeatsRecordedQuestion(state, q) {
			return true
		}
	}
	return false
}

// repeatsRecordedQuestion reports whether question covers the same ground as one already
// asked in this mode.
func repeatsRecordedQuestion(state *AgentSessionState, question string) bool {
	for _, prev := range state.ModeQuestionsAsked {
		if isRepeatedQuestion(prev, question) {
			return true
		}
	}
	return false
}
