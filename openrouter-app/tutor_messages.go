package main

import (
	"fmt"
	"sort"
	"strings"
)

// Messages that need no model call are written by the server, so they are instant and
// always accurate about the student's progress.

const welcomeMessage = "# Welcome to iPyTutor 👋\n\n" +
	"I'm your Python study partner. Each week I'll teach you the key concept, give you practice problems like the ones in your **iPyIntervu** interview, and keep track of your progress so you know when you're ready.\n\n" +
	"**New here?** Tell me your major (for example *Biology* or *Business*) and I'll set up your study profile.\n\n" +
	"**Studied with me before?** Type your study code. It looks like `maple-river-4821`."

const askMajorAgainMessage = "To get started, tell me your major, in a word or two (for example *Nursing* or *Economics*). " +
	"If you've studied with me before, type your study code instead."

const serverRestartedMessage = "Sorry, the tutor restarted and lost track of what we were doing just now. " +
	"Your progress is saved, though. Pick what to do next with the buttons below."

func studyCodeIssuedMessage(p studentProfile) string {
	return fmt.Sprintf("Great, your study profile is set up for **%s**.\n\n"+
		"Your study code is **`%s`**. Write it down! This browser will remember it, but you'll need it to pick up where you left off on another computer.\n\n"+
		"Which week are you studying?\n\n%s\n\nType the week number (for example `5`).", p.Major, p.StudyCode, weekListMarkdown(p))
}

func unknownStudyCodeMessage(code string) string {
	return fmt.Sprintf("I couldn't find the study code `%s`. Check it for typos and try again, or tell me your major to start a new profile.", code)
}

const tooManyCodeTriesMessage = "That's too many study codes that didn't match. Reload the page to try again, or tell me your major to start a new profile."

// weekListMarkdown lists the syllabus weeks, marking those the student has studied.
func weekListMarkdown(p studentProfile) string {
	lines := make([]string, 0, len(weeklyKeyConceptSelections))
	for _, sel := range weeklyKeyConceptSelections {
		line := "- " + sel.SelectedKeyConcept
		if wp := p.Weeks[sel.CurrentWeekNumber]; wp != nil {
			r := weekReadiness(weekInfo(sel.CurrentWeekNumber), wp)
			line += " · *" + r.Label + "*"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func chooseWeekMessage(p studentProfile) string {
	return "Which week would you like to study?\n\n" + weekListMarkdown(p) + "\n\nType the week number (for example `5`)."
}

func invalidWeekMessage(p studentProfile) string {
	return "That isn't one of the weeks.\n\n" + weekListMarkdown(p) + "\n\nType the week number (for example `5`)."
}

func welcomeBackMessage(p studentProfile) string {
	var b strings.Builder
	b.WriteString("# Welcome back 👋\n\n")
	w := weekInfo(p.CurrentWeek)
	if w == nil {
		fmt.Fprintf(&b, "Your study code is `%s`.\n\n%s", p.StudyCode, chooseWeekMessage(p))
		return b.String()
	}
	wp := p.Weeks[w.Number]
	fmt.Fprintf(&b, "You're studying **%s**.\n\n", w.Title)
	b.WriteString(weekStatusLines(w, wp))
	fmt.Fprintf(&b, "\n**Next:** %s\n\nTo study a different week, pick it from the menu or type `week` and its number.", nextStep(w, wp))
	return b.String()
}

// weekStatusLines summarizes lessons and practice for a week as a short list.
func weekStatusLines(w *weekContent, wp *weekProgress) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- 📖 Lessons: %d of %d done\n", lessonsDone(w, wp), len(w.Lessons))
	r := weekReadiness(w, wp)
	for _, p := range r.Parts {
		fmt.Fprintf(&b, "- %s %s: %s\n", practiceEmoji(p.Kind), practiceLabel(w.Number, p.Kind), partSummary(p))
	}
	fmt.Fprintf(&b, "- 🎯 Interview readiness: **%s**\n", r.Label)
	return b.String()
}

func partSummary(p partStatus) string {
	if p.Attempts == 0 {
		return "not tried yet"
	}
	tries := "1 try"
	if p.Attempts != 1 {
		tries = fmt.Sprintf("%d tries", p.Attempts)
	}
	if p.Level == "" {
		return tries
	}
	return fmt.Sprintf("latest **%s** (%s)", levelLabel(p.Level), tries)
}

func weekPlanMessage(w *weekContent, wp *weekProgress) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", w.Title)
	if len(w.Topics) > 0 {
		b.WriteString("**This week you'll learn:** " + strings.Join(w.Topics, "; ") + ".\n\n")
	}
	b.WriteString("**Your plan for the week**\n")
	fmt.Fprintf(&b, "1. 📖 **Teach me**: work through the %d short lessons (%d done so far).\n", len(w.Lessons), lessonsDone(w, wp))
	if w.Number == 1 {
		b.WriteString("2. 💬 **Decomposition problem**: break real tasks from your field into input, process and output, like the interview asks.\n")
		b.WriteString("3. 🎯 **Interview guide**: see exactly what earns Competent and Exceptional.\n\n")
	} else {
		b.WriteString("2. 💬 **Concept question**: explain how this week's Python works in a real situation.\n")
		b.WriteString("3. 🧩 **Code problem**: break a problem down, then write the code and explain it.\n")
		b.WriteString("4. 🐞 **Bug hunt**: track down the bug in a short program and explain how you found it.\n")
		b.WriteString("5. 🎯 **Interview guide**: see exactly what earns Competent and Exceptional.\n\n")
	}
	b.WriteString("Every practice answer gets feedback and an **Interview check**: the rating it would likely get in the interview. ")
	b.WriteString("Ask for a **hint** when you're stuck, or the **solution** when you want to see one worked through.\n\n")
	b.WriteString(weekStatusLines(w, wp))
	fmt.Fprintf(&b, "\n**Next:** %s\n\nYou can also just ask me anything about %s.", nextStep(w, wp), w.Topic)
	return b.String()
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

func progressMessage(p studentProfile, current int) string {
	var b strings.Builder
	b.WriteString("# Your progress\n\n")
	fmt.Fprintf(&b, "**Study code:** `%s` · **Major:** %s\n\n", p.StudyCode, p.Major)
	var weeks []int
	for n := range p.Weeks {
		if weekInfo(n) != nil {
			weeks = append(weeks, n)
		}
	}
	if current > 0 && p.Weeks[current] == nil {
		weeks = append(weeks, current)
	}
	sort.Ints(weeks)
	if len(weeks) == 0 {
		b.WriteString("You haven't started a week yet. Pick one to begin.")
		return b.String()
	}
	b.WriteString("| Week | Lessons | Concepts | Code | Bug hunt | Readiness |\n|---|---|---|---|---|---|\n")
	for _, n := range weeks {
		w := weekInfo(n)
		wp := p.Weeks[n]
		r := weekReadiness(w, wp)
		cells := map[string]string{kindConcept: "–", kindCode: "–", kindBug: "–"}
		if n == 1 {
			cells[kindCode], cells[kindBug] = "n/a", "n/a"
		}
		for _, part := range r.Parts {
			switch {
			case part.Level != "":
				cells[part.Kind] = levelLabel(part.Level)
			case part.Attempts > 0:
				cells[part.Kind] = "tried"
			}
		}
		name := w.Title
		if n == current {
			name = "**" + name + "**"
		}
		fmt.Fprintf(&b, "| %s | %d/%d | %s | %s | %s | %s |\n", name, lessonsDone(w, wp), len(w.Lessons), cells[kindConcept], cells[kindCode], cells[kindBug], r.Label)
	}
	b.WriteString("\nEach part shows the rating of your latest rated practice. Readiness uses the interview's rule: Not Ready Yet in any part means not ready yet overall.\n")
	if w := weekInfo(current); w != nil {
		fmt.Fprintf(&b, "\n**Next for %s:** %s", strings.SplitN(w.Title, " - ", 2)[0], nextStep(w, p.Weeks[current]))
	}
	return b.String()
}

func studyCodeMessage(p studentProfile) string {
	return fmt.Sprintf("Your study code is **`%s`**. Type it on any device to pick up where you left off.", p.StudyCode)
}

func helpMessage(w *weekContent) string {
	var b strings.Builder
	b.WriteString("Here's what I can do. Use the buttons below or type these:\n\n")
	b.WriteString("- **teach me** / **next lesson**: work through this week's lessons\n")
	if w != nil && w.Number == 1 {
		b.WriteString("- **decomposition problem**: practice breaking down a task\n")
	} else {
		b.WriteString("- **concept question**, **code problem**, **bug hunt**: practice each part of the interview\n")
	}
	b.WriteString("- **hint** or **solution**: when you're working on a problem\n")
	b.WriteString("- **another one**, **a harder one**, **an easier one**: more practice of the same kind\n")
	b.WriteString("- **interview guide**: what the interview asks and how it's rated\n")
	b.WriteString("- **progress**: your progress across weeks\n")
	b.WriteString("- **week 5** (any number): study a different week\n")
	b.WriteString("- **plan**: this week's study plan\n\n")
	b.WriteString("Or just ask me anything about Python.")
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

func allLessonsDoneMessage(w *weekContent, wp *weekProgress) string {
	return fmt.Sprintf("🎉 You've worked through all %d lessons for %s. Ask me about any of them again, or keep practicing.\n\n**Next:** %s", len(w.Lessons), w.Title, nextStep(w, wp))
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
