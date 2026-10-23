package main

import (
	"embed"
	"fmt"
	"regexp"
	"strings"
)

// The week materials are the interview's own (key concepts, competency guides, rubrics), so
// the tutor teaches to what the interview grades. Copy updated ones over from
// iPyIntervuServer unchanged. env/tutor holds the tutor's own lesson plans and bug guide.
//
//go:embed env/IPYIntervu_support_files/*.md env/rubrics/*.md env/tutor/*.md
var contentFS embed.FS

const (
	kindLesson  = "lesson"
	kindConcept = "concept"
	kindCode    = "code"
	kindBug     = "bug"
)

const (
	levelNotReady    = "not_ready"
	levelCompetent   = "competent"
	levelExceptional = "exceptional"
)

// lesson is one entry of a week's lesson plan (env/tutor/weekN_lessons.md).
type lesson struct {
	Title  string
	Points []string
}

type weekContent struct {
	Number int
	// Title is the syllabus name, e.g. "Week 5 - Conditionals (if/elif/else)".
	Title string
	// Topic is the title without the week number, e.g. "Conditionals (if/elif/else)".
	Topic   string
	Topics  []string
	Lessons []lesson

	KeyConcepts string
	// Examples is the teaching part of the competency guide; its interviewer-only
	// sections are left out.
	Examples string
	Rubric   string
	// RubricLevels maps a level (levelNotReady, ...) and a rubric answer type
	// ("Conceptual", "Code", "AI Use") to that section's bullets.
	RubricLevels map[string]map[string][]string
	// BugGuide is the week's section of env/tutor/bug_hunting_guide.md, and BugIdeas its
	// list of suitable bugs; both empty for week 1.
	BugGuide string
	BugIdeas string
}

// practiceKinds returns the practice formats the week's interview has. Week 1 is
// decomposition only.
func (w *weekContent) practiceKinds() []string {
	if w.Number == 1 {
		return []string{kindConcept}
	}
	return []string{kindConcept, kindCode, kindBug}
}

func (w *weekContent) hasPractice(kind string) bool {
	for _, k := range w.practiceKinds() {
		if k == kind {
			return true
		}
	}
	return false
}

var (
	curriculum       = mustLoadCurriculum()
	bugGuideShared   = mustBugGuideSection("Shared descriptions")
	weekTitlePrefix  = regexp.MustCompile(`^Week \d+ - `)
	interviewerAside = regexp.MustCompile(`\*\*Grading reference only[^*]*\*\*\s*`)
)

func weekInfo(week int) *weekContent {
	return curriculum[week]
}

func mustLoadCurriculum() map[int]*weekContent {
	weeks := make(map[int]*weekContent, len(weeklyKeyConceptSelections))
	for _, sel := range weeklyKeyConceptSelections {
		w, err := loadWeek(sel)
		if err != nil {
			panic(err)
		}
		weeks[sel.CurrentWeekNumber] = w
	}
	return weeks
}

func loadWeek(sel weekSelection) (*weekContent, error) {
	n := sel.CurrentWeekNumber
	read := func(path string) (string, error) {
		data, err := contentFS.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("week %d: %w", n, err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	keyConcepts, err := read(fmt.Sprintf("env/IPYIntervu_support_files/week%d_key_concepts.md", n))
	if err != nil {
		return nil, err
	}
	guide, err := read(fmt.Sprintf("env/IPYIntervu_support_files/week%d_competency_guide.md", n))
	if err != nil {
		return nil, err
	}
	rubric, err := read(fmt.Sprintf("env/rubrics/week%d_rubric.md", n))
	if err != nil {
		return nil, err
	}
	lessonPlan, err := read(fmt.Sprintf("env/tutor/week%d_lessons.md", n))
	if err != nil {
		return nil, err
	}
	w := &weekContent{
		Number:       n,
		Title:        sel.SelectedKeyConcept,
		Topic:        weekTitlePrefix.ReplaceAllString(sel.SelectedKeyConcept, ""),
		Topics:       markdownBullets(markdownSection(keyConcepts, "## Topics Covered")),
		Lessons:      parseLessons(lessonPlan),
		KeyConcepts:  keyConcepts,
		Rubric:       rubric,
		RubricLevels: parseRubricLevels(rubric),
	}
	var examples []string
	for _, heading := range []string{"## Problem Decomposition Context", "## Key Concepts Overview", "## In scope", "## Simple Example Demonstration"} {
		if body := markdownSection(guide, heading); body != "" {
			examples = append(examples, heading+"\n\n"+body)
		}
	}
	// Interviewer-only asides, such as "never read aloud", do not apply to teaching.
	w.Examples = interviewerAside.ReplaceAllString(strings.Join(examples, "\n\n"), "")
	if n > 1 {
		w.BugGuide = mustBugGuideSection(fmt.Sprintf("Week %d ", n))
		w.BugIdeas = boldBlock(w.BugGuide, "**Suitable defects")
		if w.BugIdeas == "" {
			return nil, fmt.Errorf("week %d: bug guide has no suitable defects", n)
		}
	}
	if len(w.Lessons) == 0 {
		return nil, fmt.Errorf("week %d: lesson plan has no lessons", n)
	}
	return w, nil
}

// markdownSection returns the body under the first heading that starts with prefix, up to
// the next heading of the same or a higher level.
func markdownSection(md, prefix string) string {
	lines := strings.Split(md, "\n")
	level := strings.Count(strings.Fields(prefix)[0], "#")
	start := -1
	for i, line := range lines {
		if start < 0 {
			if strings.HasPrefix(line, prefix) {
				start = i + 1
			}
			continue
		}
		if l := headingLevel(line); l > 0 && l <= level {
			return strings.TrimSpace(strings.Join(lines[start:i], "\n"))
		}
	}
	if start < 0 {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[start:], "\n"))
}

// boldBlock returns the lines after the first line starting with label (a bold lead-in
// such as "**Suitable defects**"), up to the next bold lead-in or heading.
func boldBlock(md, label string) string {
	var out []string
	in := false
	for _, line := range strings.Split(md, "\n") {
		if !in {
			in = strings.HasPrefix(line, label)
			continue
		}
		if strings.HasPrefix(line, "**") || headingLevel(line) > 0 {
			break
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func headingLevel(line string) int {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n == 0 || n >= len(line) || line[n] != ' ' {
		return 0
	}
	return n
}

func markdownBullets(md string) []string {
	var out []string
	for _, line := range strings.Split(md, "\n") {
		if item, ok := strings.CutPrefix(strings.TrimSpace(line), "- "); ok {
			out = append(out, strings.TrimSpace(item))
		}
	}
	return out
}

// parseLessons reads a lesson plan: each "## " heading is a lesson and its bullets are the
// points to cover. Text before the first lesson is a note for editors.
func parseLessons(md string) []lesson {
	var lessons []lesson
	for _, line := range strings.Split(md, "\n") {
		if title, ok := strings.CutPrefix(line, "## "); ok {
			lessons = append(lessons, lesson{Title: strings.TrimSpace(title)})
			continue
		}
		if item, ok := strings.CutPrefix(strings.TrimSpace(line), "- "); ok && len(lessons) > 0 {
			last := &lessons[len(lessons)-1]
			last.Points = append(last.Points, strings.TrimSpace(item))
		}
	}
	return lessons
}

// parseRubricLevels collects the bullets of each level's answer sections. Rubrics spell the
// lowest level "Not Yet Ready" or "Not Ready Yet" and some answer headings carry a note in
// parentheses ("Conceptual Answer (problem decomposition)").
func parseRubricLevels(md string) map[string]map[string][]string {
	levels := map[string]map[string][]string{}
	level, answer := "", ""
	for _, line := range strings.Split(md, "\n") {
		switch headingLevel(line) {
		case 1:
			level, answer = "", ""
		case 2:
			level, answer = rubricLevelName(strings.TrimSpace(line[3:])), ""
		case 3:
			answer = ""
			if level != "" {
				name, ok := strings.CutSuffix(strings.SplitN(strings.TrimSpace(line[4:]), " (", 2)[0], " Answer")
				if ok {
					answer = name
				}
			}
		default:
			if level == "" || answer == "" {
				continue
			}
			if item, ok := strings.CutPrefix(strings.TrimSpace(line), "- "); ok {
				if levels[level] == nil {
					levels[level] = map[string][]string{}
				}
				levels[level][answer] = append(levels[level][answer], strings.TrimSpace(item))
			}
		}
	}
	return levels
}

func rubricLevelName(heading string) string {
	switch strings.ToLower(heading) {
	case "not yet ready", "not ready yet":
		return levelNotReady
	case "competent":
		return levelCompetent
	case "exceptional":
		return levelExceptional
	}
	return ""
}

func mustBugGuideSection(prefix string) string {
	data, err := contentFS.ReadFile("env/tutor/bug_hunting_guide.md")
	if err != nil {
		panic(err)
	}
	section := markdownSection(string(data), "## "+prefix)
	if section == "" {
		panic("bug_hunting_guide.md has no section " + prefix)
	}
	return section
}

// practiceLabel names a practice format for students.
func practiceLabel(week int, kind string) string {
	switch kind {
	case kindConcept:
		if week == 1 {
			return "Decomposition problem"
		}
		return "Concept question"
	case kindCode:
		return "Code problem"
	case kindBug:
		return "Bug hunt"
	}
	return "Practice"
}

func practiceEmoji(kind string) string {
	switch kind {
	case kindConcept:
		return "💬"
	case kindCode:
		return "🧩"
	case kindBug:
		return "🐞"
	}
	return "📝"
}

// interviewPartLabel names the interview part a practice format prepares for.
func interviewPartLabel(week int, kind string) string {
	switch kind {
	case kindConcept:
		if week == 1 {
			return "Decomposition"
		}
		return "Concepts"
	case kindCode:
		return "Code"
	case kindBug:
		return "Bug hunt"
	}
	return kind
}
