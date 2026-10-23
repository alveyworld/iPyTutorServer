package main

import "testing"

func TestStudentReply(t *testing.T) {
	cases := []struct {
		name, raw, want string
		tagged          bool
	}{
		{"notes before the tags",
			"Do NOT say \"Lesson 1 of 5\" or mention the lesson structure. Write only the lesson itself.\n\n<reply>\n## Strings and methods\n\nA string is text.\n</reply>",
			"## Strings and methods\n\nA string is text.", true},
		{"text after the closing tag is dropped",
			"<reply>\nNice work.\n</reply>\nLEVEL: competent", "Nice work.", true},
		{"cut off before the closing tag",
			"Plan: explain first.\n<reply>\nHere's how slicing works", "Here's how slicing works", true},
		{"notes that mention the tag",
			"I'll write the lesson inside <reply> tags.\n<reply>\nThe lesson.\n</reply>", "The lesson.", true},
		{"tags in other cases and spacing",
			"notes\n< Reply >\nHello!\n</REPLY>", "Hello!", true},
		{"no tags", "Just a plain answer.", "Just a plain answer.", false},
		{"empty tags fall back to the rest", "Some answer.\n<reply>\n</reply>", "Some answer.", false},
		{"stray closing tag", "An answer.\n</reply>", "An answer.", false},
	}
	for _, c := range cases {
		got, tagged := studentReply(c.raw)
		if got != c.want || tagged != c.tagged {
			t.Errorf("%s: studentReply = %q, %v; want %q, %v", c.name, got, tagged, c.want, c.tagged)
		}
	}
}

func TestUntaggedLessonStartsAtItsHeading(t *testing.T) {
	raw := "Do NOT ask if they're ready.\nAlways finish with ---.\n\n## Comparison operators and booleans\n\nComparisons give True or False."
	if got := cleanReply("s", callLesson, raw); got != "## Comparison operators and booleans\n\nComparisons give True or False." {
		t.Errorf("lesson = %q", got)
	}
	// Other replies have no heading to cut to, so they are left whole.
	if got := cleanReply("s", callChat, "Sure.\n## Heading later"); got != "Sure.\n## Heading later" {
		t.Errorf("chat = %q", got)
	}
}
