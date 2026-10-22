package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseBriefDesignExample(t *testing.T) {
	text := `QUALITY: partial
CLARIFICATION: no
EVIDENCE: identified inputs ("the user types the order total"); described the discount step
COVERED: inputs; processing steps
GAPS: output format; invalid input handling
NEXT: Probe how they'd present the result to the customer.
FALLBACK: How would you show the final total to the customer?
AVOID: What inputs does the program need?
RECOMMEND: continue
LEVELS: decomposition=competent; correctness=competent; understanding=not_ready
ISSUES: Last question mentioned dictionaries (out of scope).`
	b, ok := parseBrief(text, modeCode, 4)
	if !ok {
		t.Fatal("brief not ok")
	}
	if b.Quality != "partial" || b.Clarification || b.Recommend != "continue" {
		t.Fatalf("enum fields wrong: %+v", b)
	}
	if !reflect.DeepEqual(b.Covered, []string{"inputs", "processing steps"}) || len(b.Evidence) != 2 {
		t.Fatalf("lists wrong: covered %v evidence %v", b.Covered, b.Evidence)
	}
	want := []gradeLabel{
		{Dimension: dimDecomposition, Level: levelCompetent, AnswerIndex: 4}, {Dimension: dimCorrectness, Level: levelCompetent, AnswerIndex: 4}, {Dimension: dimUnderstanding, Level: levelNotReady, AnswerIndex: 4},
	}
	if !reflect.DeepEqual(b.Levels, want) {
		t.Fatalf("levels = %+v", b.Levels)
	}
	if len(b.Issues) != 1 {
		t.Fatalf("issues = %v", b.Issues)
	}
}

func TestParseBriefTolerance(t *testing.T) {
	text := "Here are my notes:\n**NEXT:** Ask what happens with a negative score.\n- RECOMMEND: Close (enough evidence)\nLEVELS: conceptual = Not Yet Ready, ai_use=exceptional\nISSUES: none"
	b, ok := parseBrief(text, modeConceptual, 2)
	if !ok || b.Next != "Ask what happens with a negative score." {
		t.Fatalf("next = %q ok=%v", b.Next, ok)
	}
	if b.Recommend != "close" {
		t.Fatalf("recommend = %q", b.Recommend)
	}
	if len(b.Levels) != 1 || b.Levels[0].Dimension != dimConceptual || b.Levels[0].Level != levelNotReady {
		t.Fatalf("levels = %+v (ai_use must be dropped in Conceptual)", b.Levels)
	}
	if len(b.Issues) != 0 {
		t.Fatalf("issues = %v, want none", b.Issues)
	}
	if b.Quality != "" {
		t.Fatalf("missing quality should stay empty, got %q", b.Quality)
	}
}

func TestParseBriefFailsWithoutNextOrFallback(t *testing.T) {
	if _, ok := parseBrief("QUALITY: solid\nRECOMMEND: close", modeBug, 1); ok {
		t.Fatal("a brief without NEXT or FALLBACK must count as failed")
	}
}

func TestParseLevelsLine(t *testing.T) {
	got := parseLevelsLine("Sure.\nLEVELS: ai_use=competent; correctness=exceptional", modeCode, 7)
	want := []gradeLabel{{Dimension: dimAIUse, Level: levelCompetent, AnswerIndex: 7}, {Dimension: dimCorrectness, Level: levelExceptional, AnswerIndex: 7}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestParseLevelsLineWithoutPrefix(t *testing.T) {
	// Seen from the live model: the levels-only reply drops the "LEVELS:" prefix.
	got := parseLevelsLine("decomposition=competent; understanding=not_ready", modeCode, 3)
	want := []gradeLabel{{Dimension: dimDecomposition, Level: levelCompetent, AnswerIndex: 3}, {Dimension: dimUnderstanding, Level: levelNotReady, AnswerIndex: 3}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	if got := parseLevelsLine("strategy=competent", modeBug, 1); len(got) != 1 {
		t.Fatalf("bare bug label not parsed: %+v", got)
	}
}

func TestParseCompanyLine(t *testing.T) {
	name, domain, rest, ok := parseCompanyLine("COMPANY: Brightline Bakery | small-batch bakery with online ordering.\nWe take pastry orders online.\nWhat would you do first?")
	if !ok || name != "Brightline Bakery" || domain != "small-batch bakery with online ordering" {
		t.Fatalf("got %q %q ok=%v", name, domain, ok)
	}
	if rest != "We take pastry orders online.\nWhat would you do first?" {
		t.Fatalf("rest = %q", rest)
	}
	if _, _, rest, ok := parseCompanyLine("We take pastry orders online. What first?"); ok || rest == "" {
		t.Fatalf("missing line: ok=%v rest=%q", ok, rest)
	}
	if name, _, _, ok := parseCompanyLine("**COMPANY:** Green Acres Farm - organic vegetable farm\nScenario."); !ok || name != "Green Acres Farm" {
		t.Fatalf("bold/dash form: name=%q ok=%v", name, ok)
	}
}

func TestPersonaIntro(t *testing.T) {
	s := newD5Session("s1")
	s.Personas[modeCode] = d5Persona{Name: "Mei", Role: "software developer"}
	s.CompanyName, s.CompanyDomain = "Brightline Bakery", "small-batch bakery with online ordering"
	want := "Hi, I'm Mei, a software developer at Brightline Bakery, a small-batch bakery with online ordering."
	if got := s.personaIntro(modeCode); got != want {
		t.Fatalf("got %q", got)
	}
	s.Personas[modeBug] = d5Persona{Name: "Omar", Role: "QA engineer"}
	s.CompanyDomain = "An organic farm"
	if got := s.personaIntro(modeBug); got != "Hi, I'm Omar, a QA engineer at Brightline Bakery, an organic farm." {
		t.Fatalf("got %q", got)
	}
}

func TestPickD5PersonasUsesPools(t *testing.T) {
	p := pickD5Personas("session-x")
	for mode, pool := range d5NamePools {
		found := false
		for _, n := range pool {
			found = found || n == p[mode].Name
		}
		if !found || p[mode].Role != d5PersonaRoles[mode] {
			t.Fatalf("mode %s persona %+v not from pool", mode, p[mode])
		}
	}
}

func TestParseTargetLevel(t *testing.T) {
	for _, raw := range []string{"exceptional", "**Exceptional**", "Exceptional.", "LEVEL: exceptional"} {
		got := parseTargetLevel(raw, modeCode, dimAIUse, 9)
		if len(got) != 1 || got[0].Dimension != dimAIUse || got[0].Level != levelExceptional {
			t.Errorf("parseTargetLevel(%q) = %+v", raw, got)
		}
	}
	if got := parseTargetLevel("correctness=competent", modeCode, dimAIUse, 9); len(got) != 1 || got[0].Dimension != dimCorrectness {
		t.Errorf("dimension=level form: %+v", got)
	}
	if got := parseTargetLevel("I think it's fine", modeCode, dimAIUse, 9); len(got) != 0 {
		t.Errorf("no level: %+v", got)
	}
}

func TestUnlabelledAnswersChecksTargetedDimension(t *testing.T) {
	s := newD5Session("t")
	s.Answers[7] = d5Answer{Mode: modeCode, Target: dimCorrectness, Answer: "code"}
	s.addLabels(modeCode, []gradeLabel{{Dimension: dimUnderstanding, Level: levelCompetent, AnswerIndex: 7, Source: labelSourceEvaluator}})
	if _, ok := s.unlabelledAnswers(modeCode)[7]; !ok {
		t.Fatal("a code paste labelled only on understanding still needs a correctness label")
	}
	s.addLabels(modeCode, []gradeLabel{{Dimension: dimCorrectness, Level: levelCompetent, AnswerIndex: 7, Source: labelSourceLevels}})
	if _, ok := s.unlabelledAnswers(modeCode)[7]; ok {
		t.Fatal("answer with its targeted dimension labelled should not be listed")
	}
}

func TestSplitDefectLine(t *testing.T) {
	opening := "The tool prints a total.\n```python\nprint(1 + '2')\n```\nHow would you find what's wrong?\n**DEFECT:** line 1 adds an int to a str, which crashes."
	rest, defect := splitDefectLine(opening)
	if defect != "line 1 adds an int to a str, which crashes." {
		t.Fatalf("defect = %q", defect)
	}
	if strings.Contains(rest, "DEFECT") || !strings.HasSuffix(rest, "How would you find what's wrong?") {
		t.Fatalf("rest = %q", rest)
	}
	if rest, defect := splitDefectLine("No defect line here."); defect != "" || rest != "No defect line here." {
		t.Fatalf("missing line: rest=%q defect=%q", rest, defect)
	}
}

func TestEvaluatorSeesBugDefect(t *testing.T) {
	state, sess := d5TestState(5, modeBug)
	sess.BugDefect = "line 3 uses or instead of and"
	msgs := d5EvaluatorMessages(state, sess, modeBug, 1, dimStrategy, nil, nil)
	if !strings.Contains(msgs[0].Content, "line 3 uses or instead of and") {
		t.Fatal("Evaluator prompt is missing the bug's defect")
	}
	sys := d5InterviewerSystemPrompt(state, sess, modeBug, d5Move{Kind: moveFollowUp, Instruction: "x"}, nil)
	if strings.Contains(sys, "line 3 uses or instead of and") {
		t.Fatal("the interviewer must never see the defect")
	}
}

func TestCorrectnessIsPassFail(t *testing.T) {
	for raw, want := range map[string]string{"works": levelExceptional, "Works.": levelExceptional, "broken": levelNotReady, "**broken**": levelNotReady} {
		got := parseTargetLevel(raw, modeCode, dimCorrectness, 3)
		if len(got) != 1 || got[0].Level != want {
			t.Errorf("parseTargetLevel(%q) = %+v, want %s", raw, got, want)
		}
	}
	s := newD5Session("t")
	s.addLabels(modeCode, []gradeLabel{{Dimension: dimCorrectness, Level: levelCompetent, AnswerIndex: 3, Source: labelSourceEvaluator}})
	if got := s.labelsSnapshot()[modeCode]; len(got) != 1 || got[0].Level != levelExceptional {
		t.Fatalf("competent correctness should count as a pass (exceptional), got %+v", got)
	}
	state, _ := d5TestState(5, modeCode)
	msgs := d5LevelsMessages(state, modeCode, dimCorrectness, "Paste your code", "print(1)", "Data available: a score. What's wanted: a grade.")
	if !strings.Contains(msgs[0].Content, "works or broken") || !strings.Contains(msgs[0].Content, "What's wanted: a grade.") {
		t.Fatalf("correctness prompt should be pass/fail and include the task: %s", msgs[0].Content)
	}
}

func TestSplitDefectLineFirst(t *testing.T) {
	opening := "DEFECT: line 3 uses > instead of >=, so 500 falls through to error.\nThe tool sorts invoices into tiers.\n```python\nif amount > 500:\n    tier = 'high'\n```\nHow would you find what's wrong?"
	rest, defect := splitDefectLine(opening)
	if defect != "line 3 uses > instead of >=, so 500 falls through to error." {
		t.Fatalf("defect = %q", defect)
	}
	if strings.Contains(rest, "DEFECT") || !strings.HasPrefix(rest, "The tool sorts invoices") || !strings.HasSuffix(rest, "What's wrong?") && !strings.HasSuffix(rest, "find what's wrong?") {
		t.Fatalf("rest = %q", rest)
	}
}

func TestDefectLooksBad(t *testing.T) {
	bad := []string{
		"",
		"The snippet is actually correct; no defect exists here.",
		"Line 8 calls strip() which works—wait, that's fine, the real bug is line 7.",
		"There is no bug in this code as written.",
		strings.Repeat("long ", 70),
	}
	for _, d := range bad {
		if !d5DefectLooksBad(d) {
			t.Errorf("should be bad: %q", d)
		}
	}
	good := []string{
		"Line 4 concatenates a string and a float, causing a TypeError.",
		"The range stop is n instead of n + 1, so the last odd number is never added.",
	}
	for _, d := range good {
		if d5DefectLooksBad(d) {
			t.Errorf("should be good: %q", d)
		}
	}
}

func TestBugStrategyMessagesRateTheWholeConversation(t *testing.T) {
	state, sess := d5TestState(5, modeBug)
	sess.BugDefect = "line 2 uses or instead of and"
	askAndAnswer(state, sess, d5Move{Kind: moveOpenMode, Target: dimStrategy}, "Where would you start?", "Reproduce with 1 hour sitting and 0 meetings, then check each clause.")
	askAndAnswer(state, sess, d5Move{Kind: moveFollowUp, Target: dimStrategy}, "Which input made it obvious?", "1 and 0: only one clause is true.")
	msgs := d5BugStrategyMessages(state, sess)
	if len(msgs) != 2 {
		t.Fatalf("want a system and a user message, got %d", len(msgs))
	}
	for _, want := range []string{"whole", "does not cancel", "not_ready, competent or exceptional", "line 2 uses or instead of and", "never mark an answer down for not finding the defect", "Exceptional"} {
		if !strings.Contains(msgs[0].Content, want) {
			t.Errorf("system prompt lacks %q:\n%s", want, msgs[0].Content)
		}
	}
	for _, want := range []string{"Where would you start?", "Reproduce with 1 hour sitting", "Which input made it obvious?", "1 and 0: only one clause"} {
		if !strings.Contains(msgs[1].Content, want) {
			t.Errorf("transcript lacks %q:\n%s", want, msgs[1].Content)
		}
	}
}

func TestLastAnswerIndex(t *testing.T) {
	state, sess := d5TestState(5, modeBug)
	if got := sess.lastAnswerIndex(modeBug); got != 0 {
		t.Fatalf("no answers: got %d", got)
	}
	askAndAnswer(state, sess, d5Move{Kind: moveOpenMode, Target: dimStrategy}, "Where would you start?", "Reproduce it.")
	askAndAnswer(state, sess, d5Move{Kind: moveFollowUp, Target: dimStrategy}, "Which input?", "1 and 0.")
	if got := sess.lastAnswerIndex(modeBug); got != 2 {
		t.Fatalf("two Bug answers: got %d", got)
	}
	if got := sess.lastAnswerIndex(modeCode); got != 0 {
		t.Fatalf("no Code answers: got %d", got)
	}
}
