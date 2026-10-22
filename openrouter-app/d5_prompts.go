package main

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Bug Hunting level descriptions are read from the reviewed drafts until they move into
// the weekly rubrics (design §8.1).
//
//go:embed D5-bug-rubric-drafts.md
var bugRubricDrafts string

const d5OpeningMaxTokens = 300
const d5BugOpeningMaxTokens = 400
const d5ReplyMaxTokens = 250

// d5CoachingMaxTokens is a safety cap, not the target: the prompt asks for under 250
// words. 500 cut about one reply in eight off mid-sentence.
const d5CoachingMaxTokens = 800

// markdownSection returns the "## " section whose title starts with titlePrefix, up to
// the next "## " heading, or "".
func markdownSection(md, titlePrefix string) string {
	lines := strings.Split(md, "\n")
	var out []string
	in := false
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			if in {
				break
			}
			in = strings.HasPrefix(strings.TrimPrefix(line, "## "), titlePrefix)
		}
		if in {
			out = append(out, line)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// withoutSections drops the "## " sections whose titles start with any of the prefixes.
func withoutSections(md string, prefixes ...string) string {
	var out []string
	skip := false
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(line, "## ") {
			title := strings.TrimPrefix(line, "## ")
			skip = false
			for _, p := range prefixes {
				if strings.HasPrefix(title, p) {
					skip = true
				}
			}
		}
		if !skip {
			out = append(out, line)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func readInstructionFile(path string) string {
	data, err := instructionFS.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// d5RubricForMode returns the level descriptions the Evaluator judges a mode against.
// Grading rules ("Overall Guidance", rating-label rules) are left out: Go applies those.
func d5RubricForMode(week int, mode string) string {
	if mode == modeBug {
		shared := markdownSection(bugRubricDrafts, "Shared descriptions")
		weekly := markdownSection(bugRubricDrafts, fmt.Sprintf("Week %d ", week))
		return strings.TrimSpace(shared + "\n\n" + weekly)
	}
	rubric := readInstructionFile(fmt.Sprintf("env/rubrics/week%d_rubric.md", week))
	return withoutSections(rubric, "Overall Guidance", "Allowed rating labels")
}

// bugSuitableDefects returns the week's "Suitable defects" note from the Bug drafts.
func bugSuitableDefects(week int) string {
	section := markdownSection(bugRubricDrafts, fmt.Sprintf("Week %d ", week))
	start := strings.Index(section, "**Suitable defects**")
	if start < 0 {
		return ""
	}
	rest := section[start:]
	if end := strings.Index(rest, "\n**What strategy"); end >= 0 {
		rest = rest[:end]
	}
	return strings.TrimSpace(strings.ReplaceAll(rest, "**", ""))
}

// weekFocus lists the selected week's topics from its key-concepts file, for the opening
// focus line (design §4, variants).
func weekFocus(state *AgentSessionState) string {
	text := readInstructionFile(fmt.Sprintf("env/IPYIntervu_support_files/week%d_key_concepts.md", state.CurrentWeekNumber))
	section := markdownSection(text, "Topics Covered")
	var topics []string
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- ") {
			topics = append(topics, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- ")))
		}
	}
	if len(topics) == 0 {
		return state.SelectedKeyConcept
	}
	return strings.Join(topics, ", ")
}

func weekGuide(week int) string {
	return strings.TrimSpace(readInstructionFile(fmt.Sprintf("env/IPYIntervu_support_files/week%d_key_concepts.md", week)) +
		"\n\n" + readInstructionFile(fmt.Sprintf("env/IPYIntervu_support_files/week%d_competency_guide.md", week)))
}

func allowedPythonIdeas(week int) string {
	var ideas []string
	for w := 2; w <= week && w <= lastSyllabusWeek; w++ {
		ideas = append(ideas, conceptsIntroducedByWeek[w]...)
	}
	return strings.Join(ideas, ", ")
}

// taskLimits states, in plain words, what a Code task or Bug snippet must not need at
// this week. The scope regexes catch later-week words; these catch later-week ideas a task
// needs without naming them (a "single or multiple" output needs if/else).
func taskLimits(week int) []string {
	var limits []string
	if week < 3 {
		limits = append(limits, "The program cannot ask the user for anything; every value is already set in the program.")
	}
	if week < 5 {
		limits = append(limits, "The program does exactly the same steps for every input: no choosing between different outputs or messages (nothing like \"if more than…\", \"otherwise…\", or picking one word or another).")
	}
	if week < 6 {
		limits = append(limits, "It handles one record or one set of values only: no repeating steps and no working through many items.")
	}
	if week < 7 {
		limits = append(limits, "No menus and no repeating until the user quits.")
	}
	if week < 8 {
		limits = append(limits, "No collection of several values stored to work through later.")
	}
	if week < 9 {
		limits = append(limits, "No files: data is typed in by the user or set in the program. Never mention files, batches, exports or stored records.")
	}
	return limits
}

func loopBanApplies(week int) bool {
	return week >= 7
}

const d5LoopNote = " Never ask for or suggest `while True`, `break` or `continue`; if the candidate uses them, ask why they chose that."

func modeLabel(mode string) string {
	switch mode {
	case modeConceptual:
		return "conceptual"
	case modeCode:
		return "coding"
	case modeBug:
		return "debugging"
	default:
		return "interview"
	}
}

type d5MoveKind string

const (
	moveOpenFirst       d5MoveKind = "OPEN_FIRST"
	moveOpenMode        d5MoveKind = "OPEN_MODE"
	moveFollowUp        d5MoveKind = "FOLLOW_UP"
	moveClarify         d5MoveKind = "CLARIFY"
	moveRedirectVague   d5MoveKind = "REDIRECT_VAGUE"
	moveRequestCode     d5MoveKind = "REQUEST_CODE"
	moveCodeFollowUp    d5MoveKind = "CODE_FOLLOW_UP"
	moveCloseMode       d5MoveKind = "CLOSE_MODE"
	moveCoaching        d5MoveKind = "COACHING"
	moveCoachingLater   d5MoveKind = "COACHING_DEFERRED"
	moveResultsReminder d5MoveKind = "RESULTS_REMINDER"
)

// d5Move is the director's decision for one student message.
type d5Move struct {
	Kind        d5MoveKind
	Instruction string
	// Target is the rubric dimension the question in this reply targets.
	Target    string
	MaxTokens int
	Reason    string
}

func (m d5Move) isOpening() bool {
	return m.Kind == moveOpenFirst || m.Kind == moveOpenMode
}

// openingInstruction is the move text for a mode's first reply. needsCompany asks the
// model for the COMPANY line (first opening only).
func openingInstruction(state *AgentSessionState, mode string, needsCompany bool) string {
	var b strings.Builder
	switch mode {
	case modeConceptual:
		if needsCompany {
			fmt.Fprintf(&b, "Start the interview. Your reply must begin with one line in exactly this form:\nCOMPANY: <company name> | <a few words on what the company does>\nChoose a realistic company where someone with a %s background might work. ", state.StudentMajor)
		} else {
			b.WriteString("Start the interview. ")
		}
		b.WriteString("Do not introduce yourself; that has been done. ")
		if state.isProblemDecompositionWeek() {
			b.WriteString("Describe one realistic work problem at the company (2–4 sentences) that someone would need to break into inputs, steps and outputs, then ask how they would start breaking it down. Do not break it down yourself and do not mention code.")
		} else {
			fmt.Fprintf(&b, "Describe one realistic work situation at the company (2–4 sentences) that connects to %s, then ask one conceptual question about it. No code.", weekFocus(state))
		}
	case modeCode:
		fmt.Fprintf(&b, "Start the coding part of the interview. Do not introduce yourself; that has been done. Present one small programming task as a short story problem from work at the company (2–3 sentences about who needs what and why). Then add these two lines, written in plain everyday words:\nData available: <the information the program will have>\nWhat's wanted: <what the program should produce>\n\"What's wanted\" describes the result the user sees, never how the program works (not \"a loop that…\"). Never write code anywhere in this reply: no Python, no variable names, no function calls such as input() or int(), no code formatting, and no steps, hints or solution. Start directly with the story; no filler such as \"Great\". The task should exercise %s and be solvable with only these Python ideas: %s. Then ask one question: how they would break the problem down before writing any code.",
			weekFocus(state), allowedPythonIdeas(state.CurrentWeekNumber))
		if limits := taskLimits(state.CurrentWeekNumber); len(limits) > 0 {
			b.WriteString("\nThe task must stay within these limits:\n- " + strings.Join(limits, "\n- "))
		}
	case modeBug:
		fmt.Fprintf(&b, "Start the debugging part of the interview. Do not introduce yourself; that has been done. Plan the bug before writing anything else: your reply must begin with one line in exactly this form, which the candidate will never see:\nDEFECT: <the single bug you will put in the code, the line it will be on, and what goes wrong, in one sentence>\nThen start directly with the description; no filler such as \"Great\", \"Sure\" or \"Got it\". Say in one sentence what a small internal tool at the company is supposed to do. Then show a short Python snippet (5–12 lines) in a ```python block with exactly one defect related to %s. The code must not work correctly as written: the defect must make it crash or give a wrong result for some input. There must be only one defect; everything else must be correct. Use only these Python ideas: %s. Write the code so that it really contains the bug named in your DEFECT line. Mark the bug on the line where it is with a trailing comment of the form \"# Bug: <what is wrong on this line>\" (for example: total = total + count / 2  # Bug: divides only count, not the sum). Use no other comments. The candidate is not being tested on spotting the bug; they are being asked how they would go about finding a bug like this one. Then ask one question: how they would go about finding this bug.",
			weekFocus(state), allowedPythonIdeas(state.CurrentWeekNumber))
		if limits := taskLimits(state.CurrentWeekNumber); len(limits) > 0 {
			b.WriteString("\nThe snippet and its purpose must stay within these limits:\n- " + strings.Join(limits, "\n- "))
		}
		if defects := bugSuitableDefects(state.CurrentWeekNumber); defects != "" {
			b.WriteString("\nIdeas for the defect (pick one and adapt it to the company):\n" + defects)
		}
	}
	b.WriteString("\nFor this opening reply only, you may use up to 150 words plus any code block.")
	return b.String()
}

// d5InterviewerSystemPrompt builds the Interviewer prompt (design §4): the static persona
// block first so prefix caching can hit, then the dynamic notes and the move.
func d5InterviewerSystemPrompt(state *AgentSessionState, sess *d5Session, mode string, move d5Move, brief *d5Brief) string {
	p := sess.Personas[mode]
	var b strings.Builder
	if sess.CompanyName != "" {
		fmt.Fprintf(&b, "You are %s, %s at %s", p.Name, withArticle(p.Role), sess.CompanyName)
		if sess.CompanyDomain != "" {
			fmt.Fprintf(&b, " (%s)", sess.CompanyDomain)
		}
		b.WriteString(".\n")
	} else {
		fmt.Fprintf(&b, "You are %s, %s at a company in the candidate's field; you will name the company.\n", p.Name, withArticle(p.Role))
	}
	week1 := state.isProblemDecompositionWeek()
	if week1 {
		fmt.Fprintf(&b, "You are interviewing a candidate with a %s background for an entry-level role. This conversation is about breaking real-world problems into inputs, steps and outputs; never discuss code or Python.\n", state.StudentMajor)
	} else {
		fmt.Fprintf(&b, "You are interviewing a candidate with a %s background for an entry-level role that uses Python.\n", state.StudentMajor)
	}
	b.WriteString(`
How you speak:
- A warm, professional interviewer and a colleague at the company, not a teacher.
  Never mention weeks, courses, classes, homework, or what the candidate has studied.
- Respond naturally to what the candidate just said in one or two sentences, then ask
  exactly ONE question. Then stop.
- Never say whether an answer was right, explain, hint, give sample answers, or answer
  your own question. Never write the candidate's reply.
- Use only details the candidate actually gave. Don't introduce yourself again,
  restate the scenario, or say you are moving on or wrapping up.
- Never repeat one of your earlier questions word for word. If the candidate didn't
  answer it, come at the same point from a different angle.
`)
	if !week1 {
		fmt.Fprintf(&b, "- Use only these Python ideas: %s. Never mention any other Python\n  feature, even to rule it out. Never mention dictionaries. If the candidate uses\n  something not on this list, that is fine; you may ask them what it does.", allowedPythonIdeas(state.CurrentWeekNumber))
		if loopBanApplies(state.CurrentWeekNumber) {
			b.WriteString(d5LoopNote)
		}
		b.WriteString("\n")
	}
	b.WriteString("- Plain text, under 90 words.\n\n--- (dynamic below this line) ---\n")

	if material := sess.Material[mode]; material != "" {
		b.WriteString("Scenario: " + material + "\n")
	}
	if !move.isOpening() {
		var notes []string
		if brief != nil && brief.Mode == mode {
			if brief.Next != "" {
				notes = append(notes, brief.Next)
			}
			if len(brief.Issues) > 0 {
				notes = append(notes, "Correct this in your reply: "+strings.Join(brief.Issues, "; ")+".")
			}
		}
		avoid := append([]string(nil), state.ModeQuestionsAsked...)
		if brief != nil && brief.Mode == mode {
			avoid = append(avoid, brief.Avoid...)
		}
		if len(avoid) > 0 {
			notes = append(notes, "Do not re-ask: "+strings.Join(avoid, " | "))
		}
		if len(notes) > 0 {
			b.WriteString("Colleague's private notes: " + strings.Join(notes, " ") + "\n")
		}
	}
	b.WriteString("Your task for this reply: " + move.Instruction + "\n")
	return b.String()
}

// d5InterviewerMessages returns the system prompt plus the current mode's last six
// transcript messages. Openings get a short user turn so the chat starts with the user.
func d5InterviewerMessages(system string, sess *d5Session, mode string, opening bool) []chatMessage {
	msgs := []chatMessage{{Role: "system", Content: system}}
	if opening {
		return append(msgs, chatMessage{Role: "user", Content: "I'm ready to begin."})
	}
	history := sess.modeMessages(mode)
	if len(history) > 6 {
		history = history[len(history)-6:]
	}
	for _, m := range history {
		msgs = append(msgs, chatMessage{Role: m.Role, Content: m.Content})
	}
	return msgs
}

func transcriptText(sess *d5Session, mode string) string {
	var b strings.Builder
	name := sess.Personas[mode].Name
	for _, m := range sess.modeMessages(mode) {
		if m.Role == "assistant" {
			fmt.Fprintf(&b, "Interviewer (%s): %s\n\n", name, m.Content)
		} else {
			fmt.Fprintf(&b, "Candidate: %s\n\n", m.Content)
		}
	}
	return strings.TrimSpace(b.String())
}

func targetDescription(dim string) string {
	switch dim {
	case dimConceptual:
		return "conceptual understanding"
	case dimDecomposition:
		return "decomposition (breaking the problem down)"
	case dimCorrectness:
		return "correctness of their code"
	case dimUnderstanding:
		return "understanding of their own code"
	case dimAIUse:
		return "AI use (how they used and checked AI tools)"
	case dimStrategy:
		return "debugging strategy"
	default:
		return dim
	}
}

func progressSentences(state *AgentSessionState, sess *d5Session, mode string) string {
	asked := len(state.ModeQuestionsAsked)
	switch mode {
	case modeConceptual:
		return fmt.Sprintf("Questions asked in this part: %d (3–5 expected; the part closes automatically at %d). Vague answers so far: %d.", asked, maxConceptualQuestions, state.ModeVagueAnswers)
	case modeCode:
		pasted := "no"
		if sess.CodePasted {
			pasted = "yes"
		}
		return fmt.Sprintf("Code pasted: %s. Questions since the paste: %d of up to %d (one should be about AI use). Vague answers so far: %d.", pasted, len(postCodeQuestions(state)), maxPostCodeQuestions, state.ModeVagueAnswers)
	case modeBug:
		return fmt.Sprintf("Questions asked in this part: %d of %d. Vague answers so far: %d.", asked, maxBugQuestions, state.ModeVagueAnswers)
	}
	return ""
}

func briefLines(b *d5Brief) string {
	if b == nil {
		return ""
	}
	var lines []string
	add := func(label, value string) {
		if value != "" {
			lines = append(lines, label+": "+value)
		}
	}
	add("COVERED", strings.Join(b.Covered, "; "))
	add("GAPS", strings.Join(b.Gaps, "; "))
	add("NEXT", b.Next)
	return strings.Join(lines, "\n")
}

// d5EvaluatorMessages builds the Evaluator call (design §5): rubric and guide first, the
// transcript last. issues are post-check hits on the interviewer's latest reply.
func d5EvaluatorMessages(state *AgentSessionState, sess *d5Session, mode string, answerIndex int, target string, prev *d5Brief, issues []string) []chatMessage {
	var b strings.Builder
	fmt.Fprintf(&b, `You are a senior interviewer reviewing a mock job interview for an entry-level Python role. You never talk to the candidate. You write private notes for the interviewer, in exactly the line format at the end.

Part of the interview: %s. Topic: %s. Candidate's background: %s.
`, modeLabel(mode), state.SelectedKeyConcept, state.StudentMajor)
	if !state.isProblemDecompositionWeek() {
		fmt.Fprintf(&b, "Python ideas the interviewer may use: %s. Never steer toward anything else; dictionaries are never allowed.", allowedPythonIdeas(state.CurrentWeekNumber))
		if loopBanApplies(state.CurrentWeekNumber) {
			b.WriteString(" Never steer toward `while True`, `break` or `continue`.")
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "\nRubric level descriptions for this part (use them to judge; never quote them):\n%s\n\nGuide for this topic:\n%s\n\n", d5RubricForMode(state.CurrentWeekNumber, mode), weekGuide(state.CurrentWeekNumber))
	if mode == modeBug && sess.BugDefect != "" {
		fmt.Fprintf(&b, "The snippet's deliberate defect (it is marked on its line in the snippet the candidate sees), for context only: %s\nRate the debugging strategy itself against the rubric. A sound, systematic strategy is competent or better even if it has not reached this defect yet; never mark an answer down for not finding the defect.\n\n", sess.BugDefect)
	}
	b.WriteString(d5Calibration(state) + "\n")
	b.WriteString("If the candidate uses Python features beyond what they have studied, that is fine as long as they can explain what it does; judge their explanation, never the choice.\n")
	b.WriteString("Candidates are expected and encouraged to use AI tools; if they say they did not use AI, label ai_use competent (never higher).\n")
	if mode == modeCode {
		b.WriteString("Code is assessed for the candidate's understanding, not its quality. correctness is pass/fail: exceptional if the code runs and produces the correct output for the task, not_ready if it errors or gives wrong output; never competent, and ignore style, names, comments, robustness and edge cases. Judge understanding by how well the candidate explains the code, not by how the code is written.\n")
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "Progress: %s\n", progressSentences(state, sess, mode))
	fmt.Fprintf(&b, "Assess the candidate's latest answer (answer #%d). The question it answered targeted: %s.\n", answerIndex, targetDescription(target))
	if prev != nil && prev.Mode == mode {
		fmt.Fprintf(&b, "Your previous notes:\n%s\n", briefLines(prev))
	}
	if len(issues) > 0 {
		fmt.Fprintf(&b, "Problems detected in the interviewer's latest reply: %s\n", strings.Join(issues, "; "))
	}
	dims := strings.Join(modeDimensions[mode], ", ")
	fmt.Fprintf(&b, `
Write exactly these lines, in this order, and nothing else:
QUALITY: vague | partial | solid | strong
CLARIFICATION: yes | no (did the latest message ask for clarification instead of answering?)
EVIDENCE: what the latest answer showed, as short items separated by ;
COVERED: points the candidate has shown so far in this part, separated by ;
GAPS: points not yet shown, separated by ;
NEXT: one sentence on what the next question should probe, on new ground, without giving answers away
FALLBACK: one complete interview question the interviewer could ask next
AVOID: questions already asked that must not be repeated, separated by ;
RECOMMEND: continue | close (close only when there is enough evidence to judge this part)
LEVELS: dimension=level for the latest answer only. Dimensions: %s. Levels: not_ready, competent, exceptional. Always include the targeted dimension (%s). Add another dimension only if the answer directly addresses it; never label a dimension the answer did not discuss.
ISSUES: rule problems in the interviewer's latest reply (more than one question, giving away answers, out-of-scope ideas, teaching), or none
`, dims, target)
	return []chatMessage{
		{Role: "system", Content: b.String()},
		{Role: "user", Content: "Interview transcript for this part:\n\n" + transcriptText(sess, mode) +
			"\n\n---\nThe transcript ends here. Do not continue the interview or repeat these instructions. Write your private notes about the candidate's latest answer now, in exactly the line format given, starting with QUALITY:"},
	}
}

// d5VerifyDefectMessages asks whether a Bug snippet really contains a bug: "yes: <the
// actual bug>" or "no: <what the code does>". Some snippets confidently claimed bugs
// their code did not have; others had a real bug their DEFECT line described wrongly, so
// the check names the actual bug rather than verifying the claim.
func d5VerifyDefectMessages(snippet, defect string) []chatMessage {
	system := `You check deliberately buggy Python snippets written for a debugging interview. Each should contain one real bug: it crashes (including a syntax error), loops forever, or gives a wrong result for some realistic input, compared with what the description says the tool should do.
Trace the code exactly as written, line by line. The author's note about the bug is a hint and may be inaccurate.
Reply with exactly "yes: <the actual bug, its line, and what goes wrong, in one sentence>" if the code really has such a bug, or "no: <what the code actually does>" if it works as described.`
	return []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: "Snippet:\n" + snippet + "\n\nAuthor's note about the bug: " + defect},
	}
}

// d5Calibration tells the labellers whom they are judging: an introductory Python
// student at the selected week, measured against that week's rubric, not a professional.
// Without it every answer came back Competent.
func d5Calibration(state *AgentSessionState) string {
	return fmt.Sprintf(`Calibration: the candidate is an introductory Python student who has studied %s. Judge each answer against the rubric descriptions for this week, not against what a professional developer would say.
- exceptional: the answer matches the rubric's Exceptional description for this week (clear, complete, specific, with a reason or example). Award it whenever that is met; do not hold it back for perfection or for things the student has not studied.
- competent: correct and adequate, but missing something the rubric's Exceptional description asks for.
- not_ready: wrong, missing the point of the question, or too vague to judge.
If the interviewer's question was not about this week's topic, the answer is not evidence either way: give no label for it, never not_ready.
AI use: candidates are expected and encouraged to use AI, so never rate an answer lower because they used it; not using AI at all is competent at most. Rate how they used and checked it: not_ready only if they copied without understanding or cannot say how they checked it; competent if they describe their use and some checking; exceptional if they tested deliberately, changed what the AI produced, or used it to deepen their understanding.`, state.SelectedKeyConcept)
}

// d5LevelsMessages builds the small levels-only call made at mode close (design §8). It
// asks for one word; the label goes on the targeted dimension. Asked for "dimension=level"
// the model often answered with the bare level, which was then dropped.
func d5LevelsMessages(state *AgentSessionState, mode, target, question, answer string, task string) []chatMessage {
	if target == dimCorrectness {
		// Correctness is pass/fail: the code only has to run and give the right output.
		system := fmt.Sprintf(`Decide whether a candidate's Python code works for the task below. It works if it runs without errors and produces the correct output for the task. Ignore style, variable names, comments, robustness and unusual edge cases.
Reply with exactly one word: works or broken.

Task:
%s`, task)
		return []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: "Candidate's code:\n" + answer},
		}
	}
	system := fmt.Sprintf(`Rate one interview answer against the rubric below, for this dimension only: %s.
Reply with exactly one word and nothing else: not_ready, competent or exceptional (or none if the question was not about this week's topic).

%s

Rubric:
%s`, targetDescription(target), d5Calibration(state), d5RubricForMode(state.CurrentWeekNumber, mode))
	return []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: "Question: " + question + "\nAnswer: " + answer},
	}
}

// d5BugStrategyMessages builds the one call that rates the whole Bug part. The Bug rubric
// has a single dimension whose four aspects (approach, hypotheses, narrowing down, link to
// the concept) only show across the conversation, and each follow-up question is narrow, so
// per-answer labels held strong debuggers at Competent: no single answer shows a ranked list
// of causes and why each check comes first.
func d5BugStrategyMessages(state *AgentSessionState, sess *d5Session) []chatMessage {
	var b strings.Builder
	b.WriteString("Rate a candidate's debugging strategy across the whole Bug-hunting part of an interview, against the rubric below.\n")
	b.WriteString("The rubric describes the strategy shown over the whole conversation. The interviewer asks narrow follow-up questions, so no single answer shows everything: read all of the candidate's answers together and rate the strategy they showed as a whole. A later short or narrow answer does not cancel what an earlier one showed.\n")
	b.WriteString("Reply with exactly one word and nothing else: not_ready, competent or exceptional.\n\n")
	if sess.BugDefect != "" {
		fmt.Fprintf(&b, "The snippet's deliberate defect, for context only: %s\nRate the strategy itself. A sound, systematic strategy is competent or better even if it has not reached this defect yet; never mark an answer down for not finding the defect.\n\n", sess.BugDefect)
	}
	b.WriteString(d5Calibration(state))
	fmt.Fprintf(&b, "\n\nRubric:\n%s", d5RubricForMode(state.CurrentWeekNumber, modeBug))
	return []chatMessage{
		{Role: "system", Content: b.String()},
		{Role: "user", Content: "Bug-hunting part of the interview:\n\n" + transcriptText(sess, modeBug)},
	}
}

func modeTitle(mode string) string {
	switch mode {
	case modeConceptual:
		return "Conceptual"
	case modeCode:
		return "Code"
	case modeBug:
		return "Bug hunting"
	}
	return mode
}

// d5CoachingSystemPrompt builds the coaching prompt. The coach is a recruiter on the
// company's HR team who coaches interview performance: vagueness, wording, structure,
// strengths and weaknesses in how the candidate answered. Code improvement is out of
// scope; a separate tool helps with learning the code.
func d5CoachingSystemPrompt(state *AgentSessionState, sess *d5Session) string {
	p := sess.Personas[modeCoaching]
	company := sess.CompanyName
	if company == "" {
		company = "the company"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "You are %s, %s at %s. The candidate (%s background) just finished a practice job interview and asked for feedback. You coach interview skills: how clearly and specifically they answered, not the technical content. You are warm, direct and encouraging. Never mention weeks, courses or homework.\n\n", p.Name, withArticle(p.Role), company, state.StudentMajor)
	fmt.Fprintf(&b, "Their results are final; never change them. Report them exactly as given, but never invent a reason for a rating: explain a rating only from what the answers below actually support:\n- Conceptual: %s\n- Code: %s\n- Bug hunting: %s\n- Overall: %s\n\n", state.ConceptualAssessmentBucket, state.CodeAssessmentBucket, state.BugAssessmentBucket, state.FinalRating)

	b.WriteString(d5CoachingTranscript(sess))

	b.WriteString(`Coach the candidate on how they interviewed:
- Point out answers that were actually vague or non-committal, quoting their words briefly, and show how a more specific answer would sound in general terms.
- Point out real wording issues: hedging, filler, unclear structure, answers that didn't address the question asked. Only tentative words are hedging ("maybe", "I guess", "I think", "probably", "sort of", "kind of"); "I will" and "I would" are committed answers, not hedges.
- Name the strengths you see (up to 3). Name weaknesses only where they really appear in their answers (up to 3); never invent one. If their answers were strong, say so plainly and give stretch tips for an even better interview instead.
- The conceptual part deliberately asked for no code, so never fault them for not writing or showing code there.
- Give 1-2 concrete habits for their next interview (for example: answer the question first, then give one example; say how you would check your work).
- Explain briefly how their answers led to each rating, in interview terms (clarity, specificity, completeness), not technical terms. You may say which answers were stronger or weaker, but never cite answer numbers or dimension names.

Accuracy rules:
- Every answer above is shown in full, next to the question it replied to. Judge an answer only against that question, and only by what it says. Never say an answer was cut off, trailed off or was incomplete (an answer marked as clipped is the only exception).
- Put words in quotation marks only if they are copied exactly from an answer above. If you are not sure of the exact words, describe the answer instead of quoting it. Never say a candidate said or did something that the answers above do not show.
- If a part was rated lower than its answers seem to deserve and you cannot find a real weakness in them, do not make one up. Say that the interviewers weighed a few of the answers lower, without guessing which, and give habits that would keep the answers as strong as they are. When asked why a part was rated lower, answer the same way.
- If the candidate challenges a point, check it against the answers above: say so plainly if you misread them, and keep your view if you read them correctly. Do not simply agree.
- Candidates are expected and encouraged to use AI tools. Never criticise someone for using AI, never suggest hiding or downplaying it, and never give it as the reason for a rating. Saying plainly how they used it, and how they checked what it produced, is a strength.
- You do not know the candidate's name; do not address them by any name, and never use your own name or a colleague's as theirs.

Do not coach on code: never suggest code changes, Python features, practice exercises, or how to solve the tasks. If they ask about the code itself, tell them the code-learning tool is the place for that, and bring the conversation back to interviewing.
In your first reply, cover the points above and end with one check-in question. In later replies, answer their questions the same way. Plain text with short paragraphs or bullets, under 250 words. Finish every sentence.`)
	return b.String()
}

// d5CoachingAnswerClip is the longest answer shown to the coach in full. Code pastes can be
// long; a longer one is clipped with a note, so the coach never mistakes the cut for the
// candidate trailing off.
const d5CoachingAnswerClip = 4000

// d5CoachingTranscript lists each part's scenario and every answer in full, next to the
// question it replied to. Per-answer ratings are left out: the coach quoted them back to
// candidates and contradicted the overall results. The coach reads these
// instead of guessing from the candidate's words alone.
func d5CoachingTranscript(sess *d5Session) string {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	var b strings.Builder
	for _, mode := range []string{modeConceptual, modeCode, modeBug} {
		var indexes []int
		for idx, a := range sess.Answers {
			if a.Mode == mode {
				indexes = append(indexes, idx)
			}
		}
		if len(indexes) == 0 {
			continue
		}
		sort.Ints(indexes)
		fmt.Fprintf(&b, "%s part.\n", modeTitle(mode))
		if material := strings.TrimSpace(sess.Material[mode]); material != "" {
			fmt.Fprintf(&b, "What the candidate was given:\n%s\n", material)
		}
		for n, idx := range indexes {
			a := sess.Answers[idx]
			fmt.Fprintf(&b, "\nQuestion %d: %s\nAnswer %d: %s\n", n+1, a.Question, n+1, d5ClipAnswer(a.Answer))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// d5ClipAnswer returns the answer, clipped at a word boundary with an explicit note when
// it is longer than d5CoachingAnswerClip.
func d5ClipAnswer(answer string) string {
	answer = strings.TrimSpace(answer)
	if len(answer) <= d5CoachingAnswerClip {
		return answer
	}
	cut := d5CoachingAnswerClip
	for cut > 0 && !utf8.RuneStart(answer[cut]) {
		cut--
	}
	if i := strings.LastIndexAny(answer[:cut], " \n"); i > cut/2 {
		cut = i
	}
	return answer[:cut] + "\n[clipped here: the rest of this long answer is not shown]"
}
