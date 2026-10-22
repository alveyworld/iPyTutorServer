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
// expects, streamed like OpenRouter, and counts the calls it gets.
type fakeOpenRouter struct {
	mu    sync.Mutex
	calls map[string]int
	// feedbackLevel is the LEVEL the next feedback replies end with.
	feedbackLevel string
}

func (f *fakeOpenRouter) count(kind string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[kind]
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
		return "solution", "Here is the worked solution."
	case strings.Contains(task, "How to reply:"):
		level := f.feedbackLevel
		if level == "" {
			level = "competent"
		}
		return "feedback", "You named the input clearly.\n\n**Interview check:** this would likely be rated **Competent**.\n\nNow paste your code.\nLEVEL: " + level
	case strings.Contains(task, "quick check question"):
		return "lesson_reply", "Yes, that's right."
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
	code     string
	turnN    int
}

// newTutorHarness makes a tutor whose model calls go to upstream. Problems it prepares in
// the background may outlive the test; they then fail against the closed fake.
func newTutorHarness(t *testing.T, dir string, upstream string) *tutorHarness {
	t.Helper()
	store, err := newProgressStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	llm := llmClient{apiKey: "test-key", url: upstream}
	return &tutorHarness{t: t, tutor: newTutor(llm, store), turns: newTurnStore(), session: "session-" + t.Name()}
}

func withFakeOpenRouter(t *testing.T) (*fakeOpenRouter, string) {
	t.Helper()
	fake := &fakeOpenRouter{calls: map[string]int{}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	return fake, server.URL
}

func (h *tutorHarness) request(method, path string, body any, turnID string) *httptest.ResponseRecorder {
	h.t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req = req.WithContext(context.WithValue(req.Context(), sessionIDContextKey, h.session))
	if turnID != "" {
		req.Header.Set(turnIDHeader, turnID)
	}
	rec := httptest.NewRecorder()
	switch path {
	case "/api/session/bootstrap":
		h.tutor.handleBootstrap(rec, req)
	case "/api/session/state":
		h.tutor.handleState(rec, req)
	default:
		h.tutor.handleChat(h.turns)(rec, req)
	}
	if rec.Code != http.StatusOK {
		h.t.Fatalf("%s %s: status %d: %s", method, path, rec.Code, rec.Body.String())
	}
	return rec
}

func (h *tutorHarness) bootstrap() (string, stateResponse) {
	h.t.Helper()
	var resp bootstrapResponse
	rec := h.request(http.MethodPost, "/api/session/bootstrap", bootstrapRequest{StudyCode: h.code}, "")
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
	rec := h.request(http.MethodPost, "/api/chat", chatRequest{Messages: h.messages, Action: action, StudyCode: h.code}, turnID)
	var resp chatResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || len(resp.Choices) == 0 {
		h.t.Fatalf("bad chat response %q: %v", rec.Body.String(), err)
	}
	reply := resp.Choices[0].Message.Content
	h.messages = append(h.messages, chatMessage{Role: "assistant", Content: reply})
	if resp.State.StudyCode != "" {
		h.code = resp.State.StudyCode
	}
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

func TestStudentJourney(t *testing.T) {
	_, upstream := withFakeOpenRouter(t)
	dir := t.TempDir()
	h := newTutorHarness(t, dir, upstream)

	welcome, state := h.bootstrap()
	mustContain(t, "welcome", welcome, "Welcome to iPyTutor", "major", "study code")
	if state.Phase != string(phaseWelcome) {
		t.Fatalf("phase = %s", state.Phase)
	}

	reply, state := h.send("I'm a biology major", "")
	if state.StudyCode == "" || state.Major != "biology" || state.Phase != string(phaseChooseWeek) {
		t.Fatalf("after major: %+v", state)
	}
	mustContain(t, "study code reply", reply, state.StudyCode, "Week 5 - Conditionals")

	reply, state = h.send("5", "")
	mustContain(t, "week plan", reply, "# Week 5 - Conditionals (if/elif/else)", "Teach me", "Bug hunt", "lesson 1")
	if state.Week == nil || state.Week.Number != 5 || len(state.Week.PracticeKinds) != 3 {
		t.Fatalf("week state = %+v", state.Week)
	}

	reply, state = h.send("📖 Teach me", "lesson")
	mustContain(t, "lesson", reply, "## A lesson", "Lesson 1 of 5")
	if state.Activity == nil || state.Activity.Kind != kindLesson || state.Activity.Lesson != 1 {
		t.Fatalf("activity = %+v", state.Activity)
	}
	reply, state = h.send("It compares two values", "")
	mustContain(t, "lesson reply", reply, "that's right")
	if state.Week.LessonsDone != 1 {
		t.Fatalf("lessons done = %d, want 1", state.Week.LessonsDone)
	}
	reply, state = h.send("next lesson", "")
	mustContain(t, "second lesson", reply, "Lesson 2 of 5")

	// A code problem runs decompose → code → explain, as in the interview.
	reply, state = h.send("🧩 Code problem", "practice:code")
	mustContain(t, "code problem", reply, "Code problem: Seed germination sorter", "**Data available:**", "hint")
	if strings.Contains(reply, "REFERENCE") || strings.Contains(reply, "sprouted >= 20") {
		t.Fatalf("answer key leaked into the problem:\n%s", reply)
	}
	if state.Activity == nil || state.Activity.Stage != stageDecompose || !state.Activity.Open {
		t.Fatalf("activity = %+v", state.Activity)
	}
	_, state = h.send("Input: the sprouted count. Process: compare it with 20. Output: ready or wait.", "")
	if state.Activity.Stage != stageCode {
		t.Fatalf("stage after decomposition = %s", state.Activity.Stage)
	}
	reply, state = h.send("💡 Hint", "hint")
	mustContain(t, "hint", reply, "💡")
	_, state = h.send("```python\nsprouted = int(input())\nif sprouted >= 20:\n    print('ready')\n```", "")
	if state.Activity.Stage != stageExplain {
		t.Fatalf("stage after code = %s", state.Activity.Stage)
	}
	reply, state = h.send("The if checks whether 20 or more sprouted. I asked AI for the syntax and tested 19 and 20.", "")
	mustContain(t, "wrap-up", reply, "Code problem finished", "Readiness for Week 5")
	if state.Activity.Open || state.Activity.Stage != stageDone {
		t.Fatalf("activity after wrap-up = %+v", state.Activity)
	}
	for _, part := range state.Week.Readiness.Parts {
		if part.Kind == kindCode && (part.Level != levelCompetent || part.Attempts != 1) {
			t.Fatalf("code part = %+v", part)
		}
	}
	reply, _ = h.send("hint", "")
	mustContain(t, "hint with no problem open", reply, "no practice problem open")

	reply, _ = h.send("my progress", "")
	mustContain(t, "progress", reply, "| **Week 5 - Conditionals (if/elif/else)** | 1/5 | – | Competent | – | Keep practicing |")

	// Progress is on disk: a new store on the same directory sees it.
	reopened, err := newProgressStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := reopened.get(h.code)
	if err != nil {
		t.Fatal(err)
	}
	wp := p.Weeks[5]
	// Lesson 2 was shown but not answered, so only lesson 1 is done.
	if p.CurrentWeek != 5 || !wp.lessonDone(0) || wp.lessonDone(1) || len(wp.Practice) != 1 {
		t.Fatalf("saved progress = %+v", wp)
	}
	if rec := wp.Practice[0]; rec.Level != levelCompetent || rec.Attempts != 3 || rec.HintsUsed != 1 || rec.Title != "Seed germination sorter" {
		t.Fatalf("saved record = %+v", rec)
	}

	// A server restart loses the session; the browser's study code restores it.
	restarted := newTutorHarness(t, dir, upstream)
	restarted.code, restarted.messages = h.code, h.messages
	reply, state = restarted.send("what was I doing?", "")
	mustContain(t, "after restart", reply, "restarted", "progress is saved")
	if state.Phase != string(phaseStudying) || state.Week == nil || state.Week.Number != 5 {
		t.Fatalf("restored state = %+v", state)
	}

	// Reloading the page with a remembered code welcomes the student back.
	welcome, state = restarted.bootstrap()
	mustContain(t, "welcome back", welcome, "Welcome back", "Week 5 - Conditionals", "Lessons: 1 of 5 done")
	if state.Week == nil || state.Activity != nil {
		t.Fatalf("state after reload = %+v", state)
	}
}

func TestReturningStudentTypesStudyCode(t *testing.T) {
	_, upstream := withFakeOpenRouter(t)
	dir := t.TempDir()
	first := newTutorHarness(t, dir, upstream)
	first.bootstrap()
	first.send("Nursing", "")
	first.send("week 1", "")
	code := first.code

	other := newTutorHarness(t, dir, upstream)
	other.session = "another-browser"
	other.bootstrap()
	reply, _ := other.send("bogus-code-1234", "")
	mustContain(t, "unknown code", reply, "couldn't find")
	reply, state := other.send(strings.ToUpper(strings.ReplaceAll(code, "-", " ")), "")
	mustContain(t, "resumed", reply, "Welcome back", "Week 1 - Problem Decomposition")
	if state.StudyCode != code || state.Week == nil || state.Week.Number != 1 {
		t.Fatalf("resumed state = %+v", state)
	}

	// "Use a different study code" clears the browser's remembered code.
	other.code = ""
	for i := 0; i < maxFailedCodeTries+1; i++ {
		other.tutor.resetSession(other.tutor.sessions.getOrCreate(other.session))
		reply, _ = other.send("wrong-guess-1111", "")
	}
	mustContain(t, "rate limit", reply, "too many study codes")
}

func TestWeek1OffersOnlyDecomposition(t *testing.T) {
	_, upstream := withFakeOpenRouter(t)
	h := newTutorHarness(t, t.TempDir(), upstream)
	h.bootstrap()
	h.send("Business", "")
	reply, state := h.send("1", "")
	mustContain(t, "week 1 plan", reply, "Decomposition problem")
	if strings.Contains(reply, "Bug hunt") || len(state.Week.PracticeKinds) != 1 {
		t.Fatalf("week 1 offers more than decomposition: %+v\n%s", state.Week.PracticeKinds, reply)
	}
	reply, _ = h.send("code problem", "")
	mustContain(t, "no code problem in week 1", reply, "decomposition only")
	reply, state = h.send("decomposition problem", "")
	mustContain(t, "decomposition problem", reply, "Decomposition problem:")
	if state.Activity == nil || state.Activity.Stage != stageAnswer {
		t.Fatalf("activity = %+v", state.Activity)
	}
}

func TestBugHuntStagesAndSolution(t *testing.T) {
	fake, upstream := withFakeOpenRouter(t)
	h := newTutorHarness(t, t.TempDir(), upstream)
	h.bootstrap()
	h.send("Music", "")
	h.send("week 5", "")
	reply, state := h.send("🐞 Bug hunt", "practice:bug")
	mustContain(t, "bug hunt", reply, "Bug hunt: Grade boundary checker", "```python")
	if state.Activity.Stage != stageStrategy {
		t.Fatalf("stage = %s", state.Activity.Stage)
	}
	fake.mu.Lock()
	fake.feedbackLevel = "none"
	fake.mu.Unlock()
	_, state = h.send("What does elif do again?", "")
	if state.Activity.Stage != stageStrategy {
		t.Fatalf("a question moved the stage on to %s", state.Activity.Stage)
	}
	reply, state = h.send("✅ Show solution", "solution")
	mustContain(t, "solution", reply, "worked solution")
	if state.Activity.Open {
		t.Fatal("problem still open after the solution")
	}
	p, _ := h.tutor.store.get(h.code)
	if rec := p.Weeks[5].Practice[0]; !rec.SolutionShown || rec.Level != "" || rec.Attempts != 0 {
		t.Fatalf("record = %+v", rec)
	}
	reply, state = h.send("another one", "")
	mustContain(t, "another bug hunt", reply, "Bug hunt:")
	if state.Activity.Kind != kindBug || !state.Activity.Open {
		t.Fatalf("activity = %+v", state.Activity)
	}
}

func TestRepeatedTurnIDReplaysReply(t *testing.T) {
	fake, upstream := withFakeOpenRouter(t)
	h := newTutorHarness(t, t.TempDir(), upstream)
	h.bootstrap()
	h.send("Art", "")
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
	h := newTutorHarness(t, t.TempDir(), server.URL)
	h.bootstrap()
	h.send("History", "")
	h.send("3", "")
	reply, state := h.send("teach me", "")
	mustContain(t, "lesson outline fallback", reply, "## Getting input from the user", "couldn't reach the tutor model")
	if state.Activity == nil || state.Activity.Kind != kindLesson {
		t.Fatalf("activity = %+v", state.Activity)
	}
	reply, _ = h.send("What is a string?", "lesson")
	mustContain(t, "second lesson outline", reply, "## Converting types")
	reply, _ = h.send("Why does input return a string?", "")
	mustContain(t, "chat fallback", reply, "couldn't reach the tutor model")
}
