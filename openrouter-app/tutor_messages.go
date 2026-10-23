package main

import (
	"fmt"
	"strings"
)

// Messages that need no model call are written by the server, so they are instant and
// always match the course content.

// welcomeMessage greets a new visit and lists the weeks.
func welcomeMessage() string {
	return "# Welcome to iPyTutor 👋\n\n" +
		"I'm your Python study partner for the **iPyIntervu** interviews. Pick **any week** and I'll teach its lessons, give you practice problems like the interview's, and tell you how each answer would likely be rated and how to prepare.\n\n" +
		"**Tip:** tell me your major first (for example *Biology* or *Business*) and I'll set examples and problems in your field, like the interview does.\n\n" +
		"Or jump straight in. Which week would you like to study?\n\n" + weekListMarkdown() +
		"\n\nType a week number (for example `5`), a topic (`lists`), or a lesson (`week 6 lesson 2`)."
}

const askMajorOrWeekMessage = "Tell me your major in a word or two (for example *Nursing* or *Economics*), or pick a week to study: type its number, such as `5`."

const serverRestartedMessage = "Sorry, the tutor restarted and lost track of our conversation. Pick a week to carry on, for example `5` or `week 6 lesson 2`.\n\n"

func majorSetMessage(major string, week int) string {
	if week > 0 {
		return fmt.Sprintf("Got it: from now on I'll set examples and practice problems in **%s**.", major)
	}
	return fmt.Sprintf("Great, I'll set examples and practice problems in **%s**.\n\nWhich week would you like to study?\n\n%s\n\nType a week number (for example `5`), a topic (`lists`), or a lesson (`week 6 lesson 2`).", major, weekListMarkdown())
}

// weekListMarkdown lists the syllabus weeks.
func weekListMarkdown() string {
	lines := make([]string, 0, len(weeklyKeyConceptSelections))
	for _, sel := range weeklyKeyConceptSelections {
		lines = append(lines, "- "+sel.SelectedKeyConcept)
	}
	return strings.Join(lines, "\n")
}

func chooseWeekMessage() string {
	return "Which week would you like to study?\n\n" + weekListMarkdown() + "\n\nType the week number (for example `5`), or a week and lesson (`week 6 lesson 2`)."
}

func invalidWeekMessage() string {
	return "That isn't one of the weeks.\n\n" + weekListMarkdown() + "\n\nType the week number (for example `5`)."
}

// pickWeekFirstMessage asks for a week before a command that needs one.
func pickWeekFirstMessage(lead string) string {
	if lead == "" {
		lead = "First, pick a week to study."
	}
	return lead + "\n\n" + weekListMarkdown()
}

// weekOverviewMessage introduces a week: its topics, lessons and practice.
func weekOverviewMessage(w *weekContent, major string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", w.Title)
	if len(w.Topics) > 0 {
		b.WriteString("**Topics:** " + strings.Join(w.Topics, "; ") + ".\n\n")
	}
	if w.Number > 1 {
		fmt.Fprintf(&b, "I'll assume you know everything from %s, and build on it.\n\n", earlierWeeks(w.Number))
	}
	b.WriteString("**Lessons.** Take them in order with **Teach me**, or jump to any one, such as `lesson 3`:\n")
	for i, l := range w.Lessons {
		fmt.Fprintf(&b, "%d. %s\n", i+1, l.Title)
	}
	b.WriteString("\n**Practice like the interview:** ")
	var kinds []string
	for _, k := range w.practiceKinds() {
		kinds = append(kinds, practiceEmoji(k)+" **"+practiceLabel(w.Number, k)+"**")
	}
	b.WriteString(strings.Join(kinds, " · "))
	b.WriteString(". Every answer gets feedback, an **Interview check** (the rating it would likely get), and advice on how to prepare. Ask for a **hint** or the **solution** any time.\n\n")
	b.WriteString("🎯 The **Interview guide** shows what this week's interview asks and how it's rated.")
	if major == "" {
		b.WriteString("\n\n*Tell me your major (`my major is Biology`) and I'll set examples in your field.*")
	}
	return b.String()
}

func earlierWeeks(week int) string {
	if week == 2 {
		return "Week 1"
	}
	return fmt.Sprintf("Weeks 1–%d", week-1)
}

// interviewGuideMessage explains the week's interview from its rubric.
func interviewGuideMessage(w *weekContent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# How the %s interview works\n\n", strings.SplitN(w.Title, " - ", 2)[0])
	levels := w.RubricLevels
	bullets := func(level, answer string, n int) string {
		items := levels[level][answer]
		if len(items) > n {
			items = items[:n]
		}
		return strings.Join(items, "; ")
	}
	if w.Number == 1 {
		b.WriteString("The Week 1 interview has **one part**. The interviewer describes a realistic task from your field and asks you to break it down: the **input**, the ordered **steps** of the process, and the **output**. Follow-up questions stay on the same task. There is no code.\n\n")
		fmt.Fprintf(&b, "- **Competent:** %s.\n", bullets(levelCompetent, "Conceptual", 4))
		fmt.Fprintf(&b, "- **Exceptional:** %s.\n", bullets(levelExceptional, "Conceptual", 4))
		fmt.Fprintf(&b, "- **Not Ready Yet:** %s.\n\n", bullets(levelNotReady, "Conceptual", 4))
	} else {
		b.WriteString("The interview has **three parts**, each rated Not Ready Yet, Competent or Exceptional.\n\n")
		b.WriteString("### 1. 💬 Concept questions\nThe interviewers describe a situation at a company in your field and ask how this week's Python works in it, with follow-ups.\n")
		fmt.Fprintf(&b, "- **Competent:** %s.\n", bullets(levelCompetent, "Conceptual", 5))
		fmt.Fprintf(&b, "- **Exceptional:** %s.\n\n", bullets(levelExceptional, "Conceptual", 5))
		b.WriteString("### 2. 🧩 Code problem\nYou get a short story problem. First you explain how you'd break it down (input, process, output). Then you write the Python (you may use AI to help), paste it, explain how it works, and describe how you used and checked AI.\n")
		fmt.Fprintf(&b, "- **Competent code:** %s.\n", bullets(levelCompetent, "Code", 5))
		fmt.Fprintf(&b, "- **Exceptional code:** %s.\n", bullets(levelExceptional, "Code", 5))
		fmt.Fprintf(&b, "- **AI use, Competent:** %s. **Exceptional:** %s.\n\n", bullets(levelCompetent, "AI Use", 3), bullets(levelExceptional, "AI Use", 3))
		b.WriteString("### 3. 🐞 Bug hunt\nYou're shown a short program with a bug and asked how you'd find it. Your **debugging process** is rated, not just the fix.\n")
		b.WriteString("- **Competent:** reproduce the problem with a concrete input, name a plausible cause, print or trace values at a sensible point, and have a next step.\n")
		b.WriteString("- **Exceptional:** a deliberate plan: predict the expected values, rank several possible causes, choose test values on purpose (including edge cases), and explain exactly how this week's concept produces the symptom.\n\n")
		b.WriteString("### Overall rating\nThe overall rating is **Not Ready Yet** if any part is Not Ready Yet, and **Exceptional** only if all three parts are Exceptional. So practice all three.\n\n")
	}
	b.WriteString("### Mistakes that lead to Not Ready Yet\n")
	for _, answer := range []string{"Conceptual", "Code"} {
		for i, item := range levels[levelNotReady][answer] {
			if i == 3 {
				break
			}
			b.WriteString("- " + item + "\n")
		}
	}
	b.WriteString("\n### Tips\n")
	b.WriteString("- Answer the question you're asked, then stop and let the interviewer follow up.\n")
	b.WriteString("- Think out loud: say *why*, not just *what*. Reasons and examples are what move an answer from Competent to Exceptional.\n")
	b.WriteString("- Mention an edge case: an unusual value, a boundary, or bad input.\n")
	if w.Number > 1 {
		b.WriteString("- Be ready to explain every line of code you paste, especially if AI helped you write it, and say how you checked it.\n")
		b.WriteString("- You may use Python from earlier weeks, and even later ones, as long as you can explain it.\n")
	}
	b.WriteString("\nWant to try it? Ask for a practice problem with the buttons below.")
	return b.String()
}

// helpMessage lists what the student can type. w is nil before a week is chosen.
func helpMessage(w *weekContent) string {
	var b strings.Builder
	b.WriteString("Here's what I can do. Use the buttons, or type these:\n\n")
	b.WriteString("- **week 5**, **lists**, **week 6 lesson 2**: study any week, topic or lesson, in any order\n")
	b.WriteString("- **teach me** / **next lesson** / **lesson 3**: lessons in the week you're studying\n")
	if w != nil && w.Number == 1 {
		b.WriteString("- **decomposition problem**: practice breaking down a task\n")
	} else {
		b.WriteString("- **concept question**, **code problem**, **bug hunt**: practice each part of the interview\n")
	}
	b.WriteString("- **hint** or **solution**: when you're working on a problem\n")
	b.WriteString("- **another one**, **a harder one**, **an easier one**: more practice of the same kind\n")
	b.WriteString("- **interview guide**: what the interview asks and how it's rated\n")
	b.WriteString("- **overview**: the week's topics and lessons\n")
	b.WriteString("- **my major is Biology**: set examples in your field\n\n")
	b.WriteString("Or just ask me anything about Python, or paste code and I'll explain it.")
	return b.String()
}

func noProblemOpenMessage(w *weekContent) string {
	kinds := make([]string, 0, 3)
	for _, k := range w.practiceKinds() {
		kinds = append(kinds, "**"+strings.ToLower(practiceLabel(w.Number, k))+"**")
	}
	return "There's no practice problem open right now. Want one? Ask for a " + joinOr(kinds) + "."
}

func noWeek1PracticeMessage(kind string) string {
	return fmt.Sprintf("Week 1's interview is decomposition only, so there's no %s yet; those start in Week 2. Want a **decomposition problem** instead?", strings.ToLower(practiceLabel(2, kind)))
}

func lastLessonMessage(w *weekContent) string {
	return fmt.Sprintf("That was the last of the %d lessons for %s. 🎉 Ask for any lesson again by number (`lesson 1` to `lesson %d`), practice for the interview, or move on to another week.", len(w.Lessons), w.Title, len(w.Lessons))
}

const tutorUnavailableMessage = "Sorry, I couldn't reach the tutor model just now. Please try again in a moment."

// lessonOutline is the lesson plan itself, shown when the model can't be reached.
func lessonOutline(w *weekContent, i int) string {
	l := w.Lessons[i]
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", l.Title)
	for _, p := range l.Points {
		b.WriteString("- " + p + "\n")
	}
	b.WriteString("\n*I couldn't reach the tutor model just now, so here is the lesson outline. Ask me about any point, or try again for the full lesson.*")
	return b.String()
}

func joinOr(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " or " + items[1]
	}
	return strings.Join(items[:len(items)-1], ", ") + " or " + items[len(items)-1]
}
