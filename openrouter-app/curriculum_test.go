package main

import (
	"strings"
	"testing"
)

func TestEveryWeekLoads(t *testing.T) {
	for _, sel := range weeklyKeyConceptSelections {
		n := sel.CurrentWeekNumber
		w := weekInfo(n)
		if w == nil {
			t.Fatalf("week %d missing", n)
		}
		if w.Topic == "" || strings.HasPrefix(w.Topic, "Week") {
			t.Errorf("week %d topic = %q", n, w.Topic)
		}
		if len(w.Topics) == 0 {
			t.Errorf("week %d has no topics", n)
		}
		if len(w.Lessons) < 4 {
			t.Errorf("week %d has %d lessons, want at least 4", n, len(w.Lessons))
		}
		for _, l := range w.Lessons {
			if len(l.Points) < 3 {
				t.Errorf("week %d lesson %q has %d points", n, l.Title, len(l.Points))
			}
		}
		if w.Examples == "" || strings.Contains(w.Examples, "Grading reference only") {
			t.Errorf("week %d teaching notes: %q", n, w.Examples)
		}
		answers := []string{"Conceptual"}
		if n > 1 {
			answers = append(answers, "Code", "AI Use")
		}
		for _, level := range []string{levelNotReady, levelCompetent, levelExceptional} {
			for _, answer := range answers {
				if len(w.RubricLevels[level][answer]) == 0 {
					t.Errorf("week %d rubric has no %s %s bullets", n, level, answer)
				}
			}
		}
		if n > 1 && (w.BugGuide == "" || !strings.HasPrefix(w.BugIdeas, "- ")) {
			t.Errorf("week %d bug ideas = %q", n, w.BugIdeas)
		}
	}
}

func TestWeek1HasOnlyDecompositionPractice(t *testing.T) {
	w := weekInfo(1)
	if got := w.practiceKinds(); len(got) != 1 || got[0] != kindConcept {
		t.Fatalf("week 1 practice kinds = %v", got)
	}
	if w.hasPractice(kindCode) || w.hasPractice(kindBug) {
		t.Fatal("week 1 offers code or bug practice")
	}
	if practiceLabel(1, kindConcept) != "Decomposition problem" {
		t.Fatalf("week 1 label = %q", practiceLabel(1, kindConcept))
	}
	if !weekInfo(5).hasPractice(kindBug) {
		t.Fatal("week 5 should offer bug hunts")
	}
}

func TestInterviewGuideUsesRubric(t *testing.T) {
	for _, sel := range weeklyKeyConceptSelections {
		w := weekInfo(sel.CurrentWeekNumber)
		guide := interviewGuideMessage(w)
		if strings.Contains(guide, ":** .") || strings.Contains(guide, ":** **") {
			t.Errorf("week %d guide has an empty rubric line:\n%s", w.Number, guide)
		}
		if w.Number == 1 {
			if strings.Contains(guide, "Bug hunt") || !strings.Contains(guide, "one part") {
				t.Errorf("week 1 guide should describe one decomposition part:\n%s", guide)
			}
			continue
		}
		for _, want := range []string{"three parts", "Concept questions", "Code problem", "Bug hunt", "Not Ready Yet"} {
			if !strings.Contains(guide, want) {
				t.Errorf("week %d guide missing %q", w.Number, want)
			}
		}
	}
	if !strings.Contains(interviewGuideMessage(weekInfo(5)), "every case in the prompt reaches exactly one branch") {
		t.Error("week 5 guide should quote the week 5 code rubric")
	}
}

func TestBasePromptStatesScope(t *testing.T) {
	week2 := tutorBasePrompt(weekInfo(2), "Biology")
	for _, want := range []string{"Biology", "Week 2 - Variables & Expressions", "week 2: variables", "Not yet learned: input() and type casting", "every value is set in the program", "<rubric>"} {
		if !strings.Contains(week2, want) {
			t.Errorf("week 2 prompt missing %q", want)
		}
	}
	if strings.Contains(week2, "week 3: input()") {
		t.Error("week 2 prompt lists week 3 as learned")
	}
	week1 := tutorBasePrompt(weekInfo(1), "Nursing")
	if !strings.Contains(week1, "use no code") || strings.Contains(week1, "Bug hunt") {
		t.Error("week 1 prompt should forbid code and describe the one-part interview")
	}
	week7 := tutorBasePrompt(weekInfo(7), "Art")
	if !strings.Contains(week7, "condition in the header") {
		t.Error("week 7 prompt should prefer condition-driven while loops")
	}
	if !strings.Contains(tutorBasePrompt(weekInfo(9), "Art"), "Never use dictionaries") {
		t.Error("week 9 prompt should still rule out dictionaries")
	}
}

func TestProblemPromptsMatchInterviewFormats(t *testing.T) {
	base := tutorBasePrompt(weekInfo(5), "Biology")
	code := problemMessages(base, weekInfo(5), kindCode, "Biology", difficultyStandard, []string{"Lab sample sorter"}, nil)[0].Content
	for _, want := range []string{"**Data available:**", "**What's wanted:**", "Lab sample sorter", "no code anywhere", "REFERENCE:"} {
		if !strings.Contains(code, want) {
			t.Errorf("code problem prompt missing %q", want)
		}
	}
	bug := problemMessages(base, weekInfo(5), kindBug, "Biology", difficultyHarder, nil, []string{"It had no program"})[0].Content
	for _, want := range []string{"elif` branches in the wrong order", "How would you track down this bug?", "a little harder", "It had no program"} {
		if !strings.Contains(bug, want) {
			t.Errorf("bug problem prompt missing %q", want)
		}
	}
	week2 := problemMessages(tutorBasePrompt(weekInfo(2), "Art"), weekInfo(2), kindCode, "Art", difficultyStandard, nil, nil)[0].Content
	if !strings.Contains(week2, "cannot ask the user for anything") {
		t.Error("week 2 code problems must not need input()")
	}
}
