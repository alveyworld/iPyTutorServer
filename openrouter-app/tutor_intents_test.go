package main

import "testing"

func TestDetectIntent(t *testing.T) {
	cases := []struct {
		msg        string
		practicing bool
		want       intent
	}{
		{"hint", true, intent{Kind: intentHint}},
		{"Can I get a hint please?", true, intent{Kind: intentHint}},
		{"I'm stuck", true, intent{Kind: intentHint}},
		{"help", true, intent{Kind: intentHint}},
		{"help", false, intent{Kind: intentHelp}},
		{"show me the solution", true, intent{Kind: intentSolution}},
		{"I give up", true, intent{Kind: intentSolution}},
		{"teach me", false, intent{Kind: intentLesson}},
		{"Can you teach me?", false, intent{Kind: intentLesson}},
		{"next lesson", false, intent{Kind: intentNextLesson}},
		{"next", false, intent{Kind: intentNextLesson, MoveOn: true}},
		{"lesson 3", false, intent{Kind: intentLesson, Lesson: 3}},
		{"teach me lesson 2", false, intent{Kind: intentLesson, Lesson: 2}},
		{"give me a code problem", false, intent{Kind: intentPractice, Practice: kindCode}},
		{"bug hunt", false, intent{Kind: intentPractice, Practice: kindBug}},
		{"a harder bug hunt", false, intent{Kind: intentPractice, Practice: kindBug, Difficulty: difficultyHarder}},
		{"concept question please", false, intent{Kind: intentPractice, Practice: kindConcept}},
		{"decomposition problem", false, intent{Kind: intentPractice, Practice: kindConcept}},
		{"practice problem", false, intent{Kind: intentPractice}},
		{"quiz me", false, intent{Kind: intentPractice}},
		{"another one", false, intent{Kind: intentAnother}},
		{"one more", false, intent{Kind: intentAnother}},
		{"an easier one", false, intent{Kind: intentAnother, Difficulty: difficultyEasier}},
		{"another problem", false, intent{Kind: intentAnother}},
		{"interview tips", false, intent{Kind: intentGuide}},
		{"How does the interview work?", false, intent{Kind: intentGuide}},
		{"week 7", false, intent{Kind: intentWeek, Week: 7}},
		{"switch to week 3", false, intent{Kind: intentWeek, Week: 3}},
		{"skip ahead to week 9", false, intent{Kind: intentWeek, Week: 9}},
		{"week 6 lesson 2", false, intent{Kind: intentWeek, Week: 6, Lesson: 2}},
		{"Week 6, lesson 2", false, intent{Kind: intentWeek, Week: 6, Lesson: 2}},
		{"lesson 4 of week 8", false, intent{Kind: intentWeek, Week: 8, Lesson: 4}},
		{"study lists", false, intent{Kind: intentWeek, Week: 8, Topic: true}},
		{"I want to learn string methods", false, intent{Kind: intentWeek, Week: 4, Topic: true}},
		{"teach me for loops", false, intent{Kind: intentWeek, Week: 6, Topic: true, Teach: true}},
		{"let's study while loops and menus", false, intent{Kind: intentWeek, Week: 7, Topic: true}},
		{"change week", false, intent{Kind: intentWeekMenu}},
		{"overview", false, intent{Kind: intentOverview}},
		{"plan", false, intent{Kind: intentOverview}},
		{"my major is Nursing", false, intent{Kind: intentSetMajor}},
		{"I'm a biology major", false, intent{Kind: intentSetMajor}},

		// Answers and questions are never commands.
		{"one", true, intent{}},
		{"elif", true, intent{}},
		{"The answer is 42 because of integer division", true, intent{}},
		{"I would use a hint variable to store the message for the user", true, intent{}},
		{"week 12", false, intent{}},
		{"What does elif mean?", false, intent{}},
		{"study for the interview", false, intent{}},
		{"my progress", false, intent{}},
		{"```python\nhint = 3\n```", true, intent{}},
		{"total = price * quantity\nprint(total)", true, intent{}},
	}
	for _, c := range cases {
		if got := detectIntent(c.msg, c.practicing); got != c.want {
			t.Errorf("detectIntent(%q, %v) = %+v, want %+v", c.msg, c.practicing, got, c.want)
		}
	}
}

func TestDetectWeekAndMajor(t *testing.T) {
	cases := []struct {
		msg  string
		bare bool
		want intent
	}{
		{"week 3, my major is Biology", false, intent{Kind: intentWeek, Week: 3, Major: "Biology"}},
		{"My major is Biology, week 3", false, intent{Kind: intentWeek, Week: 3, Major: "Biology"}},
		{"week 3 my major is biology", false, intent{Kind: intentWeek, Week: 3, Major: "biology"}},
		{"Hi! I'm a business major. Can we do week 4 lesson 2?", false, intent{Kind: intentWeek, Week: 4, Lesson: 2, Major: "business"}},
		{"I'm a nursing major and I want week 5", false, intent{Kind: intentWeek, Week: 5, Major: "nursing"}},
		{"I’m a Math and Physics major, week 6", false, intent{Kind: intentWeek, Week: 6, Major: "Math and Physics"}},
		{"week 6, lesson 2, my major is Economics", false, intent{Kind: intentWeek, Week: 6, Lesson: 2, Major: "Economics"}},
		{"lesson 4 of week 8 - I study Chemistry", true, intent{Kind: intentWeek, Week: 8, Lesson: 4, Major: "Chemistry"}},
		{"I'm majoring in music; week 2 please", false, intent{Kind: intentWeek, Week: 2, Major: "music"}},
		{"Biology, week 3", true, intent{Kind: intentWeek, Week: 3, Major: "Biology"}},
		{"week 7. Computer Science", true, intent{Kind: intentWeek, Week: 7, Major: "Computer Science"}},
		{"Biology for week 3", true, intent{Kind: intentWeek, Week: 3, Major: "Biology"}},
		{"I'm a biology major starting week 3", false, intent{Kind: intentWeek, Week: 3, Major: "biology"}},
	}
	for _, c := range cases {
		if got, ok := detectWeekAndMajor(c.msg, c.bare); !ok || got != c.want {
			t.Errorf("detectWeekAndMajor(%q, %v) = %+v, %v; want %+v", c.msg, c.bare, got, ok, c.want)
		}
	}

	rejected := []struct {
		msg  string
		bare bool
	}{
		{"week 3", true},
		{"my major is Biology", true},
		{"Biology, week 3", false}, // a bare word mid-study is more likely an answer
		{"lists, week 3", true},    // a week's topic, not a major
		{"week 3, week 5, my major is Biology", true},
		{"week 3, my major is Biology, I like loops", true},
		{"In week 3 I learned that input() returns a string, so my major is irrelevant", true},
		{"week 3, my major is Biology\nweek 4", true},
		{"week 12, my major is Biology", true},
	}
	for _, c := range rejected {
		if got, ok := detectWeekAndMajor(c.msg, c.bare); ok {
			t.Errorf("detectWeekAndMajor(%q, %v) = %+v, want not ok", c.msg, c.bare, got)
		}
	}
}

func TestParseAction(t *testing.T) {
	cases := map[string]intent{
		"lesson":         {Kind: intentLesson},
		"lesson:3":       {Kind: intentLesson, Lesson: 3},
		"next_lesson":    {Kind: intentNextLesson},
		"overview":       {Kind: intentOverview},
		"practice:code":  {Kind: intentPractice, Practice: kindCode},
		"practice":       {Kind: intentPractice},
		"another:harder": {Kind: intentAnother, Difficulty: difficultyHarder},
		"another":        {Kind: intentAnother},
		"week:9":         {Kind: intentWeek, Week: 9},
		"hint":           {Kind: intentHint},
		"guide":          {Kind: intentGuide},
	}
	for action, want := range cases {
		got, ok := parseAction(action)
		if !ok || got != want {
			t.Errorf("parseAction(%q) = %+v, %v; want %+v", action, got, ok, want)
		}
	}
	for _, action := range []string{"", "week:0", "week:10", "lesson:0", "practice:essay", "progress", "delete"} {
		if got, ok := parseAction(action); ok {
			t.Errorf("parseAction(%q) = %+v, want not ok", action, got)
		}
	}
}

func TestCleanMajor(t *testing.T) {
	cases := map[string]string{
		"Biology":                   "Biology",
		"I'm a biology major":       "biology",
		"my major is Business.":     "Business",
		"I study Computer Science":  "Computer Science",
		"Hi! I'm majoring in Music": "Music",
	}
	for in, want := range cases {
		if got, ok := cleanMajor(in); !ok || got != want {
			t.Errorf("cleanMajor(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "hi", "hello!", "I want to learn how loops work in Python today please", "```code```", "week 3, my major is Biology", "Biology lesson 2"} {
		if got, ok := cleanMajor(in); ok {
			t.Errorf("cleanMajor(%q) = %q, want rejected", in, got)
		}
	}
}
