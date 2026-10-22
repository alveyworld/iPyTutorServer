package main

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	simulatedStudentPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(student|you|candidate|applicant|user)\s*:\s*`),
		regexp.MustCompile(`(?i)\b(your answer(?: would be)?|the answer is|correct answer|expected answer|sample answer)\s*:`),
	}
	selfAnswerPhrasePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(this means|that means|it works by|works by|you would|you could|you should|you'd|so you can|for example)\b`),
		regexp.MustCompile(`(?i)\b(in python,?|append adds|extend adds|the (?:bug|issue|problem|defect|fix) is)\b`),
		regexp.MustCompile(`(?i)\b(that's because|because it|the reason is|so the (?:list|code|program|output))\b`),
		regexp.MustCompile(`(?i)^(?:well|so|basically|simply put|in short),?\s+`),
		regexp.MustCompile(`(?i)^(?:a|an|the)\s+[a-z][a-z\s]{0,40}\s+(?:is|are|means|refers to|stores|holds|adds|removes|iterates|returns)\b`),
		regexp.MustCompile(`(?i)^[a-z][a-z\s]{0,30}\s+(?:is|are|lets you|allows you|enables you|adds|removes|stores|holds|means|refers to)\b`),
	}
	codeFencePattern = regexp.MustCompile("(?s)```.*?```")
)

func questionGuardEnabledForState(state *AgentSessionState) bool {
	if state == nil {
		return false
	}
	if state.ConversationPhase != phaseAssessmentInProgress {
		return false
	}
	if state.CoachingRequested && state.ActiveMode == modeCoaching {
		return false
	}
	if isActiveModeComplete(state) {
		return false
	}
	switch state.ActiveMode {
	case modeConceptual, modeCode, modeBug:
		return true
	default:
		return false
	}
}

func findSimulatedStudentIndex(text string) int {
	best := -1
	for _, pattern := range simulatedStudentPatterns {
		if loc := pattern.FindStringIndex(text); loc != nil && (best < 0 || loc[0] < best) {
			best = loc[0]
		}
	}
	return best
}

func stripCodeFences(text string) string {
	return strings.TrimSpace(codeFencePattern.ReplaceAllString(text, " "))
}

func countSentences(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	count := 0
	start := 0
	for i, r := range text {
		if r == '.' || r == '!' || r == '?' {
			segment := strings.TrimSpace(text[start : i+1])
			if segment != "" {
				count++
			}
			start = i + 1
		}
	}
	if tail := strings.TrimSpace(text[start:]); tail != "" {
		count++
	}
	return count
}

func isLikelyAnswerContent(after string) bool {
	after = strings.TrimSpace(after)
	if after == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(after), "for example") {
		return true
	}
	if len(after) < 20 {
		return false
	}
	if findSimulatedStudentIndex(after) >= 0 {
		return true
	}
	if strings.Contains(after, "```") {
		return true
	}
	lower := strings.ToLower(after)
	for _, pattern := range selfAnswerPhrasePatterns {
		if pattern.MatchString(after) {
			return true
		}
	}
	if strings.Contains(lower, "the answer") || strings.Contains(lower, "you would ") {
		return true
	}
	if isSimulatedStudentAnswerLine(after) {
		return true
	}
	if countSentences(after) >= 2 && !strings.Contains(after, "?") {
		return true
	}
	runes := []rune(after)
	if len(runes) >= 80 && !strings.HasSuffix(strings.TrimSpace(after), "?") {
		first := runes[0]
		if unicode.IsUpper(first) && countSentences(after) >= 1 {
			for _, pattern := range selfAnswerPhrasePatterns {
				if pattern.MatchString(after) {
					return true
				}
			}
			if strings.Contains(lower, " is ") || strings.Contains(lower, " are ") {
				return true
			}
		}
	}
	return false
}

func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func isSimulatedStudentAnswerLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" || strings.Contains(line, "?") {
		return false
	}
	if len(line) < 12 {
		return false
	}
	lower := strings.ToLower(line)
	if strings.HasPrefix(lower, "good point") ||
		strings.HasPrefix(lower, "thanks") ||
		strings.HasPrefix(lower, "got it") ||
		strings.HasPrefix(lower, "understood") ||
		strings.HasPrefix(lower, "okay") ||
		strings.HasPrefix(lower, "ok ") ||
		strings.HasPrefix(lower, "to clarify") {
		return false
	}
	if simulatedStudentAnswerLinePattern.MatchString(line) {
		return true
	}
	if simulatedStudentShortAnswerPattern.MatchString(line) {
		return true
	}
	if first, _ := utf8.DecodeRuneInString(line); first != utf8.RuneError && unicode.IsLower(first) {
		return true
	}
	return false
}

func hasSimulatedStudentLineBetweenQuestions(visible string) bool {
	visible = strings.TrimSpace(stripCodeFences(stripIPyIntervuTail(visible)))
	if visible == "" {
		return false
	}
	parts := strings.Split(visible, "?")
	if len(parts) < 2 {
		return false
	}
	for i := 0; i < len(parts)-1; i++ {
		between := strings.TrimSpace(parts[i+1])
		if between == "" {
			continue
		}
		if isSimulatedStudentAnswerLine(firstNonEmptyLine(between)) {
			return true
		}
	}
	return false
}

func findQuestionAnswerBoundaryInClean(clean string) int {
	searchFrom := 0
	for {
		rel := strings.Index(clean[searchFrom:], "?")
		if rel < 0 {
			return -1
		}
		qIdx := searchFrom + rel
		after := strings.TrimSpace(clean[qIdx+1:])
		if after == "" {
			searchFrom = qIdx + 1
			continue
		}
		nextQRel := strings.Index(after, "?")
		between := after
		if nextQRel >= 0 {
			between = strings.TrimSpace(after[:nextQRel])
		}
		if isLikelyAnswerContent(between) || isSimulatedStudentAnswerLine(firstNonEmptyLine(between)) {
			return qIdx
		}
		if nextQRel < 0 {
			return -1
		}
		searchFrom = qIdx + 1 + nextQRel + 1
	}
}

// looksLikeSelfAnsweredQuestion reports whether visible assistant text asks a question
// and then continues with answer-like content before the student responds.
func looksLikeSelfAnsweredQuestion(visible string) bool {
	visible = strings.TrimSpace(visible)
	if visible == "" {
		return false
	}
	if findSimulatedStudentIndex(visible) >= 0 {
		return true
	}
	if hasSimulatedStudentLineBetweenQuestions(visible) {
		return true
	}
	return findQuestionAnswerBoundaryInClean(stripCodeFences(visible)) >= 0
}

// guardQuestionOnlyResponse truncates visible assistant text so only the question remains.
func guardQuestionOnlyResponse(visible string) string {
	visible = strings.TrimSpace(visible)
	if visible == "" {
		return visible
	}
	if idx := findSimulatedStudentIndex(visible); idx >= 0 {
		return strings.TrimRight(visible[:idx], " \t\n\r")
	}
	clean := stripCodeFences(visible)
	if idx := findQuestionAnswerBoundaryInClean(clean); idx >= 0 {
		return strings.TrimRight(clean[:idx+1], " \t\n\r")
	}
	return visible
}

func guardAssessmentContentResponse(visible string) string {
	visible = strings.TrimSpace(guardQuestionOnlyResponse(visible))
	if countStudentDirectedQuestions(visible) <= 1 {
		return visible
	}
	questions := studentDirectedInterviewQuestions(visible)
	if len(questions) == 0 {
		return visible
	}
	first := questions[0]
	if idx := strings.Index(visible, first); idx >= 0 {
		return strings.TrimRight(visible[:idx+len(first)], " \t\n\r")
	}
	return visible
}

func clientVisibleAssistantContentGuarded(accumulated string, state *AgentSessionState) string {
	visible := clientVisibleAssistantContent(accumulated)
	if !questionGuardEnabledForState(state) {
		return visible
	}
	return guardAssessmentContentResponse(visible)
}

func countStudentDirectedQuestions(visible string) int {
	return len(studentDirectedInterviewQuestions(visible))
}

func countInterviewQuestions(visible string) int {
	return countStudentDirectedQuestions(visible)
}

var studentDirectedInterviewQuestionPattern = regexp.MustCompile(`(?i)\b(?:what would you|how would you|how might you|what might you|what (?:do you|would you|are|is)|how (?:do you|would you|might you|are|is)|which|where would you|when would you|would you|can you|could you|do you|are you|identify|describe|explain|list|name|consider)\b[^.?\n]{0,200}\?`)

func studentDirectedInterviewQuestions(visible string) []string {
	visible = stripCodeFences(stripIPyIntervuTail(visible))
	return studentDirectedInterviewQuestionPattern.FindAllString(visible, -1)
}

var simulatedStudentAnswerLinePattern = regexp.MustCompile(`(?i)^(a|an|the)\s+(table|list|report|chart|summary|output|result|spreadsheet|document|file|map|set)\b`)

var simulatedStudentShortAnswerPattern = regexp.MustCompile(`(?i)^(the )?(type|value|result|answer|output) (would be|is|was)\b`)

var neutralAssessmentLeadInPattern = regexp.MustCompile(`(?i)(?:^|\n)\s*(?:got it\.|thanks\.|understood\.|okay\.)\s`)

// looksLikeCompositeAssessmentReply reports stacked interview content in one reply.
func looksLikeCompositeAssessmentReply(visible string) bool {
	visible = strings.TrimSpace(stripIPyIntervuTail(visible))
	if visible == "" {
		return false
	}
	if hasSimulatedStudentLineBetweenQuestions(visible) {
		return true
	}
	return countStudentDirectedQuestions(visible) > 1
}

// looksLikeSevereCompositeReply reports stacked mini-interviews: simulated student
// answers, multiple questions, or multiple neutral lead-ins in one reply.
func looksLikeSevereCompositeReply(visible string) bool {
	visible = strings.TrimSpace(stripCodeFences(stripIPyIntervuTail(visible)))
	if visible == "" {
		return false
	}
	if hasSimulatedStudentLineBetweenQuestions(visible) {
		return true
	}
	if countStudentDirectedQuestions(visible) > 1 {
		return true
	}
	return countNeutralAssessmentLeadIns(visible) > 1
}

func countNeutralAssessmentLeadIns(text string) int {
	return len(neutralAssessmentLeadInPattern.FindAllStringIndex(text, -1))
}

func isCorrectiveFollowUpKind(kind string) bool {
	return kind == "corrective_retry"
}

func buildDisplayAssistantRaw(handoffParts []string, lastAssistant string) string {
	if len(handoffParts) == 0 {
		return lastAssistant
	}
	if strings.TrimSpace(lastAssistant) == "" {
		return strings.Join(handoffParts, "\n\n")
	}
	return strings.Join(handoffParts, "\n\n") + "\n\n" + lastAssistant
}

var (
	evaluativePraisePattern     = regexp.MustCompile(`(?i)\b(?:that's|that is) (?:exactly |absolutely )?(?:right|correct)\b|\bexactly right\b|\byou're (?:absolutely |exactly )?right\b|\byou are (?:absolutely |exactly )?right\b|\bwell done\b|\bgreat (?:job|answer|work)\b|\bnice work\b|\bthat's a (?:clear|solid|good|great|strong|nice)\b|\bis a (?:clear|solid|good|great|strong|nice) (?:approach|answer|start|idea|plan)\b|(?:^|\n)\s*(?:exactly|correct|confirmed)\b|\blines up with\b`)
	answerExplanationPattern    = regexp.MustCompile(`(?i)\blet's (?:step|walk) through\b|\bfor example\b|\bthe (?:correct|right|expected) answer\b`)
	paragraphBreakPattern       = regexp.MustCompile(`\n\s*\n`)
	acknowledgmentOpenerPattern = regexp.MustCompile(`(?i)^(?:got it|thanks|thank you|understood|okay|ok)\b`)
)

// looksLikeEvaluationOrStagedTurn reports replies that grade or explain the student's answer,
// or that act out another exchange: an acknowledgment opening a later paragraph means the
// model wrote a reply to an answer the student never gave. None of these depend on a
// question mark, so they catch explanations placed before the reply's only question.
func looksLikeEvaluationOrStagedTurn(visible string) bool {
	visible = strings.TrimSpace(stripCodeFences(stripIPyIntervuTail(visible)))
	if visible == "" {
		return false
	}
	if evaluativePraisePattern.MatchString(visible) || answerExplanationPattern.MatchString(visible) {
		return true
	}
	for i, paragraph := range paragraphBreakPattern.Split(visible, -1) {
		if i > 0 && acknowledgmentOpenerPattern.MatchString(strings.TrimSpace(paragraph)) {
			return true
		}
	}
	return false
}

var interviewAskPattern = regexp.MustCompile(`(?i)\?|\bplease (?:paste|share|send|provide|submit)\b|\bwalk me through\b|\btell me\b|\bpoint me to\b|\b(?:explain|describe|outline|share|show me)\b[^.!\n]{0,40}\byour\b|\bbreak\b[^.!\n]{0,40}\binto\b`)

const personaNameAlt = `alex|julia|taylor|morgan|riley|casey|samantha|david`

var (
	replySentencePattern = regexp.MustCompile(`[^.!?\n]+[.!?]*`)
	// "Alex, would you like to begin?" / "Would you like to begin, Julia?"
	personaAddressedPattern    = regexp.MustCompile(`(?i)^\s*(?:so|and|now|ok(?:ay)?)?[,\s]*(?:` + personaNameAlt + `)\s*[,:—–-]|[,—–-]\s*(?:` + personaNameAlt + `)\s*[?.!]*\s*$`)
	templatePlaceholderPattern = regexp.MustCompile(`\[[A-Za-z][A-Za-z ]{1,30}\]`)
)

// asksStudentSomething reports whether a reply contains a question or a direct request for
// the student. Sentences addressed to a persona ("Alex, would you like to begin?") do not
// count: the student cannot answer them.
var (
	// "Hey Taylor!" / "Thanks, Alex." — greeting an interviewer as if the student were one.
	personaGreetingPattern = regexp.MustCompile(`(?i)^\s*(?:hey|hi|hello|thanks|thank you)[,\s]+(?:` + personaNameAlt + `)\s*[,.!:—–-]`)
	// Grading notes: the model reasoning about the student instead of talking to them.
	reasoningLeakPattern = regexp.MustCompile(`(?i)\bthe student(?:'s)?\b|\brubric\b|\bsync block\b|\b(?:conceptual|code|bug[- ]hunting) portion\b`)
)

// addressesPersona reports a sentence that greets or questions an interviewer instead of
// the student.
func addressesPersona(visible string) bool {
	for _, sentence := range replySentencePattern.FindAllString(stripCodeFences(stripIPyIntervuTail(visible)), -1) {
		if personaGreetingPattern.MatchString(sentence) || (strings.Contains(sentence, "?") && personaAddressedPattern.MatchString(sentence)) {
			return true
		}
	}
	return false
}

// leaksReasoning reports grading notes or internal vocabulary in a reply meant for the student.
func leaksReasoning(visible string) bool {
	return reasoningLeakPattern.MatchString(stripCodeFences(stripIPyIntervuTail(visible)))
}

func asksStudentSomething(visible string) bool {
	for _, sentence := range replySentencePattern.FindAllString(stripCodeFences(stripIPyIntervuTail(visible)), -1) {
		if !personaAddressedPattern.MatchString(sentence) && interviewAskPattern.MatchString(sentence) {
			return true
		}
	}
	return false
}

// hasUnfilledPlaceholder reports template placeholders such as "[companyName]" left in the
// prose of a reply. Markdown links ("[text](url)") and code are not placeholders.
func hasUnfilledPlaceholder(visible string) bool {
	prose := inlineCodePattern.ReplaceAllString(stripCodeFences(stripIPyIntervuTail(visible)), " ")
	for _, loc := range templatePlaceholderPattern.FindAllStringIndex(prose, -1) {
		if loc[1] < len(prose) && prose[loc[1]] == '(' {
			continue
		}
		return true
	}
	return false
}

var (
	// Classroom framing: course structure and what the student has been taught.
	classroomFramingPattern = regexp.MustCompile(`(?i)\b(?:in class|this class|our class|the course|this course|cse ?\d+|homework|syllabus|lectures?|module \d+|weeks? \d+|you(?:'ve| have) (?:learned|studied|been taught|covered)|we(?:'ve| have) (?:learned|covered)|what you know from)\b`)
	// Instructor roles; allowed for education majors (see Persona identity in protocols).
	instructorRolePattern = regexp.MustCompile(`(?i)\b(?:instructors?|teachers?|professors?|tutors?|teaching assistants?)\b`)
	educationMajorPattern = regexp.MustCompile(`(?i)educat|teach`)
)

func isEducationMajor(major string) bool {
	return educationMajorPattern.MatchString(major)
}

// speaksAsInstructor reports a reply framed as a class rather than a job interview: course
// weeks, homework, what the student has learned, or an instructor role.
func speaksAsInstructor(state *AgentSessionState, visible string) bool {
	prose := stripCodeFences(stripIPyIntervuTail(visible))
	if classroomFramingPattern.MatchString(prose) {
		return true
	}
	return !isEducationMajor(state.StudentMajor) && instructorRolePattern.MatchString(prose)
}
