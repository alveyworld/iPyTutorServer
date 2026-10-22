package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestD5AskedForClarification(t *testing.T) {
	for msg, want := range map[string]bool{
		"what do you mean by risk band?":                                true,
		"Sorry, can you rephrase that?":                                 true,
		"Do you want the steps or the code?":                            true,
		"Would I check the input first? I'd add a print before the if.": false,
		"I'd compare the score to each threshold.":                      false,
		"Would I check the input first?":                                true,
		"I think it's the order of the elif checks?":                    false,
		strings.Repeat("word ", 30) + "?":                               false,
	} {
		if got := d5AskedForClarification(msg); got != want {
			t.Errorf("d5AskedForClarification(%q) = %v, want %v", msg, got, want)
		}
	}
}

func TestD5ScopeFixes(t *testing.T) {
	if found := d5OutOfScopeConcepts(1, "The user types in each client's allergies, and the planner turns them into meals."); len(found) != 0 {
		t.Fatalf("week 1 'user types' flagged: %v", found)
	}
	if found := d5OutOfScopeConcepts(2, "The user types in each client's weight."); len(found) == 0 {
		t.Fatal("week 2 'user types' should still be flagged as input()")
	}
	if found := d5OutOfScopeConcepts(5, "We keep customer files for every patient visit."); len(found) != 0 {
		t.Fatalf("business wording 'customer files' flagged: %v", found)
	}
	if found := d5OutOfScopeConcepts(5, "The script should read the data file each morning."); len(found) == 0 {
		t.Fatal("reading a data file should be flagged before week 9")
	}
	if found := d5OutOfScopeConcepts(9, "Store the totals in a dictionary."); len(found) == 0 {
		t.Fatal("dictionaries are never in scope")
	}
}

func TestPromptsBannedLoopControl(t *testing.T) {
	if !promptsBannedLoopControl(7, "Could you rewrite it with a `while True` loop and a break?", "") {
		t.Fatal("week 7 prompt for while True not flagged")
	}
	if promptsBannedLoopControl(7, "Why did you use `break` there?", "while True:\n    if x == 'q':\n        break") {
		t.Fatal("asking about the candidate's own break should be allowed")
	}
	if promptsBannedLoopControl(6, "Could you use break?", "") {
		t.Fatal("the ban starts in week 7")
	}
	if promptsBannedLoopControl(8, "Let's continue with the next question about your list.", "") {
		t.Fatal("prose 'continue' is not the continue statement")
	}
}

func TestCorrectnessVerdictAllowsWarmth(t *testing.T) {
	if correctnessVerdictPattern.MatchString("Thanks, that's a helpful way to think about it. What would you check next?") {
		t.Fatal("warm phrasing flagged")
	}
	for _, s := range []string{"That's exactly right. What next?", "You're correct about the order.", "Exactly! Now, what about the fee?", "Spot on."} {
		if !correctnessVerdictPattern.MatchString(s) {
			t.Errorf("verdict not flagged: %q", s)
		}
	}
}

func TestD5SimulatedReplyCut(t *testing.T) {
	text := "Thanks for that. How would you test it?\nCandidate: I would try 50 dollars."
	idx := d5SimulatedReplyCut(text)
	if idx < 0 || strings.TrimSpace(text[:idx]) != "Thanks for that. How would you test it?" {
		t.Fatalf("cut at %d", idx)
	}
	if d5SimulatedReplyCut("Here's what I'd like from you: a short plan. What comes first?") >= 0 {
		t.Fatal("prose 'from you:' must not be cut")
	}
}

func TestD5PostCheckAppendsFallbackWhenNoQuestion(t *testing.T) {
	state, sess := d5TestState(5, modeConceptual)
	res := d5PostCheck(state, sess, modeConceptual, d5Move{Kind: moveFollowUp}, "Thanks, that's a thoughtful approach.", "What would happen on the boundary?")
	if !strings.HasSuffix(res.Reply, "What would happen on the boundary?") {
		t.Fatalf("reply = %q", res.Reply)
	}
	if len(res.Issues) == 0 {
		t.Fatal("missing question should be reported")
	}
}

func TestD5PostCheckFlagsInstructorAndPersona(t *testing.T) {
	state, sess := d5TestState(5, modeConceptual)
	sess.Personas[modeConceptual] = d5Persona{Name: "Jordan", Role: "hiring manager"}
	res := d5PostCheck(state, sess, modeConceptual, d5Move{Kind: moveFollowUp}, "As your instructor, I'd like to hear more. Taylor, would you like to begin?", "")
	joined := strings.Join(res.Issues, "|")
	if !strings.Contains(joined, "instructor") || !strings.Contains(joined, "another interviewer") {
		t.Fatalf("issues = %v", res.Issues)
	}
}

func TestD5LooksLikeCodeSubmission(t *testing.T) {
	for msg, want := range map[string]bool{
		"First read the subtotal, then apply the discount if it is over 50, then print the total.": false,
		"I'd loop for each item and add it up.":                                                    false,
		"```python\nprint('hi')\n```":                                                              true,
		"subtotal = float(input('Subtotal: '))\nprint(subtotal * 0.9)":                             true,
		"total = price * qty":                                                                      false,
		"total = float(input('Price: '))":                                                          true,
		"for item in range(count):\n    total += price":                                            true,
	} {
		if got := d5LooksLikeCodeSubmission(msg); got != want {
			t.Errorf("d5LooksLikeCodeSubmission(%q) = %v, want %v", msg, got, want)
		}
	}
}

func TestStripLeadingVerdict(t *testing.T) {
	for in, want := range map[string]string{
		"Exactly. So if a sensor's status can only be active or inactive, what type fits?": "So if a sensor's status can only be active or inactive, what type fits?",
		"That's right! how would you label it?":                                            "How would you label it?",
		"Correct — and what about the ID?":                                                 "And what about the ID?",
		"Thanks, that's helpful. What next?":                                               "Thanks, that's helpful. What next?",
		"Right away, tell me how you'd start.":                                             "Right away, tell me how you'd start.",
	} {
		if got := stripLeadingVerdict(in); got != want {
			t.Errorf("stripLeadingVerdict(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestD5IsVagueAnswer(t *testing.T) {
	for msg, want := range map[string]bool{
		"boolean":                     false,
		"a string":                    false,
		"keep it as a string":         false,
		"idk":                         true,
		"Pass.":                       true,
		"":                            true,
		"not sure, maybe a print":     true,
		"I'm not sure":                true,
		"what do you mean by status?": false,
		"I don't know the cause, but I would add a print before the loop and compare the counter to the total": false,
	} {
		if got := d5IsVagueAnswer(msg); got != want {
			t.Errorf("d5IsVagueAnswer(%q) = %v, want %v", msg, got, want)
		}
	}
}

func TestD5PostCheckReplacesRepeatedQuestion(t *testing.T) {
	state, sess := d5TestState(2, modeConceptual)
	repeated := "Since we want to test these ideas with a quick script, can you picture how you'd structure the assignment and printed summary to make it easy for a teammate to read and adjust later?"
	state.ModeQuestionsAsked = []string{repeated}
	reply := "That's a clear way to keep the assignment and summary together on screen. " + repeated
	fallback := "Which variable names would you choose so a teammate understands them at a glance?"
	res := d5PostCheck(state, sess, modeConceptual, d5Move{Kind: moveFollowUp}, reply, fallback)
	want := "That's a clear way to keep the assignment and summary together on screen. " + fallback
	if res.Reply != want {
		t.Fatalf("reply = %q", res.Reply)
	}
	d5ApplyInterviewerReply(state, sess, d5Move{Kind: moveFollowUp, Target: dimConceptual}, res.Reply)
	if state.ModeSimilarQuestionAsks != 0 {
		t.Fatal("the replaced reply must not count as a repeated question")
	}
}

func TestD5OpeningGivesCode(t *testing.T) {
	if !d5OpeningGivesCode("Our front desk greets members.\nData available: name = input(\"Your name: \"), birth_year = int(input(\"Year you were born: \"))\nWhat's wanted: a greeting.") {
		t.Fatal("code in Data available not detected")
	}
	if !d5OpeningGivesCode("Data available: the `name` and `birth_year` values") {
		t.Fatal("code formatting not detected")
	}
	plain := "Our front desk wants a friendly welcome for new gym members.\nData available: the member's name and the year they were born\nWhat's wanted: a greeting that says how old they turn this year\nHow would you break this problem down before writing any code?"
	if d5OpeningGivesCode(plain) {
		t.Fatal("plain-words story problem flagged")
	}
}

func TestIsdigitScope(t *testing.T) {
	reply := "How would you use `age_text.isdigit()` before converting it?"
	if found := d5OutOfScopeConcepts(3, reply); len(found) == 0 {
		t.Fatal("isdigit() should be out of scope before week 4")
	}
	if found := d5OutOfScopeConcepts(4, reply); len(found) != 0 {
		t.Fatalf("isdigit() should be in scope from week 4, got %v", found)
	}
	for _, m := range []string{"isdigit()", "isalpha()", "isalnum()", "isupper()", "islower()"} {
		if !strings.Contains(allowedPythonIdeas(5), m) {
			t.Fatalf("%s missing from the allowed ideas sent to the interviewer", m)
		}
		if found := d5OutOfScopeConcepts(3, "Try `code."+m+"`."); len(found) == 0 {
			t.Fatalf("%s should be out of scope before week 4", m)
		}
	}
}

func TestTaskLimitsByWeek(t *testing.T) {
	join := func(week int) string { return strings.Join(taskLimits(week), " ") }
	if !strings.Contains(join(4), "no choosing between different outputs") {
		t.Fatal("week 4 tasks must not need a decision")
	}
	if strings.Contains(join(5), "no choosing between different outputs") {
		t.Fatal("week 5 tasks may use decisions")
	}
	if !strings.Contains(join(8), "No files") || strings.Contains(join(9), "No files") {
		t.Fatal("files are allowed only from week 9")
	}
	if len(taskLimits(9)) != 0 {
		t.Fatalf("week 9 has no limits, got %v", taskLimits(9))
	}
}

func TestLenIsWeek8(t *testing.T) {
	reply := "```python\nprint(len(name))\n```"
	if found := d5OutOfScopeConcepts(7, reply); len(found) == 0 {
		t.Fatal("len() should be out of scope before week 8")
	}
	if found := d5OutOfScopeConcepts(8, reply); len(found) != 0 {
		t.Fatalf("len() should be in scope from week 8, got %v", found)
	}
}

func TestUntaughtStringMethodsNeverInScope(t *testing.T) {
	for _, m := range []string{"title", "capitalize", "find", "startswith", "endswith", "join", "lstrip", "rstrip"} {
		reply := "```python\nname = name." + m + "()\n```"
		for _, week := range []int{4, 9} {
			if found := d5OutOfScopeConcepts(week, reply); len(found) == 0 {
				t.Errorf(".%s() should be out of scope in week %d", m, week)
			}
		}
	}
	if found := d5OutOfScopeConcepts(4, "```python\nname = name.strip().upper()\n```"); len(found) != 0 {
		t.Fatalf("taught week 4 methods flagged: %v", found)
	}
}

func TestOpeningProblemsWordCheck(t *testing.T) {
	// The defect check calls the model; a fake upstream confirms every bug.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"yes: line 2 uses < instead of <="}}]}`))
	}))
	defer upstream.Close()
	saved := openRouterURL
	openRouterURL = upstream.URL
	defer func() { openRouterURL = saved }()

	state, _ := d5TestState(5, modeBug)
	p := chatRunParams{state: state, sessionID: "t"}
	snippet := "DEFECT: line 2 uses < instead of <=.\nA tool checks codes.\n```python\ncode = input('Code: ')\nif len(code) < 5:\n    print('short')\n```\nHow would you find what's wrong?"
	if problems, _ := d5OpeningProblems(p, modeBug, snippet, nil, false); !strings.Contains(strings.Join(problems, " "), "len()") {
		t.Fatalf("len() in week 5 should be a problem, got %q", problems)
	}
	clean := "DEFECT: line 2 uses > instead of >=.\nA tool checks scores.\n```python\nscore = int(input('Score: '))\nif score > 90:  # Bug: misses a score of exactly 90\n    print('A')\n```\nHow would you find what's wrong?"
	problems, verified := d5OpeningProblems(p, modeBug, clean, nil, false)
	if len(problems) != 0 || verified != "line 2 uses < instead of <=" {
		t.Fatalf("in-scope snippet: problems %q verified %q", problems, verified)
	}
	if problems, _ := d5OpeningProblems(p, modeBug, "```python\nprint(1)\n```\nWhat's wrong?", nil, false); !strings.Contains(strings.Join(problems, " "), "DEFECT") {
		t.Fatalf("missing DEFECT line not flagged: %q", problems)
	}
	if problems, _ := d5OpeningProblems(p, modeBug, "DEFECT: line 1 is wrong.", nil, false); !strings.Contains(strings.Join(problems, " "), "no code snippet") {
		t.Fatalf("a Bug opening with no code must be a problem: %q", problems)
	}
	rambling := strings.Replace(clean, "# Bug: misses a score of exactly 90", "# Bug: uses > — wait, that's fine", 1)
	if problems, _ := d5OpeningProblems(p, modeBug, rambling, nil, false); !strings.Contains(strings.Join(problems, " "), "argued with itself") {
		t.Fatalf("a self-admitting bug marker must be a problem: %q", problems)
	}
	unmarked := strings.Replace(clean, "  # Bug: misses a score of exactly 90", "", 1)
	if problems, _ := d5OpeningProblems(p, modeBug, unmarked, nil, false); !strings.Contains(strings.Join(problems, " "), "did not mark the bug") {
		t.Fatalf("a snippet without the bug marked must be a problem: %q", problems)
	}
}

func TestD5UntaughtCallsWhitelist(t *testing.T) {
	snippet := "```python\nwith open('t.txt') as f:\n    lines = f.readlines()\npairs = sorted(zip(counts, titles), reverse=True)\nfor c, t in pairs:\n    print(t.title(), round(c))\n```"
	got := strings.Join(d5UntaughtCalls(9, snippet, ""), " ")
	for _, want := range []string{"sorted()", "zip()", ".title()", "round()"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %q", want, got)
		}
	}
	for _, taught := range []string{"open()", "readlines", "print()"} {
		if strings.Contains(got, taught) {
			t.Errorf("taught call %s flagged in %q", taught, got)
		}
	}
	if found := d5UntaughtCalls(5, "```python\nn = len(name)\nif n > 3:\n    print(name.upper())\n```", ""); strings.Join(found, " ") != "len()" {
		t.Errorf("week 5: got %v, want only len()", found)
	}
	if found := d5UntaughtCalls(4, "```python\ndef greet():\n    return 'hi'\n```", ""); !strings.Contains(strings.Join(found, " "), "def") {
		t.Errorf("def not flagged: %v", found)
	}
	// The interviewer may ask about calls the candidate used.
	if found := d5UntaughtCalls(8, "Why did you use `sorted(names)` there?", "names = sorted(names)"); len(found) != 0 {
		t.Errorf("candidate's own sorted() flagged: %v", found)
	}
}

func TestD5SaysNoAI(t *testing.T) {
	for msg, want := range map[string]bool{
		"No.":                      true,
		"Nope, I wrote it myself.": true,
		"I didn't use any AI tools—I wrote the code directly and traced a few examples.": true,
		"I wrote it without AI and tested it with 0 and 100.":                            true,
		"Yes, I used an AI tool to draft the code, but I checked it carefully.":          false,
		"I didn't use AI for the logic, but I asked ChatGPT about the syntax.":           false,
		"I did use a quick AI tool to double-check the syntax.":                          false,
		"Not really sure what you mean by tools.":                                        false,
	} {
		if got := d5SaysNoAI(msg); got != want {
			t.Errorf("d5SaysNoAI(%q) = %v, want %v", msg, got, want)
		}
	}
}

func TestNoAIAnswerIsCappedAtCompetent(t *testing.T) {
	state, sess := d5TestState(7, modeCode)
	sess.CodePasted = true
	state.ModeInterviewStep = interviewStepCodeSubmitted
	askAndAnswer(state, sess, d5Move{Kind: moveCodeFollowUp, Target: dimAIUse}, "Did you use any AI tools, and how did you check them?", "No.")
	idx := sess.AnswerIndex
	if sess.VagueAnswers[idx] {
		t.Fatal(`"No." to the AI question is an answer, not a vague one`)
	}
	sess.addLabels(modeCode, []gradeLabel{{Dimension: dimAIUse, Level: levelExceptional, AnswerIndex: idx, Source: labelSourceEvaluator}})
	got := sess.labelsSnapshot()[modeCode]
	if len(got) != 1 || got[0].Dimension != dimAIUse || got[0].Level != levelCompetent {
		t.Fatalf("not using AI must be capped at competent on ai_use and stand over labellers, got %+v", got)
	}
	if _, ok := sess.unlabelledAnswers(modeCode)[idx]; ok {
		t.Fatal("a no-AI answer is already labelled and must not be backfilled")
	}
}

func TestCurlyQuotesAreNormalized(t *testing.T) {
	if !d5SaysNoAI("I didn’t use any AI tools for this—the logic is simple.") {
		t.Fatal("curly apostrophe no-AI answer not recognised")
	}
	if !d5IsVagueAnswer("I don’t know") {
		t.Fatal("curly apostrophe vague answer not recognised")
	}
}

func TestBugMarkerPattern(t *testing.T) {
	cases := map[string]bool{
		"```python\navg = total + count / 2  # Bug: divides only count\n```":              true,
		"```python\nwhile choice != 'q':  # bug: choice never re-read\n```":               true,
		"```python\navg = total + count / 2\n```":                                         false,
		"```python\nwhile c != 'q':\n    print(c)\n    # Bug: c is never read again\n```": true,
	}
	for reply, want := range cases {
		if got := bugMarkerPattern.MatchString(reply); got != want {
			t.Errorf("bugMarkerPattern(%q) = %v, want %v", reply, got, want)
		}
	}
}

func TestD5MarkedBugLine(t *testing.T) {
	content := "A tool.\n```python\nx = 1\n    total = total + i  # Bug: adds index\n```\nHow?"
	if got := d5MarkedBugLine(content); got != "total = total + i  # Bug: adds index" {
		t.Fatalf("d5MarkedBugLine = %q", got)
	}
	if got := d5MarkedBugLine("```python\nx = 1\n```"); got != "" {
		t.Fatalf("unmarked snippet gave %q", got)
	}
}
