package main

import (
	"strings"
	"testing"
)

func TestParseGeneratedProblem(t *testing.T) {
	content := "TITLE: Clinic wait-time checker\nPROBLEM:\nA clinic wants to flag long waits.\n\n**Data available:** the wait in minutes\n**What's wanted:** a message\n\nFirst, how would you break this problem down?\nREFERENCE:\nInput: wait. Process: compare. Output: message.\n```python\nif wait > 30:\n    print('long')\n```"
	p, err := parseGeneratedProblem(content)
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "Clinic wait-time checker" {
		t.Errorf("title = %q", p.Title)
	}
	if !strings.HasPrefix(p.Problem, "A clinic wants") || strings.Contains(p.Problem, "REFERENCE") || strings.Contains(p.Problem, "if wait") {
		t.Errorf("problem = %q", p.Problem)
	}
	if !strings.HasPrefix(p.Reference, "Input: wait") || !strings.Contains(p.Reference, "if wait > 30") {
		t.Errorf("reference = %q", p.Reference)
	}

	// Bold markers and a missing title are tolerated.
	p, err = parseGeneratedProblem("**PROBLEM:**\nWhat does `x // 2` give when x is 7?\n\n**REFERENCE:** 3, because // drops the remainder.")
	if err != nil {
		t.Fatal(err)
	}
	if p.Title == "" || p.Reference != "3, because // drops the remainder." {
		t.Errorf("parsed = %+v", p)
	}

	for _, bad := range []string{"", "Just a question with no markers?", "PROBLEM:\nsomething", "REFERENCE:\nkey\nPROBLEM:\nlate"} {
		if _, err := parseGeneratedProblem(bad); err == nil {
			t.Errorf("parseGeneratedProblem(%q) succeeded", bad)
		}
	}
}

func TestProblemIssues(t *testing.T) {
	w5 := weekInfo(5)
	good := generatedProblem{Problem: "A lab needs a pH label.\n\n**Data available:** a pH reading\n**What's wanted:** acidic, neutral or basic\n\nFirst, how would you break this problem down?", Reference: "x"}
	if issues := problemIssues(w5, kindCode, good); len(issues) != 0 {
		t.Errorf("good code problem flagged: %q", issues)
	}
	withCode := good
	withCode.Problem += "\nStart with `ph = float(input())`."
	if issues := problemIssues(w5, kindCode, withCode); len(issues) == 0 {
		t.Error("code problem containing code was not flagged")
	}
	noLines := generatedProblem{Problem: "Write a program that labels pH values.", Reference: "x"}
	if issues := problemIssues(w5, kindCode, noLines); len(issues) == 0 {
		t.Error("code problem without Data available / What's wanted was not flagged")
	}
	noSnippet := generatedProblem{Problem: "Something is wrong with the grader. How would you find it?", Reference: "x"}
	if issues := problemIssues(w5, kindBug, noSnippet); len(issues) == 0 {
		t.Error("bug hunt without a program was not flagged")
	}
	loop := generatedProblem{Problem: "```python\nfor score in range(3):\n    print(score)\n```\nHow would you track down this bug?", Reference: "x"}
	issues := problemIssues(w5, kindBug, loop)
	if len(issues) != 1 || !strings.Contains(issues[0], "loops (week 6)") {
		t.Errorf("week 5 bug hunt with a for loop: %q", issues)
	}
	week1 := generatedProblem{Problem: "```python\nx = 1\n```", Reference: "x"}
	if issues := problemIssues(weekInfo(1), kindConcept, week1); len(issues) == 0 {
		t.Error("week 1 problem with code was not flagged")
	}
}

func TestSplitLevel(t *testing.T) {
	cases := []struct {
		in, visible, level string
	}{
		{"Nice work.\n**Interview check:** Competent.\nLEVEL: competent", "Nice work.\n**Interview check:** Competent.", levelCompetent},
		{"Great.\n\n**LEVEL:** exceptional", "Great.", levelExceptional},
		{"Hmm.\nLEVEL: not_ready", "Hmm.", levelNotReady},
		{"Hmm.\nLEVEL: Not Ready Yet", "Hmm.", levelNotReady},
		{"Good question! Try again.\nLEVEL: none", "Good question! Try again.", ""},
		{"No level line at all.", "No level line at all.", ""},
		{"LEVEL: competent\nmore text\nLEVEL: exceptional", "more text", levelExceptional},
	}
	for _, c := range cases {
		visible, level := splitLevel(c.in)
		if visible != c.visible || level != c.level {
			t.Errorf("splitLevel(%q) = %q, %q; want %q, %q", c.in, visible, level, c.visible, c.level)
		}
	}
}

func TestLooksLikeCode(t *testing.T) {
	for _, code := range []string{
		"```python\nprint('hi')\n```",
		"total = price * qty\nprint(total)",
		"if score >= 90:\n    grade = 'A'",
		"print(total)",
	} {
		if !looksLikeCode(code) {
			t.Errorf("looksLikeCode(%q) = false", code)
		}
	}
	for _, prose := range []string{
		"Input: the prices. Process: add them up. Output: the total.",
		"If the total is over 100, apply the discount; otherwise print the total.",
		"I would first read the score, then compare it with 90.",
	} {
		if looksLikeCode(prose) {
			t.Errorf("looksLikeCode(%q) = true", prose)
		}
	}
}

func TestNextStage(t *testing.T) {
	cases := []struct {
		kind, stage string
		code        bool
		want        string
	}{
		{kindConcept, stageAnswer, false, stageFollowUp},
		{kindConcept, stageFollowUp, false, stageDone},
		{kindCode, stageDecompose, false, stageCode},
		{kindCode, stageDecompose, true, stageExplain},
		{kindCode, stageCode, true, stageExplain},
		{kindCode, stageCode, false, stageCode},
		{kindCode, stageExplain, false, stageDone},
		{kindBug, stageStrategy, false, stageFix},
		{kindBug, stageFix, false, stageDone},
	}
	for _, c := range cases {
		if got := nextStage(&activity{Kind: c.kind, Stage: c.stage}, c.code); got != c.want {
			t.Errorf("%s/%s code=%v: got %s, want %s", c.kind, c.stage, c.code, got, c.want)
		}
	}
}
