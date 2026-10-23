// fake-openrouter stands in for OpenRouter so iPyTutor can be run and clicked through
// without an API key or token costs. It answers each kind of tutor call (lesson, practice
// problem, feedback, hint, solution, chat) with a fixed reply in the format the tutor
// expects, streamed like OpenRouter. Replies are canned, not real tutoring.
//
//	go run tools/fake-openrouter/main.go [-addr 127.0.0.1:8099]
//
// Then start the tutor with OPENROUTER_URL=http://127.0.0.1:8099 (tools/run-local.sh does
// this when FAKE_LLM=1).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

var studyingPattern = regexp.MustCompile(`The student is studying (Week \d+ - [^\n.]+)`)

func main() {
	addr := flag.String("addr", "127.0.0.1:8099", "listen address")
	delay := flag.Duration("delay", 600*time.Millisecond, "pause before each reply, to feel like a model")
	flag.Parse()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) == 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		system := req.Messages[0].Content
		week := "this week"
		if m := studyingPattern.FindStringSubmatch(system); m != nil {
			week = m[1]
		}
		kind, content := reply(system, week, req.Messages)
		log.Printf("%s (%s)", kind, week)
		time.Sleep(*delay)

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, ": OPENROUTER PROCESSING\n\n")
		// Stream in a few chunks, like a real model.
		for _, part := range chunks(content, 120) {
			data, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"delta": map[string]string{"content": part}}}})
			fmt.Fprintf(w, "data: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	log.Printf("fake OpenRouter listening on http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func chunks(s string, n int) []string {
	var out []string
	for len(s) > n {
		out = append(out, s[:n])
		s = s[n:]
	}
	return append(out, s)
}

func reply(system, week string, msgs []message) (string, string) {
	last := msgs[len(msgs)-1].Content
	switch {
	case strings.Contains(system, "Write one new practice problem"):
		switch {
		case strings.Contains(system, "like the code part"):
			return "problem/code", codeProblem
		case strings.Contains(system, "like the bug hunt part"):
			return "problem/bug", bugProblem
		case strings.Contains(system, "like the decomposition part"):
			return "problem/decomposition", decompositionProblem
		}
		return "problem/concept", conceptProblem
	case strings.Contains(system, "Teach lesson"):
		title := "This lesson"
		if m := regexp.MustCompile(`Teach lesson \d+ of \d+ for this week: "([^"]+)"`).FindStringSubmatch(system); m != nil {
			title = m[1]
		}
		// Like the real model with reasoning off, it plans before the tagged reply.
		return "lesson", "Do NOT say \"Lesson 1 of 5\" or mention the lesson structure. Write only the lesson itself.\n<reply>\n" + fmt.Sprintf(lessonTemplate, title, week) + "\n</reply>"
	case strings.Contains(system, "asked for a hint"):
		return "hint", "Look closely at the order of the checks. Python runs only the **first** branch whose condition is True. Which condition does a score of 95 meet first? Try tracing it, then tell me what you find."
	case strings.Contains(system, "asked to see the solution"):
		return "solution", solution
	case strings.Contains(system, "How to reply:"):
		if strings.HasSuffix(strings.TrimSpace(last), "?") {
			return "feedback/question", "Good question! `elif` means \"else if\": Python only checks it when every condition above it was False. Have a go at the problem with that in mind, and I'll give you feedback.\nLEVEL: none"
		}
		return "feedback", "Plan: praise the input, then the missing reason.\n<reply>\n" + feedback
	case strings.Contains(system, "quick check question"):
		return "lesson_reply", "Yes, that's right! 🎉 `==` compares two values and gives `True` or `False`, while a single `=` stores a value in a variable. Mixing them up is one of the most common mistakes, so it's great that you've got it.\n\n**Interview prep:** if an interviewer shows you a condition, say out loud whether it compares or assigns, and what it evaluates to for a sample value.\n\nAsk me anything about this lesson, choose **Next lesson**, or try a practice problem."
	}
	return "chat", "Great question. Here's the short version:\n\n- A **condition** is an expression that's either `True` or `False`.\n- `if` runs its block only when the condition is `True`.\n\n```python\ntemperature = 39.2\nif temperature >= 38.0:\n    print(\"Fever: notify the nurse\")\n```\n\nThis prints the message because 39.2 is at least 38.0. Want to try a **Concept question** on this?"
}

const lessonTemplate = `## %[1]s

*(This is a canned lesson from the fake OpenRouter server, for %[2]s.)*

A decision in a program works like a decision in real life: you check something, and what you do next depends on the answer. In Python, the check is a **condition**, an expression that is either ` + "`True` or `False`" + `.

### Example

` + "```python" + `
heart_rate = 112
if heart_rate > 100:
    print("Tachycardia: flag for review")
else:
    print("Normal range")
` + "```" + `

This prints *Tachycardia: flag for review*, because 112 is greater than 100. Only one of the two branches ever runs.

### Common mistake

Writing ` + "`if heart_rate = 100:`" + ` uses ` + "`=`" + ` (store a value) instead of ` + "`==`" + ` (compare). Python reports a syntax error.

### In the interview

Expect to explain *why* a branch runs for a given value, and to mention a boundary case such as exactly 100.

### Quick check

What does ` + "`==`" + ` do, and how is it different from ` + "`=`" + `?`

const conceptProblem = `TITLE: Triage priority labels
PROBLEM:
A hospital's intake app labels each patient's priority from their pain score (0 to 10). A developer wrote this:

` + "```python" + `
if pain >= 4:
    label = "medium"
elif pain >= 8:
    label = "high"
else:
    label = "low"
` + "```" + `

A patient reports a pain score of 9. What label do they get, and why?
REFERENCE:
They get "medium": Python checks the branches top to bottom and runs only the first one whose condition is True. 9 >= 4 is True, so the elif is never checked. Fix: check >= 8 first. Exceptional answers trace the order, name the fix, and test boundaries such as 4, 8 and 9.`

const codeProblem = `TITLE: Clinic wait-time alert
PROBLEM:
A walk-in clinic wants its front desk to know when a patient has waited too long. Staff type in how many minutes a patient has been waiting, and the program shows a message.

**Data available:** the number of minutes a patient has waited
**What's wanted:** "On time" for under 30 minutes, "Check in with patient" for 30 to 59, and "Escalate to manager" for 60 or more

First, how would you break this problem down: what's the input, what processing steps are needed, and what's the output? (After that, you'll write the code.)
REFERENCE:
Input: minutes (int). Process: compare with thresholds, highest first. Output: one message.
` + "```python" + `
minutes = int(input("Minutes waited: "))
if minutes >= 60:
    print("Escalate to manager")
elif minutes >= 30:
    print("Check in with patient")
else:
    print("On time")
` + "```" + `
Edge cases: exactly 30 and 60; negative input.`

const bugProblem = `TITLE: Lab grade boundary checker
PROBLEM:
This program is supposed to give a lab report its grade from a score out of 100.

` + "```python" + `
score = int(input("Score: "))
if score >= 60:
    grade = "Pass"
elif score >= 90:
    grade = "Distinction"
else:
    grade = "Fail"
print(grade)
` + "```" + `

When a student scores 95, it prints "Pass" instead of "Distinction". How would you track down this bug? Walk me through your debugging steps, and tell me what you think is causing it.
REFERENCE:
Line 2: the >= 60 check comes before >= 90, so 95 matches it first and the elif never runs. Fix: check >= 90 first.`

const decompositionProblem = `TITLE: Planning a blood drive
PROBLEM:
Your student nursing association is running a one-day blood drive in the campus gym. You need to recruit donors, book the donation team, set up the room, and report how many units were collected to the regional blood bank.

How would you break this task down? What's the input, what are the ordered steps of the process, and what's the output?
REFERENCE:
Input: date, venue, volunteer list, donor sign-ups, blood bank requirements. Process: confirm date and venue; book the donation team; recruit and screen donors; set up stations; run the drive; record units. Output: report of units collected. Exceptional: edge cases such as no-shows or ineligible donors, and why booking comes before recruiting.`

const feedback = `Nice work: you named the **input** clearly and your steps are in a sensible order. 👍

One thing to tighten: say *how* the processing decides between outcomes. Which value is checked first, and why does that order matter? Naming the boundary values (exactly 30 and exactly 60) would show you've thought about edge cases.

**Interview check:** this would likely be rated **Competent**: to reach Exceptional, explain why you check the highest threshold first and mention what happens at the boundaries.
**Interview prep:** practice saying your breakdown out loud in one breath: input, then each decision in order, then the output, naming exactly 30 and 60 as the boundaries you'd test.

Next, write the Python for this and paste it here. You can use AI to help, but be ready to explain every line.
</reply>
LEVEL: competent`

const solution = `Here's a worked solution.

**1. Reproduce it.** Run it with 95: it prints "Pass". Try 59, 60, 89 and 90 too, to see which boundaries misbehave.

**2. Find which branch runs.** Add a print in each branch, or trace by hand: 95 >= 60 is True, so the first branch runs and Python never looks at the elif.

**3. The cause.** Branch order: only the first True branch runs, so the >= 90 check must come first.

` + "```python" + `
if score >= 90:
    grade = "Distinction"
elif score >= 60:
    grade = "Pass"
else:
    grade = "Fail"
` + "```" + `

**Interview prep:** in the bug part, say your plan before your guess: reproduce with 95, test the boundaries 59, 60, 89 and 90, then explain why only the first true branch runs.

Try **Another one** without the solution to lock it in!`
