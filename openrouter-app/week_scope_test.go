package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestForbiddenConceptsForWeek7(t *testing.T) {
	forbidden := forbiddenConceptsFromLaterWeeks(7)
	if len(forbidden) == 0 {
		t.Fatal("expected forbidden concepts for week 7")
	}
	joined := strings.ToLower(strings.Join(forbidden, " "))
	for _, term := range []string{"lists", "file i/o"} {
		if !strings.Contains(joined, term) {
			t.Fatalf("expected week 7 forbidden list to include %q", term)
		}
	}
}

func TestAllowedWeekNumbersForWeek7(t *testing.T) {
	allowed := allowedWeekNumbers(7)
	want := []int{1, 2, 3, 4, 5, 6, 7}
	if !slices.Equal(allowed, want) {
		t.Fatalf("allowedWeekNumbers(7) = %v, want %v", allowed, want)
	}
}

func TestAssessmentWeekScopeSnapshot(t *testing.T) {
	scope := assessmentWeekScopeSnapshot(7)
	if scope == nil {
		t.Fatal("expected non-nil scope")
	}
	if scope["primaryWeekNumber"] != 7 {
		t.Fatalf("primaryWeekNumber = %v", scope["primaryWeekNumber"])
	}
}

// Replays a reported Week 2 follow-up that required conditionals and loops.
func TestWeek2ReplyNeedingConditionalsAndLoopsIsOutOfScope(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:            phaseAssessmentInProgress,
		ActiveMode:                   modeConceptual,
		CurrentWeekNumber:            2,
		SelectedKeyConcept:           "Week 2 - Variables & Expressions",
		ConceptualAssessmentPhase:    assessmentPhaseInProgress,
		ModeOpeningServed:            true,
		ModeUserAnsweredSinceOpening: true,
		ModeInterviewStep:            interviewStepInterviewing,
		LastUserMessageRaw:           "The process would be to multiply price * quantity and add that to a running total variable.",
	}
	assistant := "That’s a clear separation of the problem into input, process, and output. Keeping a running total as a variable is a solid approach.\n\n" +
		"Now, suppose the order has multiple line items, and after the loop finishes, you also need the total to reflect only the items with a positive quantity. How would you guard against including a line where quantity is zero or negative—without using an if statement?\n\n" +
		"```_ipyintervu\n{\"conceptualAssessmentPhase\": \"in_progress\"}\n```"

	v := detectAssessmentViolations(state, assistant)
	if !slices.Equal(v.OutOfScope, []string{"if statements and conditionals (week 5)", "loops (week 6)"}) {
		t.Fatalf("OutOfScope = %q", v.OutOfScope)
	}
	followUp := postProcessAssistantTurn(state, assistant, false, nil)
	if followUp.Kind != "corrective_retry" {
		t.Fatalf("expected corrective_retry, got %+v", followUp)
	}
	for _, want := range []string{"loops (week 6)", "week 2: variables", "not even to tell the student not to use it", "do NOT re-present the opening scenario"} {
		if !strings.Contains(followUp.Handoff, want) {
			t.Errorf("handoff missing %q: %q", want, followUp.Handoff)
		}
	}
	if strings.Contains(followUp.Handoff, "week 3:") {
		t.Errorf("handoff should list only weeks 1-2 as learned: %q", followUp.Handoff)
	}
}

func TestOutOfScopeConcepts(t *testing.T) {
	cases := []struct {
		week int
		text string
		want []string
	}{
		{2, "What would you identify as the input before any processing begins?", nil},
		{2, "What value does area hold after `area = length * width` runs?", nil},
		{2, "```python\n# compute area and perimeter\narea = length * width\nprint(\"area and perimeter\", area)\n```\nWhat does this print?", nil},
		{2, "Why would you convert the price to a float first?", []string{"input() and type casting (week 3)"}},
		{2, "```python\nif total > 100:\n    discount = 10\n```\nWhat is discount?", []string{"if statements and conditionals (week 5)"}},
		{5, "```python\nif total > 100 and member:\n    discount = 10\n```\nWhat is discount?", nil},
		{5, "How would you repeat this for every item using a loop?", []string{"loops (week 6)"}},
		{6, "```python\nfor item in range(3):\n    total = total + item\n```\nWhat is total?", nil},
		{7, "```python\nprices = [3, 4]\n```\nWhat is prices?", []string{"lists (week 8)"}},
		{8, "How would you save the list to a file?", []string{"files (week 9)"}},
		{9, "Would a dictionary work better here?", []string{"dictionaries (never in scope)"}},
	}
	for _, c := range cases {
		if got := outOfScopeConcepts(c.week, c.text); !slices.Equal(got, c.want) {
			t.Errorf("week %d %q: got %q, want %q", c.week, c.text, got, c.want)
		}
	}
}

func TestSystemPromptStatesWeekScopeEachTurn(t *testing.T) {
	state := &AgentSessionState{
		ConversationPhase:  phaseAssessmentInProgress,
		ActiveMode:         modeConceptual,
		CurrentWeekNumber:  2,
		SelectedKeyConcept: "Week 2 - Variables & Expressions",
	}
	prompt, _, _, err := buildSystemPrompt(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"WEEK SCOPE: the student chose Week 2 - Variables & Expressions", "week 2: variables", "if statements and conditionals, loops", "This scope is internal: you are job interviewers at the company, not instructors"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	if strings.Contains(prompt, "week 3: input()") {
		t.Error("week 2 prompt should not list week 3 as learned")
	}

	state.CurrentWeekNumber, state.SelectedKeyConcept = 9, "Week 9 - Lists and Files"
	prompt, _, _, _ = buildSystemPrompt(state)
	if !strings.Contains(prompt, "Do not use or mention dictionaries") {
		t.Error("week 9 should still forbid dictionaries")
	}
}

func TestUserEntryWordingIsWeek3(t *testing.T) {
	task := "We’re writing a small program for our warehouse team: it calculates the total cost of an order. The user will enter a quantity and a price per item, and the program should compute and print the total."
	if got := outOfScopeConcepts(2, task); !slices.Equal(got, []string{"input() and type casting (week 3)"}) {
		t.Fatalf("week 2 task needing user entry: got %q", got)
	}
	if got := outOfScopeConcepts(3, task); got != nil {
		t.Fatalf("user entry is in scope for week 3, got %q", got)
	}
	for _, ok := range []string{
		"What would you identify as the input (the quantity and price) for this task?",
		"Input (weight, height), Process (BMI formula), Output (BMI).",
	} {
		if got := outOfScopeConcepts(2, ok); got != nil {
			t.Errorf("decomposition wording %q flagged as %q", ok, got)
		}
	}
}

// The model grades against the rubric and the week's support files, so one that expects a
// later-week concept makes it design out-of-scope tasks and reject in-scope answers (week 2
// expected input(); weeks 5 and 6 graded loops and while menus).
func TestWeekFilesExpectOnlyWeeksSoFar(t *testing.T) {
	for week := 1; week <= lastSyllabusWeek; week++ {
		for _, path := range []string{
			fmt.Sprintf("env/rubrics/week%d_rubric.md", week),
			fmt.Sprintf("env/IPYIntervu_support_files/week%d_key_concepts.md", week),
			fmt.Sprintf("env/IPYIntervu_support_files/week%d_competency_guide.md", week),
		} {
			data, err := instructionFS.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			inForbiddenSection := false
			for i, line := range strings.Split(string(data), "\n") {
				lower := strings.ToLower(line)
				if strings.HasPrefix(line, "#") {
					inForbiddenSection = strings.Contains(lower, "forbidden")
				}
				if inForbiddenSection || strings.Contains(lower, "forbidden") || strings.Contains(lower, "not expected") || strings.Contains(lower, "do not") || strings.Contains(lower, "must not") || strings.Contains(lower, "not assessed") {
					continue // lines stating what is out of scope
				}
				if found := outOfScopeConcepts(week, line); found != nil {
					t.Errorf("%s:%d expects %q: %s", path, i+1, found, strings.TrimSpace(line))
				}
			}
		}
	}
}

// Strings and booleans are valid Week 2 variable types; only later-week operations on them
// (string methods, comparisons, and/or/not) are out of scope.
func TestWeek2StringAndBoolVariablesInScope(t *testing.T) {
	for _, text := range []string{
		"```python\nname = \"Ava\"\nis_member = True\ndiscount_rate = 0.1\ncount = 3\n```\nWhat type does each variable hold?",
		"Which variable in your script holds a string, and which holds a boolean?",
		"```python\ngreeting = \"Hello, \" + name\nprint(f\"{greeting} member: {is_member}\")\n```\nWhat does this print?",
	} {
		if got := outOfScopeConcepts(2, text); got != nil {
			t.Errorf("week 2 string/bool use flagged as %q: %q", got, text)
		}
	}
	if got := outOfScopeConcepts(2, "```python\nname = \"ava\".upper()\n```\nWhat is name?"); !slices.Equal(got, []string{"string methods (week 4)"}) {
		t.Errorf("string method in week 2 should be flagged, got %q", got)
	}
}
