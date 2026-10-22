package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ratingValueAlt    = `Not Ready Yet|Not Yet Ready|Weak|Competent|Looking Good|Strong|Good|Solid|Exceptional|Looks Great|Excellent|Outstanding|N/A|na`
	proseRatingLabels = ratingValueAlt

	bucketPatterns = map[string]*regexp.Regexp{
		"conceptualAssessmentBucket": regexp.MustCompile(`(?i)conceptualAssessmentBucket\s*[=:]\s*(` + ratingValueAlt + `)`),
		"codeAssessmentBucket":       regexp.MustCompile(`(?i)codeAssessmentBucket\s*[=:]\s*(` + ratingValueAlt + `)`),
		"bugAssessmentBucket":        regexp.MustCompile(`(?i)bugAssessmentBucket\s*[=:]\s*(` + ratingValueAlt + `)`),
		"businessDomain":             regexp.MustCompile(`(?i)businessDomain\s*[=:]\s*(.+)`),
	}
	jsonBucketPatterns = map[string]*regexp.Regexp{
		"conceptualAssessmentBucket": regexp.MustCompile(`(?i)"conceptualAssessmentBucket"\s*:\s*"(` + ratingValueAlt + `)"`),
		"codeAssessmentBucket":       regexp.MustCompile(`(?i)"codeAssessmentBucket"\s*:\s*"(` + ratingValueAlt + `)"`),
		"bugAssessmentBucket":        regexp.MustCompile(`(?i)"bugAssessmentBucket"\s*:\s*"(` + ratingValueAlt + `)"`),
	}
	ipyintervuBlockPattern = regexp.MustCompile("(?is)```(?:json)?\\s*_ipy(?:intervu)?\\s*\\n(.*?)\\n```")
	partialIPyIntervuOpenPattern = regexp.MustCompile("(?is)```(?:json)?\\s*_ipy(?:intervu)?\\s*\\n([\\s\\S]+)$")
	proseConceptualBucketPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)conceptual\s+(?:understanding\s+)?(?:assessment\s+)?(?:level|rating|bucket)?(?:\s+is|\s*:\s*|\*\*)\s*(` + proseRatingLabels + `)`),
		regexp.MustCompile(`(?i)(?:your|the)\s+conceptual\s+(?:understanding\s+)?(?:assessment\s+)?(?:is\s+)?(?:rated\s+)?(` + proseRatingLabels + `)`),
		regexp.MustCompile(`(?i)conceptual[^.\n]{0,80}\b(` + proseRatingLabels + `)\b`),
	}
	proseCodeBucketPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)code\s+(?:problem\s+)?(?:assessment\s+)?(?:level|rating|bucket)?(?:\s+is|\s*:\s*|\*\*)\s*(` + proseRatingLabels + `|N/A|na)`),
		regexp.MustCompile(`(?i)(?:your|the)\s+code\s+(?:assessment\s+)?(?:is\s+)?(?:rated\s+)?(` + proseRatingLabels + `|N/A|na)`),
		regexp.MustCompile(`(?i)code[^.\n]{0,80}\b(` + proseRatingLabels + `|N/A|na)\b`),
	}
	proseBugBucketPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)bug\s+(?:hunting\s+)?(?:assessment\s+)?(?:level|rating|bucket)?(?:\s+is|\s*:\s*|\*\*)\s*(` + proseRatingLabels + `|N/A|na)`),
		regexp.MustCompile(`(?i)(?:your|the)\s+bug\s+(?:assessment\s+)?(?:is\s+)?(?:rated\s+)?(` + proseRatingLabels + `|N/A|na)`),
		regexp.MustCompile(`(?i)bug[^.\n]{0,80}\b(` + proseRatingLabels + `|N/A|na)\b`),
	}
)

type ipyintervuTail struct {
	ConceptualAssessmentBucket string `json:"conceptualAssessmentBucket"`
	ConceptualAssessmentPhase  string `json:"conceptualAssessmentPhase"`
	CodeAssessmentBucket       string `json:"codeAssessmentBucket"`
	CodeAssessmentPhase        string `json:"codeAssessmentPhase"`
	BugAssessmentBucket        string `json:"bugAssessmentBucket"`
	BugAssessmentPhase         string `json:"bugAssessmentPhase"`
	BusinessDomain             any    `json:"businessDomain"`
	ActiveMode                 string `json:"activeMode"`
}

var jsonPhasePatterns = map[string]*regexp.Regexp{
	"conceptualAssessmentPhase": regexp.MustCompile(`(?i)"conceptualAssessmentPhase"\s*:\s*"(in[_ -]?progress|completed?)"`),
	"codeAssessmentPhase":       regexp.MustCompile(`(?i)"codeAssessmentPhase"\s*:\s*"(in[_ -]?progress|completed?)"`),
	"bugAssessmentPhase":        regexp.MustCompile(`(?i)"bugAssessmentPhase"\s*:\s*"(in[_ -]?progress|completed?)"`),
}

func truncateSummary(text string, max int) string {
	text = strings.TrimSpace(text)
	if len(text) <= max {
		return text
	}
	return text[:max] + "..."
}

func applyPreChatUserUpdate(state *AgentSessionState, userMessage string) {
	state.MessageIndex++
	state.LastUserMessageRaw = userMessage
	state.FirstUserMessageSeen = true

	switch state.ConversationPhase {
	case phaseAwaitingMajor:
		if state.StudentMajor == "" && !isGreetingMessage(userMessage) && strings.TrimSpace(userMessage) != "" {
			state.StudentMajor = strings.TrimSpace(userMessage)
			state.ConversationPhase = phaseAwaitingKeyConcept
			state.PendingQuestion = "weeklyKeyConceptSelection"
			state.WaitingForUserResponse = true
		}
	case phaseAwaitingKeyConcept:
		if sel, ok := matchWeekSelection(userMessage); ok {
			state.SelectedKeyConcept = sel.SelectedKeyConcept
			state.CurrentWeekNumber = sel.CurrentWeekNumber
			state.ConversationPhase = phaseAssessmentInProgress
			state.ActiveMode = modeConceptual
			state.ModesCompleted = nil
			state.ConceptualAssessmentBucket = ""
			state.ConceptualAssessmentPhase = ""
			state.CodeAssessmentBucket = ""
			state.CodeAssessmentPhase = ""
			state.BugAssessmentBucket = ""
			state.BugAssessmentPhase = ""
			state.PendingQuestion = "conceptualAssessment"
			state.WaitingForUserResponse = true
			state.CoachingRequested = false
			state.CoachingEnteredBeforeResults = false
			resetModeInterviewProgress(state)
			markAssessmentStarted(state)
			// Update concepts for this week
			initializePersonasAndConcepts(state)
		}
	case phaseAssessmentInProgress:
		if isMidInterviewCoachingRequest(userMessage) {
			state.CoachingRequested = true
			state.ActiveMode = modeCoaching
			state.CoachingEnteredBeforeResults = true
		} else {
			if state.ActiveMode == modeCoaching {
				if resumed := resumeAssessmentActiveMode(state); resumed != "" {
					state.ActiveMode = resumed
					state.PendingQuestion = pendingQuestionForMode(resumed)
				}
			}
			clampAssessmentModeForward(state)
			if state.ActiveMode == modeConceptual || state.ActiveMode == modeCode || state.ActiveMode == modeBug {
				advanceInterviewProgressOnUserAnswer(state)
			}
		}
	case phaseAssessmentResults:
		if isCoachingRequest(userMessage) {
			state.CoachingRequested = true
			state.ActiveMode = modeCoaching
		}
	}
}

func applyBootstrapState(state *AgentSessionState, assistant string) {
	state.StartupPromptShown = true
	state.FirstUserMessageSeen = true
	if state.StudentMajor != "" || state.ConversationPhase != phaseAwaitingMajor {
		// A chat turn already advanced this session (late bootstrap completion,
		// e.g. a second tab sharing the session cookie); do not rewind it.
		return
	}
	state.ConversationPhase = phaseAwaitingMajor
	state.WaitingForUserResponse = true
	state.PendingQuestion = "studentMajor"
	state.MessageIndex = 1
	state.LastAssistantSummary = truncateSummary(assistant, 500)

	// Initialize persona names and concept data
	initializePersonasAndConcepts(state)
}

func initializePersonasAndConcepts(state *AgentSessionState) {
	// Persona names mapping (mode + persona number to human name)
	state.PersonaNames = map[string]string{
		"Conceptual-1": "Alex",
		"Conceptual-2": "Julia",
		"Code-1":       "Taylor",
		"Code-2":       "Morgan",
		"Bug-1":        "Riley",
		"Bug-2":        "Casey",
		"Coaching-1":   "Samantha",
		"Coaching-2":   "David",
	}

	// Concepts by week (markdown format)
	weekMarkdown := map[int]string{
		1: "**Week 1: Problem Decomposition**\n- input/process/output\n- breaking tasks into steps\n",
		2: "**Week 2: Variables & Expressions**\n- variables, assignment, expressions\n- int, float, str, bool types\n",
		3: "**Week 3: Input & Type Casting**\n- input() function\n- type casting: int(), float(), str()\n",
		4: "**Week 4: String Methods**\n- string methods: strip, upper, lower, split\n",
		5: "**Week 5: Conditionals**\n- if, elif, else\n- comparisons and boolean logic: and, or, not\n",
		6: "**Week 6: For Loops**\n- for loops, range(), iterating sequences\n",
		7: "**Week 7: While Loops & Menus**\n- while loops with conditions\n- menus and repeat-until-quit patterns\n",
		8: "**Week 8: Lists**\n- lists, list indexing, list methods\n- list iteration and manipulation\n",
		9: "**Week 9: Lists and File I/O**\n- file I/O: open, read/write, with open\n- organizing file data with lists\n",
	}

	// Build markdown for weeks 1 through currentWeekNumber
	if state.CurrentWeekNumber > 0 {
		var b strings.Builder
		b.WriteString("## Allowed Concepts (Weeks 1-" + fmt.Sprint(state.CurrentWeekNumber) + ")\n\n")
		for week := 1; week <= state.CurrentWeekNumber; week++ {
			b.WriteString(weekMarkdown[week])
		}
		state.CurrentWeekConcepts = b.String()
	}
}

func parseAssistantStateSync(state *AgentSessionState, assistant string) {
	if block := ipyintervuBlockPattern.FindStringSubmatch(assistant); len(block) == 2 {
		applyIPyIntervuTailJSON(state, block[1])
	} else if partial := partialIPyIntervuOpenPattern.FindStringSubmatch(assistant); len(partial) == 2 {
		applyIPyIntervuTailJSON(state, partial[1])
	}

	// Phase before bucket: a "complete" in this reply must be applied before the
	// bucket, or applyLegacyBucketSync rejects the bucket against the prior in_progress.
	for field, pattern := range jsonPhasePatterns {
		match := pattern.FindStringSubmatch(assistant)
		if len(match) < 2 {
			continue
		}
		applyModePhaseFromTail(state, phaseFieldToMode(field), normalizeAssessmentPhase(match[1]))
	}

	for field, pattern := range bucketPatterns {
		match := pattern.FindStringSubmatch(assistant)
		if len(match) < 2 {
			continue
		}
		value := strings.TrimSpace(match[1])
		switch field {
		case "conceptualAssessmentBucket":
			applyLegacyBucketSync(state, modeConceptual, value)
		case "codeAssessmentBucket":
			applyLegacyBucketSync(state, modeCode, value)
		case "bugAssessmentBucket":
			applyLegacyBucketSync(state, modeBug, value)
		case "businessDomain":
			state.BusinessDomain = value
		}
	}

	for field, pattern := range jsonBucketPatterns {
		match := pattern.FindStringSubmatch(assistant)
		if len(match) < 2 {
			continue
		}
		switch field {
		case "conceptualAssessmentBucket":
			applyLegacyBucketSync(state, modeConceptual, match[1])
		case "codeAssessmentBucket":
			applyLegacyBucketSync(state, modeCode, match[1])
		case "bugAssessmentBucket":
			applyLegacyBucketSync(state, modeBug, match[1])
		}
	}

	detectProseAssessmentBuckets(state, assistant)
}

func applyIPyIntervuTailJSON(state *AgentSessionState, rawJSON string) {
	jsonText := strings.TrimSpace(rawJSON)
	jsonText = strings.TrimSuffix(jsonText, "```")
	jsonText = strings.TrimSpace(jsonText)
	if jsonText == "" {
		return
	}

	var tail ipyintervuTail
	if err := json.Unmarshal([]byte(jsonText), &tail); err == nil {
		applyModeSyncFromTail(state, modeConceptual, tail.ConceptualAssessmentPhase, tail.ConceptualAssessmentBucket)
		applyModeSyncFromTail(state, modeCode, tail.CodeAssessmentPhase, tail.CodeAssessmentBucket)
		applyModeSyncFromTail(state, modeBug, tail.BugAssessmentPhase, tail.BugAssessmentBucket)
		if tail.BusinessDomain != nil {
			if raw, err := json.Marshal(tail.BusinessDomain); err == nil {
				state.BusinessDomain = string(raw)
			}
		}
		return
	}

	for field, pattern := range jsonPhasePatterns {
		match := pattern.FindStringSubmatch(jsonText)
		if len(match) < 2 {
			continue
		}
		applyModePhaseFromTail(state, phaseFieldToMode(field), normalizeAssessmentPhase(match[1]))
	}

	for field, pattern := range jsonBucketPatterns {
		match := pattern.FindStringSubmatch(jsonText)
		if len(match) < 2 {
			continue
		}
		applyLegacyBucketSync(state, bucketFieldToMode(field), match[1])
	}
}

func phaseFieldToMode(field string) string {
	switch field {
	case "conceptualAssessmentPhase":
		return modeConceptual
	case "codeAssessmentPhase":
		return modeCode
	case "bugAssessmentPhase":
		return modeBug
	default:
		return ""
	}
}

func bucketFieldToMode(field string) string {
	switch field {
	case "conceptualAssessmentBucket":
		return modeConceptual
	case "codeAssessmentBucket":
		return modeCode
	case "bugAssessmentBucket":
		return modeBug
	default:
		return ""
	}
}

func normalizeAssessmentPhase(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "in progress", "in-progress", "in_progress", "assessing", "ongoing":
		return assessmentPhaseInProgress
	case "complete", "completed", "done", "finished":
		return assessmentPhaseComplete
	default:
		return strings.TrimSpace(value)
	}
}

func isValidFinalBucket(bucket string) bool {
	switch bucket {
	case bucketNotReady, bucketCompetent, bucketExceptional, bucketNA:
		return true
	default:
		return false
	}
}

func modePhaseValue(state *AgentSessionState, mode string) string {
	switch mode {
	case modeConceptual:
		return state.ConceptualAssessmentPhase
	case modeCode:
		return state.CodeAssessmentPhase
	case modeBug:
		return state.BugAssessmentPhase
	default:
		return ""
	}
}

func setModePhase(state *AgentSessionState, mode, phase string) {
	switch mode {
	case modeConceptual:
		state.ConceptualAssessmentPhase = phase
	case modeCode:
		state.CodeAssessmentPhase = phase
	case modeBug:
		state.BugAssessmentPhase = phase
	}
}

func setModeBucket(state *AgentSessionState, mode, bucket string) {
	switch mode {
	case modeConceptual:
		state.ConceptualAssessmentBucket = bucket
	case modeCode:
		state.CodeAssessmentBucket = bucket
	case modeBug:
		state.BugAssessmentBucket = bucket
	}
}

func applyModePhaseFromTail(state *AgentSessionState, mode, phase string) {
	if mode == "" || phase == "" {
		return
	}
	if phase == assessmentPhaseInProgress && modeIsCompleted(state, mode) {
		return
	}
	setModePhase(state, mode, phase)
}

func applyModeSyncFromTail(state *AgentSessionState, mode, phase, bucket string) {
	if mode == "" {
		return
	}
	if phase == assessmentPhaseInProgress && modeIsCompleted(state, mode) {
		return
	}
	phase = normalizeAssessmentPhase(phase)
	bucket = normalizeAssessmentBucket(bucket)

	if phase != "" {
		setModePhase(state, mode, phase)
	}

	switch phase {
	case assessmentPhaseInProgress:
		return
	case assessmentPhaseComplete:
		if bucket != "" && isValidFinalBucket(bucket) {
			setModeBucket(state, mode, bucket)
		}
	default:
		if bucket != "" && isValidFinalBucket(bucket) {
			setModeBucket(state, mode, bucket)
			setModePhase(state, mode, assessmentPhaseComplete)
		}
	}
}

func applyLegacyBucketSync(state *AgentSessionState, mode, rawBucket string) {
	bucket := normalizeAssessmentBucket(rawBucket)
	if bucket == "" || !isValidFinalBucket(bucket) {
		return
	}
	if modePhaseValue(state, mode) == assessmentPhaseInProgress {
		return
	}
	setModeBucket(state, mode, bucket)
	setModePhase(state, mode, assessmentPhaseComplete)
}

func isConceptualModeComplete(state *AgentSessionState) bool {
	if state.ConceptualAssessmentBucket == "" || !isValidFinalBucket(state.ConceptualAssessmentBucket) {
		return false
	}
	if state.ConceptualAssessmentPhase == assessmentPhaseInProgress {
		return false
	}
	return state.ConceptualAssessmentPhase == assessmentPhaseComplete || state.ConceptualAssessmentPhase == ""
}

func isCodeModeComplete(state *AgentSessionState) bool {
	if state.CodeAssessmentBucket == "" || !isValidFinalBucket(state.CodeAssessmentBucket) {
		return false
	}
	if state.CodeAssessmentPhase == assessmentPhaseInProgress {
		return false
	}
	return state.CodeAssessmentPhase == assessmentPhaseComplete || state.CodeAssessmentPhase == ""
}

func isBugModeComplete(state *AgentSessionState) bool {
	if state.BugAssessmentBucket == "" || !isValidFinalBucket(state.BugAssessmentBucket) {
		return false
	}
	if state.BugAssessmentPhase == assessmentPhaseInProgress {
		return false
	}
	return state.BugAssessmentPhase == assessmentPhaseComplete || state.BugAssessmentPhase == ""
}

func normalizeAssessmentBucket(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "not yet ready", "not ready yet", "weak":
		return bucketNotReady
	case "looking good", "competent", "strong", "good", "solid":
		return bucketCompetent
	case "looks great", "exceptional", "excellent", "outstanding":
		return bucketExceptional
	case "n/a", "na":
		return bucketNA
	default:
		return strings.TrimSpace(value)
	}
}

func firstProseBucketMatch(assistant string, patterns []*regexp.Regexp) string {
	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(assistant)
		if len(match) >= 2 {
			return normalizeAssessmentBucket(match[1])
		}
	}
	return ""
}

func detectProseAssessmentBuckets(state *AgentSessionState, assistant string) {
	if state.ConceptualAssessmentPhase != assessmentPhaseInProgress && state.ConceptualAssessmentBucket == "" && state.ActiveMode == modeConceptual {
		if bucket := firstProseBucketMatch(assistant, proseConceptualBucketPatterns); bucket != "" {
			normalized := normalizeAssessmentBucket(bucket)
			if isValidFinalBucket(normalized) {
				state.ConceptualAssessmentBucket = normalized
				state.ConceptualAssessmentPhase = assessmentPhaseComplete
			}
		}
	}
	if state.CodeAssessmentPhase != assessmentPhaseInProgress && state.CodeAssessmentBucket == "" && state.ActiveMode == modeCode {
		if bucket := firstProseBucketMatch(assistant, proseCodeBucketPatterns); bucket != "" {
			normalized := normalizeAssessmentBucket(bucket)
			if isValidFinalBucket(normalized) {
				state.CodeAssessmentBucket = normalized
				state.CodeAssessmentPhase = assessmentPhaseComplete
			}
		}
	}
	if state.BugAssessmentPhase != assessmentPhaseInProgress && state.BugAssessmentBucket == "" && state.ActiveMode == modeBug {
		if bucket := firstProseBucketMatch(assistant, proseBugBucketPatterns); bucket != "" {
			normalized := normalizeAssessmentBucket(bucket)
			if isValidFinalBucket(normalized) {
				state.BugAssessmentBucket = normalized
				state.BugAssessmentPhase = assessmentPhaseComplete
			}
		}
	}
}

func appendModeCompleted(state *AgentSessionState, mode string) {
	for _, existing := range state.ModesCompleted {
		if existing == mode {
			return
		}
	}
	state.ModesCompleted = append(state.ModesCompleted, mode)
}

func computeFinalRating(state *AgentSessionState) string {
	if state.isProblemDecompositionWeek() {
		if state.ConceptualAssessmentBucket != "" {
			return state.ConceptualAssessmentBucket
		}
		return ""
	}

	buckets := []string{
		state.ConceptualAssessmentBucket,
		state.CodeAssessmentBucket,
		state.BugAssessmentBucket,
	}
	for _, bucket := range buckets {
		if bucket == bucketNotReady {
			return bucketNotReady
		}
	}
	hasCompetent := false
	allExceptional := true
	for _, bucket := range buckets {
		if bucket == "" || bucket == bucketNA {
			continue
		}
		if bucket == bucketCompetent {
			hasCompetent = true
			allExceptional = false
		}
		if bucket != bucketExceptional {
			allExceptional = false
		}
	}
	if hasCompetent {
		return bucketCompetent
	}
	if allExceptional {
		return bucketExceptional
	}
	return ""
}

func markAssessmentStarted(state *AgentSessionState) {
	now := time.Now()
	state.AssessmentStartTime = &now
	state.AssessmentEndTime = nil
	state.AssessmentComplete = false
}

func markAssessmentEnded(state *AgentSessionState) {
	if state.AssessmentEndTime != nil {
		return
	}
	now := time.Now()
	state.AssessmentEndTime = &now
}

func assessmentModeRank(mode string) int {
	switch mode {
	case modeConceptual:
		return 1
	case modeCode:
		return 2
	case modeBug:
		return 3
	default:
		return 0
	}
}

func modeIsCompleted(state *AgentSessionState, mode string) bool {
	for _, existing := range state.ModesCompleted {
		if existing == mode {
			return true
		}
	}
	switch mode {
	case modeConceptual:
		return isConceptualModeComplete(state)
	case modeCode:
		return isCodeModeComplete(state)
	case modeBug:
		return isBugModeComplete(state)
	default:
		return false
	}
}

func minimumAllowedAssessmentMode(state *AgentSessionState) string {
	if state.isProblemDecompositionWeek() {
		if !modeIsCompleted(state, modeConceptual) {
			return modeConceptual
		}
		return ""
	}
	if !modeIsCompleted(state, modeConceptual) {
		return modeConceptual
	}
	if !modeIsCompleted(state, modeCode) {
		return modeCode
	}
	if !modeIsCompleted(state, modeBug) {
		return modeBug
	}
	return ""
}

func resumeAssessmentActiveMode(state *AgentSessionState) string {
	return minimumAllowedAssessmentMode(state)
}

func pendingQuestionForMode(mode string) string {
	switch mode {
	case modeConceptual:
		return "conceptualAssessment"
	case modeCode:
		return "codeAssessment"
	case modeBug:
		return "bugAssessment"
	default:
		return ""
	}
}

// clampAssessmentModeForward enforces Conceptual → Code → Bug with no backward steps.
func clampAssessmentModeForward(state *AgentSessionState) {
	if state.ConversationPhase != phaseAssessmentInProgress || state.ActiveMode == modeCoaching {
		return
	}
	minMode := minimumAllowedAssessmentMode(state)
	if minMode == "" {
		return
	}
	minRank := assessmentModeRank(minMode)
	currentRank := assessmentModeRank(state.ActiveMode)
	if currentRank == 0 || currentRank < minRank {
		state.ActiveMode = minMode
		state.PendingQuestion = pendingQuestionForMode(minMode)
	}
}

// applyAutomaticModeTransitions updates activeMode after bucket assignment.
// Returns true when the server should immediately continue into the next assessment mode.
func applyAutomaticModeTransitions(state *AgentSessionState) bool {
	if state.CoachingRequested && state.ActiveMode == modeCoaching {
		return false
	}

	if state.ConversationPhase == phaseAssessmentResults {
		return false
	}

	if state.isProblemDecompositionWeek() {
		if isConceptualModeComplete(state) && state.ConversationPhase != phaseAssessmentResults {
			appendModeCompleted(state, modeConceptual)
			state.CodeAssessmentBucket = bucketNA
			state.BugAssessmentBucket = bucketNA
			state.ActiveMode = ""
			state.ConversationPhase = phaseAssessmentResults
			state.FinalRating = computeFinalRating(state)
			state.AssessmentComplete = state.FinalRating != ""
			state.PendingQuestion = "assessmentResults"
			state.WaitingForUserResponse = true
			markAssessmentEnded(state)
		}
		return false
	}

	switch state.ActiveMode {
	case modeConceptual:
		if isConceptualModeComplete(state) {
			appendModeCompleted(state, modeConceptual)
			state.ActiveMode = modeCode
			state.CodeAssessmentPhase = ""
			state.PendingQuestion = "codeAssessment"
			state.WaitingForUserResponse = true
			resetModeInterviewProgress(state)
			return true
		}
	case modeCode:
		if isCodeModeComplete(state) {
			appendModeCompleted(state, modeCode)
			state.ActiveMode = modeBug
			state.BugAssessmentPhase = ""
			state.PendingQuestion = "bugAssessment"
			state.WaitingForUserResponse = true
			resetModeInterviewProgress(state)
			return true
		}
	case modeBug:
		if isBugModeComplete(state) {
			appendModeCompleted(state, modeBug)
			state.ActiveMode = ""
			state.ConversationPhase = phaseAssessmentResults
			state.FinalRating = computeFinalRating(state)
			state.AssessmentComplete = state.FinalRating != ""
			state.PendingQuestion = "assessmentResults"
			state.WaitingForUserResponse = true
			markAssessmentEnded(state)
		}
	}
	return false
}

func modeContinuationUserMessage(state *AgentSessionState) string {
	switch state.ActiveMode {
	case modeCode:
		return "[System handoff: Server state activeMode is now CodeProblem. Begin the code assessment immediately: Taylor and Morgan introduce themselves at the company from businessDomain, present one task scenario, and ask exactly ONE decomposition question (one ask only — e.g. how to break the problem down OR what the input is, not both and not input/process/output as separate questions). End your reply with ```_ipyintervu``` including \"codeAssessmentPhase\": \"in_progress\". Do not ask the student to choose or confirm the next mode.]"
	case modeBug:
		return "[System handoff: Server state activeMode is now BugHunting. Begin the bug-hunting assessment immediately: Riley and Casey introduce themselves at businessDomain.companyName, present one buggy snippet with intended behavior, and ask exactly ONE debugging-process question (how the student would find the bug — no corrected code). End your reply with ```_ipyintervu``` including \"bugAssessmentPhase\": \"in_progress\" as the last lines — required on this first bug turn. Do not ask the student to choose or confirm the next mode.]"
	default:
		return ""
	}
}

const maxChatInternalTurns = 6

// phaseClosingMessage is shown in place of the model's reply that finished a phase.
const phaseClosingMessage = "Thanks — that completes this part of the interview."

// maxCorrectiveRetries bounds corrective model calls per user turn. One was not enough:
// a retry reply can itself break the rules (e.g. answer the server note with an
// explanation and no question), and that reply was then shown to the student.
const maxCorrectiveRetries = 2

type assistantTurnFollowUp struct {
	ContinueTurn     bool
	Handoff          string
	Kind             string // mode, results, corrective_retry, server_results, fail_closed
	DirectAssistant  string
}

func currentModeBucketValue(state *AgentSessionState) string {
	switch state.ActiveMode {
	case modeConceptual:
		return state.ConceptualAssessmentBucket
	case modeCode:
		return state.CodeAssessmentBucket
	case modeBug:
		return state.BugAssessmentBucket
	default:
		return ""
	}
}

func hasIPyIntervuBlock(assistant string) bool {
	return ipyintervuBlockPattern.MatchString(assistant) || partialIPyIntervuOpenPattern.MatchString(assistant)
}

func currentModeSyncFields(state *AgentSessionState) (phaseField, bucketField, modeLabel string) {
	switch state.ActiveMode {
	case modeConceptual:
		return "conceptualAssessmentPhase", "conceptualAssessmentBucket", "Conceptual Understanding"
	case modeCode:
		return "codeAssessmentPhase", "codeAssessmentBucket", "Code Problem"
	case modeBug:
		return "bugAssessmentPhase", "bugAssessmentBucket", "Bug Hunting"
	default:
		return "", "", ""
	}
}

func currentModePhaseValue(state *AgentSessionState) string {
	return modePhaseValue(state, state.ActiveMode)
}

func isActiveModeComplete(state *AgentSessionState) bool {
	switch state.ActiveMode {
	case modeConceptual:
		return isConceptualModeComplete(state)
	case modeCode:
		return isCodeModeComplete(state)
	case modeBug:
		return isBugModeComplete(state)
	default:
		return false
	}
}

func ipyBlockMissingCurrentBucket(state *AgentSessionState, assistant string) bool {
	if !hasIPyIntervuBlock(assistant) {
		return false
	}
	lower := strings.ToLower(assistant)
	if strings.Contains(lower, "assessmentbucket") && currentModeBucketValue(state) == "" {
		return true
	}
	if state.ActiveMode == modeConceptual && strings.Contains(lower, "businessdomain") && !strings.Contains(lower, "conceptualassessmentbucket") && !strings.Contains(lower, "conceptualassessmentphase") {
		return false
	}
	return false
}

func shouldForceAssessmentSync(state *AgentSessionState, assistant string) bool {
	if state.CoachingRequested && state.ActiveMode == modeCoaching {
		return false
	}
	if ipyBlockMissingCurrentBucket(state, assistant) {
		return true
	}
	return currentModePhaseValue(state) == assessmentPhaseComplete && currentModeBucketValue(state) == ""
}

// postProcessAssistantTurn parses bucket sync, applies mode transitions, and decides
// whether the server should immediately call the model again (mode handoff or corrective retry).
func postProcessAssistantTurn(state *AgentSessionState, assistant string, correctiveRetryAttempted bool, syncLog *ipySyncLogContext) assistantTurnFollowUp {
	return postProcessAssistantTurnWithGuard(state, assistant, correctiveRetryAttempted, syncLog)
}

func postProcessAssistantTurnWithGuard(state *AgentSessionState, assistant string, correctiveRetryAttempted bool, syncLog *ipySyncLogContext) assistantTurnFollowUp {
	phaseBefore := state.ConversationPhase

	if syncLog != nil {
		complete, partial := collectIPyIntervuSyncPortions(assistant)
		logIPyIntervuSyncPortions(*syncLog, complete, partial)
	}

	parseAssistantStateSync(state, assistant)
	shouldContinueMode := applyAutomaticModeTransitions(state)
	clampAssessmentModeForward(state)
	state.LastAssistantSummary = truncateSummary(assistant, 500)

	if shouldContinueMode {
		if handoff := modeContinuationUserMessage(state); handoff != "" {
			return assistantTurnFollowUp{ContinueTurn: true, Handoff: handoff, Kind: "mode"}
		}
	}

	if state.ConversationPhase == phaseAssessmentResults && phaseBefore != phaseAssessmentResults {
		if state.FinalRating != "" {
			return assistantTurnFollowUp{
				Kind:            "server_results",
				DirectAssistant: buildServerAssessmentResultsMessage(state),
			}
		}
	}

	violations := detectAssessmentViolations(state, assistant)
	if syncLog != nil && violations.Any() {
		logAssessmentViolations(syncLog.sessionID, syncLog.turnID, syncLog.modeTurn, violations)
	}

	// After the one corrective retry, deliver the retry reply (the client-visible guard
	// trims it to a single question) unless nothing usable is left. Failing closed on
	// any residual violation left students stuck re-sending the same answer.
	if correctiveRetryAttempted && violations.StillInvalidAfterRetry() &&
		clientVisibleAssistantContentGuarded(assistant, state) == "" {
		return assistantTurnFollowUp{
			Kind:            "fail_closed",
			DirectAssistant: buildAssessmentTurnFailureMessage(),
		}
	}

	if !correctiveRetryAttempted && violations.NeedsCorrectiveRetry() {
		if handoff := buildUnifiedCorrectiveHandoff(state, violations); handoff != "" {
			return assistantTurnFollowUp{ContinueTurn: true, Handoff: handoff, Kind: "corrective_retry"}
		}
	}

	updateInterviewProgressAfterAssistant(state, assistant)
	return assistantTurnFollowUp{}
}

func applyPostChatStateUpdate(state *AgentSessionState, userMessage, assistant string) bool {
	parseAssistantStateSync(state, assistant)
	shouldContinue := applyAutomaticModeTransitions(state)
	state.LastAssistantSummary = truncateSummary(assistant, 500)
	return shouldContinue
}
