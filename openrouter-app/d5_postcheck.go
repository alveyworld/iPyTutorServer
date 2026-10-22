package main

import (
	"regexp"
	"strings"
)

var d5ClarificationPhrases = append([]string{
	"what do you mean", "can you explain the question", "could you explain the question",
	"repeat the question", "say that again", "what are you asking", "which part",
}, clarificationPhrases...)

// normalizeQuotes turns curly quotes into straight ones so phrase checks match
// "didn’t" as well as "didn't".
var quoteReplacer = strings.NewReplacer("\u2019", "'", "\u2018", "'", "\u201c", "\"", "\u201d", "\"")

func normalizeQuotes(s string) string { return quoteReplacer.Replace(s) }

// d5NonAnswers are whole replies that decline to answer.
var d5NonAnswers = map[string]bool{"no": true, "nothing": true, "pass": true, "skip": true, "?": true, "idk": true, "dunno": true, "no idea": true}

// d5IsVagueAnswer reports a non-committal answer: empty, a bare non-answer ("idk",
// "pass"), or a short reply built on a hedge ("not sure, maybe a print"). Unlike
// isVagueAnswer, a short answer is not vague by length alone: "boolean" can be a complete
// answer, and with lowest-label grading a length rule turned every concise answer into a
// Not Ready Yet for the whole mode.
func d5IsVagueAnswer(msg string) bool {
	trimmed := strings.TrimSpace(normalizeQuotes(msg))
	if trimmed == "" {
		return true
	}
	if d5LooksLikeCodeSubmission(trimmed) {
		return false
	}
	lower := strings.ToLower(strings.Trim(trimmed, ".!"))
	if d5NonAnswers[lower] {
		return true
	}
	if containsAny(lower, clarificationPhrases) {
		return false
	}
	return len(strings.Fields(lower)) <= vagueHedgeWordLimit && containsAny(lower, vagueAnswerPhrases)
}

var (
	noAIPattern   = regexp.MustCompile(`(?i)^\W*(?:no|nope|nah)\b|\b(?:didn'?t|did not|never|haven'?t|have not|don'?t|do not|wasn'?t|was not)\s+(?:really\s+|actually\s+)?(?:use|used|using|rely|need)\b[^.!?]{0,40}\b(?:ai|a\.i\.|chatgpt|copilot|gemini|claude|tools?)\b|\bwithout (?:any |using )?(?:ai|a\.i\.|tools?)\b|\bno (?:ai|a\.i\.)\b`)
	usedAIPattern = regexp.MustCompile(`(?i)\b(?:i|we)\s+(?:did\s+|also\s+)?(?:use|used|asked|tried|had)\b[^.!?]{0,30}\b(?:ai|a\.i\.|chatgpt|copilot|gemini|claude)\b|\b(?:asked|used|prompted)\s+(?:an?\s+|the\s+)?(?:ai|chatgpt|copilot|gemini|claude)\b`)
)

// d5SaysNoAI reports an answer to an AI-use question saying the candidate did not use AI.
// The rubric only describes students who used AI, so these answers are not graded on
// ai_use (grading-rules.md). An answer that also says AI was used counts as using it.
func d5SaysNoAI(msg string) bool {
	msg = normalizeQuotes(msg)
	m := regexp.MustCompile(`(?i)\bbut\b[^.!?]*\b(?:ai|chatgpt|copilot)\b`).FindString(msg)
	return noAIPattern.MatchString(msg) && !usedAIPattern.MatchString(msg) && m == ""
}

// answerVoicePattern marks a message that commits to an answer ("I'd add a print"), even
// when phrased as a question.
var answerVoicePattern = regexp.MustCompile(`(?i)\bi(?:'d| would| will|'ll| think| guess| believe| might)\b`)

// d5AskedForClarification is the tightened clarification check (design §6): a
// clarification phrase in a short message, or a short message (≤ 20 words) that is mainly
// a question back to the interviewer. Any "?" no longer counts, so "Would I check the
// input first? I'd add a print…" stays an answer.
func d5AskedForClarification(msg string) bool {
	trimmed := strings.TrimSpace(normalizeQuotes(msg))
	if trimmed == "" || d5LooksLikeCodeSubmission(trimmed) {
		return false
	}
	lower := strings.ToLower(trimmed)
	words := len(strings.Fields(lower))
	if words <= 25 && containsAny(lower, d5ClarificationPhrases) {
		return true
	}
	return words <= 20 && strings.HasSuffix(lower, "?") && !answerVoicePattern.MatchString(lower)
}

// pythonStatementLine matches a line that reads as a Python statement rather than prose.
var pythonStatementLine = regexp.MustCompile(`^\s*(?:def \w+\(|import \w|from \w+ import|(?:for|while|if|elif) .+:\s*$|else:\s*$|return\b|print\(|[A-Za-z_]\w*(?:\[[^\]]*\])?\s*[-+*/]?=[^=])`)

// d5LooksLikeCodeSubmission reports pasted code. It is stricter than looksLikeCodeSubmission,
// which treats any "if " or "for " as code, so a prose decomposition ("apply the discount
// if it is over 50") would count as a paste and skip the request for code.
func d5LooksLikeCodeSubmission(msg string) bool {
	if strings.Contains(msg, "```") {
		return true
	}
	lines := 0
	for _, line := range strings.Split(msg, "\n") {
		if pythonStatementLine.MatchString(line) {
			lines++
		}
	}
	if lines >= 2 {
		return true
	}
	return lines == 1 && len(strings.Split(strings.TrimSpace(msg), "\n")) == 1 && strings.ContainsAny(msg, "()")
}

var openingCallPattern = regexp.MustCompile(`\b(?:input|int|float|str|print|len|range|open|round)\s*\(`)

// d5OpeningGivesCode reports a Code opening that shows code. The task must be a story
// problem in plain words; code in it ("Data available: name = input(...)") hands the
// candidate part of the solution.
func d5OpeningGivesCode(reply string) bool {
	if strings.Contains(reply, "`") || openingCallPattern.MatchString(reply) {
		return true
	}
	for _, line := range strings.Split(reply, "\n") {
		if pythonStatementLine.MatchString(line) {
			return true
		}
	}
	return false
}

// bugMarkerPattern matches the "# Bug: ..." comment that marks a Bug snippet's defect,
// either trailing the faulty line or on its own line where a missing line belongs.
var bugMarkerPattern = regexp.MustCompile(`(?i)#\s*bug\s*:`)

// d5MarkedBugLine returns the snippet line that carries the "# Bug:" marker, trimmed, or "".
// It is what the candidate sees, so it is the most reliable account of the bug.
func d5MarkedBugLine(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if bugMarkerPattern.MatchString(line) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// taughtFunctions and taughtMethods are the calls the course teaches, by the week they
// are introduced. Interviewer-generated code may use only these (a whitelist; the course
// teaches no other built-ins). Students may use anything they can explain.
var taughtFunctions = map[string]int{"print": 2, "input": 3, "int": 3, "float": 3, "str": 3, "range": 6, "len": 8, "open": 9}
var taughtMethods = map[string]int{
	"strip": 4, "upper": 4, "lower": 4, "split": 4, "replace": 4,
	"isdigit": 4, "isalpha": 4, "isalnum": 4, "isupper": 4, "islower": 4,
	"append": 8, "remove": 8, "sort": 8, "pop": 8, "insert": 8,
	"read": 9, "readline": 9, "readlines": 9, "write": 9,
}

var (
	codeCallPattern     = regexp.MustCompile(`(\.)?\b([A-Za-z_]\w*)\s*\(`)
	untaughtKeywordCode = regexp.MustCompile(`\b(def|lambda|import|try|except|class|return|yield|global)\b`)
	controlWords        = map[string]bool{"if": true, "elif": true, "while": true, "for": true, "and": true, "or": true, "not": true, "in": true, "else": true}
)

// d5UntaughtCalls lists calls and keywords in the code of interviewer-generated text that
// are not taught by week, e.g. "sorted()", ".title()", "def". Calls that also appear in
// studentText (the candidate's own messages) are allowed, so the interviewer can ask about
// the candidate's own choices.
func d5UntaughtCalls(week int, text, studentText string) []string {
	code := replyCodeSegments(text)
	if code == "" {
		return nil
	}
	seen := map[string]bool{}
	var found []string
	add := func(item string) {
		if !seen[item] {
			seen[item] = true
			found = append(found, item)
		}
	}
	for _, m := range codeCallPattern.FindAllStringSubmatch(code, -1) {
		name := m[2]
		if m[1] == "." {
			if w, ok := taughtMethods[name]; (ok && w <= week) || strings.Contains(studentText, "."+name+"(") {
				continue
			}
			add("." + name + "()")
			continue
		}
		if controlWords[name] {
			continue
		}
		if w, ok := taughtFunctions[name]; (ok && w <= week) || regexp.MustCompile(`\b`+regexp.QuoteMeta(name)+`\s*\(`).MatchString(studentText) {
			continue
		}
		add(name + "()")
	}
	for _, m := range untaughtKeywordCode.FindAllStringSubmatch(code, -1) {
		if !strings.Contains(studentText, m[1]+" ") {
			add(m[1])
		}
	}
	return found
}

// d5ScopeIssues combines the word-based scope check with the taught-calls whitelist.
func d5ScopeIssues(week int, text, studentText string) []string {
	found := d5OutOfScopeConcepts(week, text)
	if calls := d5UntaughtCalls(week, text, studentText); len(calls) > 0 {
		found = append(found, "not taught: "+strings.Join(calls, ", "))
	}
	return found
}

// d5ScopeDetectors are scopeDetectors with two false positives fixed (design §11):
// prose "user input" is fine in Week 1 decomposition (handled in d5OutOfScopeConcepts),
// and prose "file(s)" alone is ordinary business wording, not week 9 file I/O.
var d5ScopeDetectors = func() []scopeDetector {
	out := make([]scopeDetector, len(scopeDetectors))
	copy(out, scopeDetectors)
	for i := range out {
		if out[i].week == 9 {
			out[i].prose = scopePatterns(
				`\b(?:read|reads|reading|write|writes|writing|open|opens|opening|save|saves|saving|load|loads|loading)\b[^.?!\n]{0,30}\b(?:text |data |csv )?files?\b`,
				`\bfile (?:i/o|io|handling)\b`, `\bopen\s*\(`, `\bcsv\b`, `\.(?:read|readline|readlines|write)\s*\(`, `\.txt\b`,
			)
		}
	}
	return out
}()

var week3ProseInputPattern = regexp.MustCompile(`(?i)\buser (?:will |would |should |can )?(?:enters?|types?|inputs?)\b|\buser input\b|\b(?:prompts?|asks?) the user\b`)

// d5OutOfScopeConcepts is outOfScopeConcepts with the D5 detector fixes.
func d5OutOfScopeConcepts(currentWeek int, visible string) []string {
	if currentWeek < 1 {
		return nil
	}
	code := replyCodeSegments(visible)
	var found []string
	for _, d := range d5ScopeDetectors {
		if d.week <= currentWeek {
			continue
		}
		prose := visible
		if currentWeek == 1 && d.week == 3 {
			// Week 1 scenarios talk about what a user enters; that is decomposition, not input().
			prose = week3ProseInputPattern.ReplaceAllString(prose, " ")
		}
		if matchesAny(d.prose, prose) || (code != "" && matchesAny(d.code, code)) {
			if d.week == neverInScopeWeek {
				found = append(found, d.name+" (never in scope)")
			} else {
				found = append(found, d.name+" (later week)")
			}
		}
	}
	return found
}

var (
	loopBanCodePattern  = regexp.MustCompile(`(?i)\bwhile\s+true\b|\bbreak\b|\bcontinue\b`)
	loopBanProsePattern = regexp.MustCompile(`(?i)\bwhile true\b|\bbreak statements?\b|\bcontinue statements?\b`)
)

// promptsBannedLoopControl reports a reply that brings up `while True`, `break` or
// `continue` in weeks 7–9 when the candidate has not used them in this mode.
func promptsBannedLoopControl(week int, reply string, candidateText string) bool {
	if !loopBanApplies(week) {
		return false
	}
	mentioned := loopBanCodePattern.MatchString(replyCodeSegments(reply)) || loopBanProsePattern.MatchString(reply)
	if !mentioned {
		return false
	}
	return !loopBanCodePattern.MatchString(candidateText)
}

// correctnessVerdictPattern is evaluativePraisePattern narrowed to verdicts on whether an
// answer was right; warm phrasing ("Thanks, that's helpful") is allowed (design §9).
var correctnessVerdictPattern = regexp.MustCompile(`(?i)\b(?:that's|that is) (?:exactly |absolutely |totally )?(?:right|correct)\b|\bexactly right\b|\byou(?:'re| are) (?:absolutely |exactly |totally )?(?:right|correct)\b|\bspot on\b|\bnailed it\b|(?:^|\n)\s*(?:exactly|correct)\b`)

// leadingVerdictPattern matches a reply that opens by judging the answer ("Exactly.",
// "That's right!", "Correct —"). The interviewer never says whether an answer was right.
var leadingVerdictPattern = regexp.MustCompile(`(?i)^(?:exactly|precisely|correct|spot on|that's (?:exactly |absolutely )?(?:right|correct)|you're (?:exactly |absolutely )?(?:right|correct)|right)\s*[.!,—–-]+\s*`)

// stripLeadingVerdict removes an opening verdict and re-capitalizes what follows.
func stripLeadingVerdict(reply string) string {
	loc := leadingVerdictPattern.FindStringIndex(reply)
	if loc == nil || loc[1] >= len(reply) {
		return reply
	}
	rest := reply[loc[1]:]
	return strings.ToUpper(rest[:1]) + rest[1:]
}

var (
	d5PersonaGreetingPattern = regexp.MustCompile(`(?i)^\s*(?:hey|hi|hello|thanks|thank you)[,\s]+(?:` + d5PersonaNamePattern + `)\s*[,.!:—–-]`)
	d5PersonaAddressPattern  = regexp.MustCompile(`(?i)^\s*(?:so|and|now|ok(?:ay)?)?[,\s]*(?:` + d5PersonaNamePattern + `)\s*[,:—–-]|[,—–-]\s*(?:` + d5PersonaNamePattern + `)\s*[?.!]*\s*$`)
)

// d5AddressesPersona reports a sentence that greets or questions another interviewer,
// using the pooled persona names. The current speaker introducing themselves is fine.
func d5AddressesPersona(reply, speaker string) bool {
	for _, sentence := range replySentencePattern.FindAllString(stripCodeFences(reply), -1) {
		if speaker != "" && strings.Contains(strings.ToLower(sentence), "i'm "+strings.ToLower(speaker)) {
			continue
		}
		if d5PersonaGreetingPattern.MatchString(sentence) || (strings.Contains(sentence, "?") && d5PersonaAddressPattern.MatchString(sentence)) {
			return true
		}
	}
	return false
}

// d5SpeaksAsInstructor is speaksAsInstructor without the education-major exception.
func d5SpeaksAsInstructor(reply string) bool {
	prose := stripCodeFences(reply)
	return classroomFramingPattern.MatchString(prose) || instructorRolePattern.MatchString(prose)
}

// d5PostCheckResult is the outcome of the after-the-fact checks on one Interviewer reply.
type d5PostCheckResult struct {
	Reply  string
	Issues []string
}

// d5PostCheck runs the §9 checks on a finished Interviewer reply. It never triggers a
// retry: self-answers are truncated, a reply with no question gets the fallback question
// appended, and everything else is reported for the next Evaluator run.
func d5PostCheck(state *AgentSessionState, sess *d5Session, mode string, move d5Move, reply, fallbackQuestion string) d5PostCheckResult {
	res := d5PostCheckResult{Reply: strings.TrimSpace(reply)}
	add := func(issue string) { res.Issues = append(res.Issues, issue) }

	if stripped := stripLeadingVerdict(res.Reply); stripped != res.Reply {
		res.Reply = stripped
		add("opened with a verdict on the answer (removed)")
	}

	if !move.isOpening() && looksLikeSelfAnsweredQuestion(res.Reply) {
		res.Reply = strings.TrimSpace(guardQuestionOnlyResponse(res.Reply))
		add("answered its own question or wrote the candidate's reply")
	}
	// A request for code ("please write … and paste it here") asks something without a "?".
	if !asksStudentSomething(res.Reply) && !looksLikeCodeRequest(strings.ToLower(res.Reply)) && fallbackQuestion != "" {
		res.Reply = strings.TrimSpace(res.Reply + "\n\n" + fallbackQuestion)
		add("asked the candidate nothing")
	}
	if countInterviewQuestions(res.Reply) > 1 {
		add("asked more than one question")
	}
	if correctnessVerdictPattern.MatchString(stripCodeFences(res.Reply)) {
		add("told the candidate whether they were right")
	}
	if leaksReasoning(res.Reply) {
		add("used internal wording (the student, rubric, portion)")
	}
	if d5AddressesPersona(res.Reply, sess.Personas[mode].Name) {
		add("addressed another interviewer instead of the candidate")
	}
	if !move.isOpening() && move.Kind != moveClarify && move.Kind != moveRequestCode {
		if q := lastQuestionSentence(res.Reply); q != "" && repeatsRecordedQuestion(state, q) {
			// A repeated question reads as a glitch and would count toward the repeated-
			// question limit, closing the mode early. Swap in the fallback question when it
			// is new; no extra model call.
			if fallbackQuestion != "" && !repeatsRecordedQuestion(state, fallbackQuestion) {
				if idx := strings.LastIndex(res.Reply, q); idx >= 0 {
					res.Reply = strings.TrimSpace(strings.TrimSpace(res.Reply[:idx]) + " " + fallbackQuestion)
				}
				add("re-asked a question already asked (replaced with the fallback question)")
			} else {
				add("re-asked a question already asked")
			}
		}
	}
	lower := strings.ToLower(res.Reply)
	if (mode == modeCode && sess.CodePasted && looksLikeCodeRequest(lower)) || (mode == modeBug && looksLikeCodeRequest(lower)) {
		add("asked for code again or for fixed code")
	}
	var candidate strings.Builder
	for _, m := range sess.modeMessages(mode) {
		if m.Role == "user" {
			candidate.WriteString(m.Content + "\n")
		}
	}
	if found := d5ScopeIssues(state.CurrentWeekNumber, res.Reply, candidate.String()); len(found) > 0 {
		add("mentioned out-of-scope ideas: " + strings.Join(found, ", "))
	}
	if promptsBannedLoopControl(state.CurrentWeekNumber, res.Reply, candidate.String()) {
		add("brought up while True, break or continue")
	}
	if hasUnfilledPlaceholder(res.Reply) {
		add("left a template placeholder")
	}
	if d5SpeaksAsInstructor(res.Reply) {
		add("spoke as an instructor or mentioned the course")
	}
	return res
}
