package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeOpenRouter answers each kind of tutor call with a fixed reply in the shape the tutor
// expects, streamed like OpenRouter, and records the calls it gets.
type fakeOpenRouter struct {
	mu    sync.Mutex
	calls map[string]int
	// prompts holds the latest system prompt for each kind of call.
	prompts map[string]string
	// feedbackLevel is the LEVEL the next feedback replies end with.
	feedbackLevel string
}

func (f *fakeOpenRouter) count(kind string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[kind]
}

func (f *fakeOpenRouter) prompt(kind string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.prompts[kind]
}

func (f *fakeOpenRouter) setFeedbackLevel(level string) {
	f.mu.Lock()
	f.feedbackLevel = level
	f.mu.Unlock()
}

func (f *fakeOpenRouter) reply(task string) (string, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case strings.Contains(task, "Write one new practice problem"):
		switch {
		case strings.Contains(task, "like the code part"):
			return "problem_code", "TITLE: Seed germination sorter\nPROBLEM:\nA greenhouse tracks seed trays.\n\n**Data available:** the number of sprouted seeds in a tray\n**What's wanted:** a label saying whether the tray is ready to move\n\nFirst, how would you break this problem down: what's the input, what processing steps are needed, and what's the output? (After that, you'll write the code.)\nREFERENCE:\nInput: sprouted count. Process: compare with 20. Output: label.\n```python\nsprouted = int(input())\nif sprouted >= 20:\n    print(\"ready\")\nelse:\n    print(\"wait\")\n```"
		case strings.Contains(task, "like the bug hunt part"):
			return "problem_bug", "TITLE: Grade boundary checker\nPROBLEM:\nThis program should label a lab score.\n```python\nscore = int(input())\nif score >= 60:\n    print(\"pass\")\nelif score >= 90:\n    print(\"distinction\")\n```\nA score of 95 prints pass. How would you track down this bug? Walk me through your debugging steps, and tell me what you think is causing it.\nREFERENCE:\nThe >= 60 branch is checked first."
		}
		return "problem_concept", "TITLE: Field sample labels\nPROBLEM:\nA field team labels samples. What does `x // 2` give when x is 7, and why?\nREFERENCE:\n3, because // drops the remainder."
	case strings.Contains(task, "Teach lesson"):
		return "lesson", "## A lesson\n\nSome teaching.\n\n### Quick check\nWhat does `==` do?"
	case strings.Contains(task, "asked for a hint"):
		return "hint", "Think about which branch Python checks first."
	case strings.Contains(task, "asked to see the solution"):
		return "solution", "Here is the worked solution.\n\n**Interview prep:** name the boundary values."
	case strings.Contains(task, "How to reply:"):
		level := f.feedbackLevel
		if level == "" {
			level = "competent"
		}
		return "feedback", "You named the input clearly.\n\n**Interview check:** this would likely be rated **Competent**.\n**Interview prep:** say why the threshold check comes first.\n\nNow paste your code.\nLEVEL: " + level
	case strings.Contains(task, "quick check question"):
		return "lesson_reply", "Yes, that's right.\n**Interview prep:** mention the boundary case."
	}
	return "chat", "Here's an answer to your question."
}

func (f *fakeOpenRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req llmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	kind, content := f.reply(req.Messages[0].Content)
	f.mu.Lock()
	f.calls[kind]++
	f.prompts[kind] = req.Messages[0].Content
	f.mu.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	chunk, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"delta": map[string]string{"content": content}}}})
	fmt.Fprintf(w, ": OPENROUTER PROCESSING\n\ndata: %s\n\ndata: [DONE]\n\n", chunk)
}

type tutorHarness struct {
	t        *testing.T
	tutor    *tutor
	turns    *turnStore
	session  string
	messages []chatMessage
	turnN    int
}

// newTutorHarness makes a tutor whose model calls go to upstream. Problems it prepares in
// the background may outlive the test; they then fail against the closed fake.
func newTutorHarness(t *testing.T, upstream string) *tutorHarness {
	t.Helper()
	llm := llmClient{apiKey: "test-key", url: upstream}
	return &tutorHarness{t: t, tutor: newTutor(llm), turns: newTurnStore(), session: "session-" + t.Name()}
}

func withFakeOpenRouter(t *testing.T) (*fakeOpenRouter, string) {
	t.Helper()
	fake := &fakeOpenRouter{calls: map[string]int{}, prompts: map[string]string{}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	return fake, server.URL
}

func (h *tutorHarness) request(path string, body any, turnID string) *httptest.ResponseRecorder {
	h.t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req = req.WithContext(context.WithValue(req.Context(), sessionIDContextKey, h.session))
	if turnID != "" {
		req.Header.Set(turnIDHeader, turnID)
	}
	rec := httptest.NewRecorder()
	switch path {
	case "/api/session/bootstrap":
		h.tutor.handleBootstrap(rec, req)
	default:
		h.tutor.handleChat(h.turns)(rec, req)
	}
	if rec.Code != http.StatusOK {
		h.t.Fatalf("POST %s: status %d: %s", path, rec.Code, rec.Body.String())
	}
	return rec
}

func (h *tutorHarness) bootstrap() (string, stateResponse) {
	h.t.Helper()
	var resp bootstrapResponse
	rec := h.request("/api/session/bootstrap", struct{}{}, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		h.t.Fatal(err)
	}
	h.messages = []chatMessage{{Role: "user", Content: "start"}, {Role: "assistant", Content: resp.Assistant}}
	return resp.Assistant, resp.State
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	State stateResponse `json:"state"`
}

// send posts one student turn; action is what a UI button would send.
func (h *tutorHarness) send(msg, action string) (string, stateResponse) {
	h.t.Helper()
	h.turnN++
	return h.sendTurn(msg, action, fmt.Sprintf("turn-%d", h.turnN))
}

func (h *tutorHarness) sendTurn(msg, action, turnID string) (string, stateResponse) {
	h.t.Helper()
	h.messages = append(h.messages, chatMessage{Role: "user", Content: msg})
	rec := h.request("/api/chat", chatRequest{Messages: h.messages, Action: action}, turnID)
	var resp chatResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || len(resp.Choices) == 0 {
		h.t.Fatalf("bad chat response %q: %v", rec.Body.String(), err)
	}
	reply := resp.Choices[0].Message.Content
	h.messages = append(h.messages, chatMessage{Role: "assistant", Content: reply})
	return reply, resp.State
}

func mustContain(t *testing.T, what, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("%s missing %q:\n%s", what, want, got)
		}
	}
}

func TestStudyAnyWeekAndLesson(t *testing.T) {
	fake, upstream := withFakeOpenRouter(t)
	h := newTutorHarness(t, upstream)

	welcome, state := h.bootstrap()
	mustContain(t, "welcome", welcome, "Welcome to iPyTutor", "any week", "Week 9 - Lists and Files", "week 6 lesson 2")
	if state.Phase != string(phaseWelcome) || len(state.Weeks) != 9 || state.Week != nil {
		t.Fatalf("welcome state = %+v", state)
	}
	if strings.Contains(strings.ToLower(welcome), "study code") {
		t.Fatal("welcome still mentions study codes")
	}

	// Straight to a lesson in the middle of a week, with no major and no earlier lessons.
	reply, state := h.send("week 6 lesson 2", "")
	mustContain(t, "week 6 lesson 2", reply, "Now studying **Week 6 - for Loops", "## A lesson", "Lesson 2 of 5")
	if state.Week == nil || state.Week.Number != 6 || len(state.Week.Lessons) != 5 || state.Activity == nil || state.Activity.Lesson != 2 {
		t.Fatalf("state = %+v", state)
	}
	lessonPrompt := fake.prompt("lesson")
	mustContain(t, "lesson prompt", lessonPrompt,
		"already knows everything from the earlier weeks (week 1: problem decomposition",
		"week 5: if statements", "This week they are learning: for loops",
		"may not have done this week's earlier lessons (Repeating with for and range())")

	reply, state = h.send("Lesson 4: Decisions inside a loop", "lesson:4")
	mustContain(t, "lesson 4", reply, "Lesson 4 of 5")
	if strings.Contains(reply, "Now studying") {
		t.Fatal("a lesson in the same week announced a week change")
	}
	reply, _ = h.send("It counts the matches", "")
	mustContain(t, "quick check reply", reply, "that's right", "**Interview prep:**")
	mustContain(t, "lesson reply prompt", fake.prompt("lesson_reply"), "**Interview prep:**")
	reply, _ = h.send("next lesson", "")
	mustContain(t, "last lesson", reply, "Lesson 5 of 5, the last one")
	reply, state = h.send("next lesson", "")
	mustContain(t, "past the last lesson", reply, "last of the 5 lessons", "`lesson 1`")
	if state.Activity != nil {
		t.Fatalf("activity after the last lesson = %+v", state.Activity)
	}
	reply, _ = h.send("lesson 9", "")
	mustContain(t, "no lesson 9", reply, "has 5 lessons")

	// Back to an earlier week, then forward by topic name.
	reply, state = h.send("week 2", "")
	mustContain(t, "week 2 overview", reply, "# Week 2 - Variables & Expressions", "assume you know everything from Week 1", "1. Variables and assignment", "Concept question")
	if state.Week.Number != 2 || state.Activity != nil {
		t.Fatalf("state = %+v", state)
	}
	reply, _ = h.send("teach me", "")
	mustContain(t, "week 2 first lesson", reply, "Lesson 1 of 5")
	reply, state = h.send("teach me for loops", "")
	mustContain(t, "topic switch", reply, "Now studying **Week 6", "Lesson 1 of 5")
	if state.Week.Number != 6 {
		t.Fatalf("week = %d, want 6", state.Week.Number)
	}
	reply, _ = h.send("study lists", "")
	mustContain(t, "topic overview", reply, "# Week 8 - Lists", "Weeks 1–7")

	// Naming this week's own topic is a question about it, not a switch.
	chats := fake.count("chat")
	reply, state = h.send("teach me about lists", "")
	if state.Week.Number != 8 || strings.Contains(reply, "Now studying") || fake.count("chat") != chats+1 {
		t.Fatalf("same-week topic: week %d, chat calls %d -> %d, reply:\n%s", state.Week.Number, chats, fake.count("chat"), reply)
	}
}

func TestMajorIsOptionalAndForgotten(t *testing.T) {
	_, upstream := withFakeOpenRouter(t)
	h := newTutorHarness(t, upstream)
	h.bootstrap()

	reply, state := h.send("I'm a biology major", "")
	mustContain(t, "major set", reply, "in **biology**", "Which week")
	if state.Major != "biology" || state.Phase != string(phaseChooseWeek) {
		t.Fatalf("state = %+v", state)
	}
	reply, _ = h.send("5", "")
	if strings.Contains(reply, "Tell me your major") {
		t.Fatal("overview asks for a major the student already gave")
	}
	reply, state = h.send("my major is Nursing", "")
	mustContain(t, "major changed", reply, "from now on", "**Nursing**")
	if state.Major != "Nursing" || state.Week.Number != 5 {
		t.Fatalf("state = %+v", state)
	}

	// A new page load starts fresh: nothing about the student is kept.
	_, state = h.bootstrap()
	if state.Major != "" || state.Week != nil || state.Phase != string(phaseWelcome) {
		t.Fatalf("state after reload = %+v", state)
	}
	reply, state = h.send("3", "")
	mustContain(t, "no major", reply, "# Week 3", "Tell me your major")
	if state.Major != "" {
		t.Fatalf("major = %q", state.Major)
	}
}

func TestCommandsBeforeChoosingAWeek(t *testing.T) {
	_, upstream := withFakeOpenRouter(t)
	h := newTutorHarness(t, upstream)
	h.bootstrap()
	// The first message after the welcome is not mistaken for a conversation lost in a
	// restart: the page's own "start" message made it look like one.
	reply, _ := h.send("hi", "")
	mustContain(t, "greeting", reply, "Tell me your major", "pick a week")
	reply, state := h.send("bug hunt", "")
	mustContain(t, "practice before a week", reply, "pick a week", "Week 5 - Conditionals")
	if state.Phase != string(phaseChooseWeek) {
		t.Fatalf("phase = %s", state.Phase)
	}
	reply, _ = h.send("lesson 3", "")
	mustContain(t, "lesson before a week", reply, "Lessons belong to a week", "week 5 lesson 2")
	reply, _ = h.send("help", "")
	mustContain(t, "help", reply, "week 6 lesson 2", "my major is Biology")
	reply, state = h.send("Study Week 7", "week:7")
	mustContain(t, "week from the menu", reply, "# Week 7 - while Loops & Menus")
	if state.Week.Number != 7 {
		t.Fatalf("week = %+v", state.Week)
	}
}

func TestCodeProblemStepsWithInterviewAdvice(t *testing.T) {
	fake, upstream := withFakeOpenRouter(t)
	h := newTutorHarness(t, upstream)
	h.bootstrap()
	h.send("week 5", "")

	reply, state := h.send("🧩 Code problem", "practice:code")
	mustContain(t, "code problem", reply, "Code problem: Seed germination sorter", "**Data available:**", "hint")
	if strings.Contains(reply, "REFERENCE") || strings.Contains(reply, "sprouted >= 20") {
		t.Fatalf("answer key leaked into the problem:\n%s", reply)
	}
	if state.Activity == nil || state.Activity.Stage != stageDecompose || !state.Activity.Open {
		t.Fatalf("activity = %+v", state.Activity)
	}
	reply, state = h.send("Input: the sprouted count. Process: compare it with 20. Output: ready or wait.", "")
	mustContain(t, "feedback", reply, "**Interview check:**", "**Interview prep:**")
	if strings.Contains(reply, "LEVEL:") || state.Activity.Stage != stageCode {
		t.Fatalf("stage %s after decomposition, reply:\n%s", state.Activity.Stage, reply)
	}
	mustContain(t, "feedback prompt", fake.prompt("feedback"), "**Interview prep:**", "Where they are: They are breaking the problem down")
	reply, _ = h.send("💡 Hint", "hint")
	mustContain(t, "hint", reply, "💡")
	_, state = h.send("```python\nsprouted = int(input())\nif sprouted >= 20:\n    print('ready')\n```", "")
	if state.Activity.Stage != stageExplain {
		t.Fatalf("stage after code = %s", state.Activity.Stage)
	}
	reply, state = h.send("The if checks whether 20 or more sprouted. I asked AI for the syntax and tested 19 and 20.", "")
	mustContain(t, "wrap-up", reply, "Code problem finished", "Another one")
	if state.Activity.Open || state.Activity.Stage != stageDone {
		t.Fatalf("activity after wrap-up = %+v", state.Activity)
	}
	reply, _ = h.send("hint", "")
	mustContain(t, "hint with no problem open", reply, "no practice problem open")

	// The next problem of the same kind avoids repeating this one.
	h.send("another one", "")
	if !strings.Contains(fake.prompt("problem_code"), "- Seed germination sorter") {
		t.Fatalf("next code problem prompt doesn't avoid the last one:\n%s", fake.prompt("problem_code"))
	}
}

func TestWeek1OffersOnlyDecomposition(t *testing.T) {
	_, upstream := withFakeOpenRouter(t)
	h := newTutorHarness(t, upstream)
	h.bootstrap()
	reply, state := h.send("1", "")
	mustContain(t, "week 1 overview", reply, "Decomposition problem")
	if strings.Contains(reply, "Bug hunt") || strings.Contains(reply, "assume you know") || len(state.Week.PracticeKinds) != 1 {
		t.Fatalf("week 1 overview: %+v\n%s", state.Week.PracticeKinds, reply)
	}
	reply, _ = h.send("code problem", "")
	mustContain(t, "no code problem in week 1", reply, "decomposition only")
	reply, state = h.send("decomposition problem", "")
	mustContain(t, "decomposition problem", reply, "Decomposition problem:")
	if state.Activity == nil || state.Activity.Stage != stageAnswer {
		t.Fatalf("activity = %+v", state.Activity)
	}
}

func TestBugHuntStepsAndSolution(t *testing.T) {
	fake, upstream := withFakeOpenRouter(t)
	h := newTutorHarness(t, upstream)
	h.bootstrap()
	h.send("week 5", "")
	reply, state := h.send("🐞 Bug hunt", "practice:bug")
	mustContain(t, "bug hunt", reply, "Bug hunt: Grade boundary checker", "```python")
	if state.Activity.Stage != stageStrategy {
		t.Fatalf("stage = %s", state.Activity.Stage)
	}
	fake.setFeedbackLevel("none")
	_, state = h.send("What does elif do again?", "")
	if state.Activity.Stage != stageStrategy {
		t.Fatalf("a question moved the stage on to %s", state.Activity.Stage)
	}
	fake.setFeedbackLevel("competent")
	_, state = h.send("I'd test 95 and 89 and print which branch runs; the >= 60 check probably comes first.", "")
	if state.Activity.Stage != stageFix {
		t.Fatalf("stage after the strategy = %s", state.Activity.Stage)
	}
	reply, state = h.send("✅ Show solution", "solution")
	mustContain(t, "solution", reply, "worked solution", "**Interview prep:**")
	if state.Activity.Open {
		t.Fatal("problem still open after the solution")
	}
	reply, state = h.send("another one", "")
	mustContain(t, "another bug hunt", reply, "Bug hunt:")
	if state.Activity.Kind != kindBug || !state.Activity.Open {
		t.Fatalf("activity = %+v", state.Activity)
	}
}

func TestRestartAsksForAWeek(t *testing.T) {
	_, upstream := withFakeOpenRouter(t)
	h := newTutorHarness(t, upstream)
	h.bootstrap()
	h.send("week 5", "")
	h.send("teach me", "")

	// A new tutor has no record of the session; the browser still has the conversation.
	restarted := newTutorHarness(t, upstream)
	restarted.messages = h.messages
	reply, state := restarted.send("what was I doing?", "")
	mustContain(t, "after restart", reply, "restarted", "Pick a week", "Week 5 - Conditionals")
	if state.Phase != string(phaseChooseWeek) {
		t.Fatalf("phase = %s", state.Phase)
	}
	reply, state = restarted.send("week 5 lesson 2", "")
	mustContain(t, "carry on", reply, "Lesson 2 of 5")
	if state.Week.Number != 5 {
		t.Fatalf("week = %+v", state.Week)
	}
}

func TestRepeatedTurnIDReplaysReply(t *testing.T) {
	fake, upstream := withFakeOpenRouter(t)
	h := newTutorHarness(t, upstream)
	h.bootstrap()
	h.send("week 2", "")
	first, _ := h.sendTurn("What is a float?", "", "same-turn")
	calls := fake.count("chat")
	h.messages = h.messages[:len(h.messages)-2]
	again, _ := h.sendTurn("What is a float?", "", "same-turn")
	if again != first || fake.count("chat") != calls {
		t.Fatalf("retry ran the turn again: %q vs %q, chat calls %d -> %d", first, again, calls, fake.count("chat"))
	}
}

func TestTutorModelUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream down", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	h := newTutorHarness(t, server.URL)
	h.bootstrap()
	h.send("3", "")
	reply, state := h.send("teach me", "")
	mustContain(t, "lesson outline fallback", reply, "## Getting input from the user", "couldn't reach the tutor model")
	if state.Activity == nil || state.Activity.Kind != kindLesson {
		t.Fatalf("activity = %+v", state.Activity)
	}
	reply, _ = h.send("next lesson", "")
	mustContain(t, "second lesson outline", reply, "## Converting types")
	reply, _ = h.send("Why does input return a string?", "")
	mustContain(t, "chat fallback", reply, "couldn't reach the tutor model")
}
