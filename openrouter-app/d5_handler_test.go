package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// fakeD5Upstream plays OpenRouter for the D5 engine: streamed Interviewer calls, the
// Evaluator brief, and the levels-only call, each recognised from its request.
type fakeD5Upstream struct {
	mu          sync.Mutex
	streamed    int
	evaluator   int
	levels      int
	questionSeq int
	recommend   string
	drafts      int
	// evaluatorBroken makes every Evaluator call return an unusable brief.
	evaluatorBroken bool
	lastRequests    []d5Request
}

var dimensionsLine = regexp.MustCompile(`Dimensions(?: for this part)?: ([a-z_, ]+)\.`)

func (f *fakeD5Upstream) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req d5Request
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Errorf("bad request: %v", err)
			return
		}
		system := req.Messages[0].Content
		f.mu.Lock()
		f.lastRequests = append(f.lastRequests, req)
		f.mu.Unlock()

		if req.Stream {
			if req.Reasoning["enabled"] != false || req.MaxTokens == 0 || req.Provider["require_parameters"] != true {
				t.Errorf("interviewer request missing D5 settings: %+v", req)
			}
			f.mu.Lock()
			f.streamed++
			f.questionSeq++
			n := f.questionSeq
			f.mu.Unlock()
			writeSSE(w, f.interviewerReply(system, n))
			return
		}

		dims := "conceptual"
		if m := dimensionsLine.FindStringSubmatch(system); m != nil {
			dims = strings.Split(m[1], ",")[0]
		}
		var content string
		if strings.Contains(system, "Start the coding part") || strings.Contains(system, "Start the debugging part") {
			// A pre-drafted opening (Phase 2): non-streamed, reasoning on.
			f.mu.Lock()
			f.drafts++
			f.questionSeq++
			n := f.questionSeq
			f.mu.Unlock()
			content = f.interviewerReply(system, n)
		} else if req.MaxTokens == 40 {
			content = "yes"
		} else if req.MaxTokens == 60 {
			f.mu.Lock()
			f.levels++
			f.mu.Unlock()
			content = "LEVELS: " + strings.TrimSpace(dims) + "=competent"
		} else {
			f.mu.Lock()
			f.evaluator++
			rec := f.recommend
			broken := f.evaluatorBroken
			f.mu.Unlock()
			if broken {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": ""}, "finish_reason": "length"}}})
				return
			}
			if rec == "" {
				rec = "continue"
			}
			content = fmt.Sprintf("QUALITY: solid\nCLARIFICATION: no\nEVIDENCE: clear reasoning\nCOVERED: basics\nGAPS: edge cases\nNEXT: Probe an edge case.\nFALLBACK: What would happen with an unusual value?\nAVOID: none\nRECOMMEND: %s\nLEVELS: %s=competent\nISSUES: none", rec, strings.TrimSpace(dims))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": content}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 100, "completion_tokens": 20},
		})
	}
}

// fakeQuestions share no content words, so the repeated-question rule never fires.
var fakeQuestions = []string{
	"Thanks, that's helpful. What would happen with a refund?",
	"I appreciate that. Which customers matter most during holidays?",
	"Good to know. Where do allergies fit into this?",
	"That makes sense. When might bulk pricing apply?",
	"Thanks for sharing. Who checks late payments?",
	"Interesting. Why would gift vouchers complicate things?",
	"Okay. How should weekend staffing look?",
	"Thanks. What about returned pastries?",
	"Helpful context. Which loyalty perks seem fair?",
	"Got that. How often do suppliers deliver flour?",
}

func (f *fakeD5Upstream) interviewerReply(system string, n int) string {
	switch {
	case strings.Contains(system, "COMPANY: <company name>"):
		return "COMPANY: Brightline Bakery | small-batch bakery with online ordering\nWe sort online orders into delivery bands by total. How would you decide which band an order belongs in?"
	case strings.Contains(system, "Start the coding part"):
		return "Data available: an order subtotal and whether it is a pickup.\nWhat's wanted: the amount owed.\nHow would you break this problem down before writing any code?"
	case strings.Contains(system, "Start the debugging part"):
		return "This script should print the amount owed.\n```python\ntotal = 60\nif total > 50:\n    total = total * 9  # Bug: should multiply by 0.9\nprint(total)\n```\nHow would you go about finding what's wrong here?\nDEFECT: line 3 multiplies by 9 instead of 0.9, so the total is ten times too large."
	case strings.Contains(system, "You coach interview skills"):
		return "You showed clear reasoning throughout. Keep practising edge cases. How did the interview feel?"
	case strings.Contains(system, "paste it here") || strings.Contains(system, "pasted here"):
		return "That's a clear plan. Please write the Python code for this task and paste it here; AI tools are fine to use."
	case strings.Contains(system, "used AI tools"):
		return "Thanks for explaining that. Did you use any AI tools while writing it, and how did you check them?"
	default:
		return fakeQuestions[n%len(fakeQuestions)]
	}
}

func writeSSE(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := w.(http.Flusher)
	_, _ = io.WriteString(w, ": OPENROUTER PROCESSING\n\n")
	for _, part := range strings.SplitAfter(content, " ") {
		chunk, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"delta": map[string]string{"content": part}}}})
		_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
		if flusher != nil {
			flusher.Flush()
		}
	}
	final, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{{"delta": map[string]string{}, "finish_reason": "stop"}},
		"usage":   map[string]any{"prompt_tokens": 1400, "completion_tokens": 30},
	})
	_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", final)
}

type d5TestClient struct {
	t        *testing.T
	states   *agentStateStore
	turns    *turnStore
	upstream *fakeD5Upstream
	turn     int
}

func newD5TestClient(t *testing.T) *d5TestClient {
	t.Helper()
	f := &fakeD5Upstream{}
	server := httptest.NewServer(f.handler(t))
	t.Cleanup(server.Close)
	savedURL, savedEngine := openRouterURL, chatEngine
	openRouterURL, chatEngine = server.URL, "d5"
	t.Cleanup(func() { openRouterURL, chatEngine = savedURL, savedEngine })
	c := &d5TestClient{t: t, states: newAgentStateStore(), turns: newTurnStore(), upstream: f}
	// Registered last, so it runs first: let background Evaluator and label jobs finish
	// before the fake upstream closes and the globals are restored.
	t.Cleanup(c.drainBackgroundJobs)
	return c
}

// say sends one student message and returns the reply plus the streamed Interviewer calls it made.
func (c *d5TestClient) say(text string) (string, int) {
	c.t.Helper()
	c.upstream.mu.Lock()
	before := c.upstream.streamed
	c.upstream.mu.Unlock()
	c.turn++
	body, _ := json.Marshal(chatCompletionRequest{Messages: []chatMessage{{Role: "user", Content: text}}})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(string(body)))
	req = req.WithContext(context.WithValue(req.Context(), sessionIDContextKey, "session-d5"))
	req.Header.Set(turnIDHeader, fmt.Sprintf("turn-%d", c.turn))
	rec := httptest.NewRecorder()
	handleChat("test-key", c.states, c.turns)(rec, req)
	if rec.Code != http.StatusOK {
		c.t.Fatalf("%q: status %d: %s", text, rec.Code, rec.Body.String())
	}
	c.upstream.mu.Lock()
	calls := c.upstream.streamed - before
	c.upstream.mu.Unlock()
	return extractAssistantContent(rec.Body.Bytes()), calls
}

func (c *d5TestClient) drainBackgroundJobs() {
	st, ok := c.states.get("session-d5")
	if !ok || st.D5 == nil {
		return
	}
	st.D5.mu.Lock()
	ch := st.D5.briefInFlight
	st.D5.mu.Unlock()
	if ch != nil {
		<-ch
	}
	st.D5.labelJobs.Wait()
	st.D5.mu.Lock()
	var drafts []chan struct{}
	for _, d := range st.D5.draftDone {
		drafts = append(drafts, d)
	}
	st.D5.mu.Unlock()
	for _, d := range drafts {
		<-d
	}
}

func (c *d5TestClient) state() *AgentSessionState {
	s, _ := c.states.get("session-d5")
	return s
}

const d5TestCode = "```python\nsubtotal = float(input('Subtotal: '))\nif subtotal > 50:\n    subtotal = subtotal * 0.9\nprint(subtotal)\n```"

func TestD5FullInterviewWeek5(t *testing.T) {
	c := newD5TestClient(t)
	if reply, _ := c.say("Nutrition Science"); !strings.Contains(reply, "Nutrition Science") {
		t.Fatalf("setup reply = %q", reply)
	}

	opening, calls := c.say("5")
	if calls != 1 || !strings.HasPrefix(opening, "Hi, I'm ") || !strings.Contains(opening, "hiring manager at Brightline Bakery") || strings.Contains(opening, "COMPANY:") {
		t.Fatalf("first opening (%d calls) = %q", calls, opening)
	}

	answer := "I would compare the order total against each band threshold, highest first, so every order lands in exactly one band."
	var reply string
	for i := 0; i < 5; i++ {
		reply, calls = c.say(answer + fmt.Sprintf(" Detail %d.", i))
		if i < 4 && calls != 1 {
			t.Fatalf("conceptual answer %d made %d interviewer calls", i, calls)
		}
	}
	if !strings.Contains(reply, phaseClosingMessage) || !strings.Contains(reply, "software developer") || !strings.Contains(reply, "Data available") {
		t.Fatalf("conceptual close + code opening = %q; asked=%q similar=%d vague=%d", reply, c.state().ModeQuestionsAsked, c.state().ModeSimilarQuestionAsks, c.state().ModeVagueAnswers)
	}
	if got := c.state().ActiveMode; got != modeCode {
		t.Fatalf("mode after conceptual = %s", got)
	}

	if reply, _ = c.say("First get the subtotal, then apply the discount if it is over 50, then add the fee unless it's a pickup, then show the total."); !strings.Contains(reply, "paste it here") || strings.Count(reply, "paste it here") != 1 {
		t.Fatalf("after decomposition = %q", reply)
	}
	if c.state().ModeInterviewStep != interviewStepAwaitingCode {
		t.Fatalf("step = %s, want awaiting code", c.state().ModeInterviewStep)
	}
	c.say(d5TestCode)
	if reply, _ = c.say("Multiplying by 0.9 takes ten percent off, which is the discount the bakery wants on bigger orders."); !strings.Contains(reply, "AI") {
		t.Fatalf("expected AI-use question, got %q", reply)
	}
	reply, _ = c.say("I asked an AI to explain float(input()) and then tested with 40 and 60 dollar orders to check the discount.")
	if !strings.Contains(reply, "QA engineer") || !strings.Contains(reply, "```python") {
		t.Fatalf("code close + bug opening = %q", reply)
	}
	if strings.Contains(reply, "DEFECT") || !strings.Contains(c.state().D5.BugDefect, "multiplies by 9") {
		t.Fatalf("hidden DEFECT line leaked or not stored: reply=%q defect=%q", reply, c.state().D5.BugDefect)
	}

	for i := 0; i < 4; i++ {
		reply, _ = c.say(fmt.Sprintf("I'd run it with a total of 60 and print total before and after the if line to see where the value goes wrong. Step %d.", i))
	}
	if !strings.Contains(reply, d5ClosingInterview) || !strings.Contains(reply, "Overall Rating: Competent") {
		t.Fatalf("results reply = %q", reply)
	}
	st := c.state()
	if st.ConversationPhase != phaseAssessmentResults || st.AssessmentEndTime == nil {
		t.Fatalf("phase = %s end=%v", st.ConversationPhase, st.AssessmentEndTime)
	}

	if reply, _ = c.say("thanks"); reply != d5ResultsReminder {
		t.Fatalf("post-results reply = %q", reply)
	}
	if reply, _ = c.say("switch to coach mode"); !strings.Contains(reply, "recruiter on the HR team at Brightline Bakery") {
		t.Fatalf("coaching reply = %q", reply)
	}
}

func TestD5CoachingDeferredDuringInterview(t *testing.T) {
	c := newD5TestClient(t)
	c.say("Nursing")
	c.say("5")
	reply, calls := c.say("can I switch to coaching?")
	if calls != 0 || !strings.HasPrefix(reply, d5CoachingAfterwards) || !strings.Contains(reply, "How would you decide which band") {
		t.Fatalf("deferred coaching (%d calls) = %q", calls, reply)
	}
	if c.state().ActiveMode != modeConceptual || c.state().CoachingEnteredBeforeResults {
		t.Fatal("coaching must not start before results")
	}
}

// An answer that mentions coaching, or code with the word in a print string, is an answer,
// not a request: it used to be answered with "Coaching opens once the interview is finished".
func TestD5AnswerMentioningCoachingIsStillAnAnswer(t *testing.T) {
	for _, answer := range []string{
		"I would keep a list of coaching sessions.",
		"```python\nprint('Thanks for the coaching session!')\n```",
	} {
		c := newD5TestClient(t)
		c.say("Nursing")
		c.say("5")
		reply, _ := c.say(answer)
		if strings.HasPrefix(reply, d5CoachingAfterwards) {
			t.Errorf("answer %q was taken for a coaching request: %q", answer, reply)
		}
		if got := c.state().D5.AnswerIndex; got != 1 {
			t.Errorf("answer %q was not recorded as an answer (AnswerIndex %d)", answer, got)
		}
	}
}

func TestD5Week1EndsAfterConceptual(t *testing.T) {
	c := newD5TestClient(t)
	c.upstream.recommend = "close"
	c.say("Nutrition Science")
	c.say("1")
	var reply string
	for i := 0; i < 5 && c.state().ConversationPhase == phaseAssessmentInProgress; i++ {
		reply, _ = c.say(fmt.Sprintf("I would list each client's goals and allergies first, then plan meals, then hand them a weekly table. Point %d.", i))
	}
	if !strings.Contains(reply, "Code Assessment: N/A") || !strings.Contains(reply, "Overall Rating: Competent") {
		t.Fatalf("week 1 results = %q", reply)
	}
}

func TestD5ReplayReturnsSameReply(t *testing.T) {
	c := newD5TestClient(t)
	c.say("Nutrition Science")
	first, _ := c.say("5")
	c.turn-- // resend the same turn ID
	again, calls := c.say("5")
	if again != first || calls != 0 {
		t.Fatalf("replay made %d calls and returned %q", calls, again)
	}
}

func TestD5GradesWhenEvaluatorFails(t *testing.T) {
	c := newD5TestClient(t)
	c.upstream.evaluatorBroken = true
	c.say("Nutrition Science")
	c.say("1")
	var reply string
	for i := 0; i < 6 && c.state().ConversationPhase == phaseAssessmentInProgress; i++ {
		reply, _ = c.say(fmt.Sprintf("I would list each client's goals and allergies first, then plan meals, then hand them a weekly table. Point %d.", i))
	}
	// Every answer is labelled competent by the levels-only backfill at close.
	if !strings.Contains(reply, "Overall Rating: Competent") {
		t.Fatalf("results without Evaluator labels = %q", reply)
	}
	if c.upstream.levels < 2 {
		t.Fatalf("expected the close to label all unlabelled answers, got %d levels calls", c.upstream.levels)
	}
}

func TestD5CodeModeClosesAfterRepeatedRequests(t *testing.T) {
	state, sess := d5TestState(5, modeCode)
	askAndAnswer(state, sess, d5Move{Kind: moveOpenMode, Target: dimDecomposition}, "How would you break this down?", "Read the subtotal, apply the discount, add the fee, print the total.")
	for i := 0; i < d5MaxCodeRequests; i++ {
		move := d5ChooseMove(state, sess, nil, "I'm still thinking about it, one moment please.", false)
		if move.Kind != moveRequestCode {
			t.Fatalf("request %d: move = %+v", i+1, move)
		}
		askAndAnswer(state, sess, move, "Please paste your Python code here.", "I'm still thinking about it, one moment please.")
	}
	if got := d5ChooseMove(state, sess, nil, "still thinking", false); got.Kind != moveCloseMode {
		t.Fatalf("after %d requests: %+v, want CLOSE_MODE", d5MaxCodeRequests, got)
	}
}

func TestD5CoachingPromptIsAboutInterviewing(t *testing.T) {
	state, sess := d5TestState(5, modeConceptual)
	sess.CompanyName = "Brightline Bakery"
	askAndAnswer(state, sess, d5Move{Kind: moveOpenMode, Target: dimConceptual}, "How would you check the numbers?", "maybe I'd sort of check the numbers")
	prompt := d5CoachingSystemPrompt(state, sess)
	for _, want := range []string{"recruiter on the HR team at Brightline Bakery", "maybe I'd sort of check the numbers", "Do not coach on code", "code-learning tool"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("coaching prompt missing %q", want)
		}
	}
	if strings.Contains(prompt, "Python ideas") {
		t.Error("coaching prompt should not offer Python practice")
	}
}

func TestD5CoachingPromptShowsFullAnswersWithTheirQuestions(t *testing.T) {
	state, sess := d5TestState(5, modeConceptual)
	long := strings.Repeat("I would compare the total against each band, highest first, so every order lands in one band. ", 8) + "The very last sentence matters."
	if len(long) <= 400 {
		t.Fatal("test answer must be longer than the old 400-byte cut")
	}
	askAndAnswer(state, sess, d5Move{Kind: moveOpenMode, Target: dimConceptual}, "A bakery sorts orders into bands. How would you decide the band?", long)
	askAndAnswer(state, sess, d5Move{Kind: moveFollowUp, Target: dimConceptual}, "Why does the order of the checks matter?", "Because the first true branch wins.")
	sess.addLabels(modeConceptual, []gradeLabel{
		{Dimension: dimConceptual, Level: levelExceptional, AnswerIndex: 1, Source: labelSourceEvaluator},
		{Dimension: dimConceptual, Level: levelCompetent, AnswerIndex: 2, Source: labelSourceEvaluator},
	})
	prompt := d5CoachingSystemPrompt(state, sess)
	// Per-answer ratings are internal and must not reach the coach.
	if strings.Contains(prompt, "rated it") {
		t.Error("coaching prompt shows per-answer ratings")
	}
	if !strings.Contains(prompt, "The very last sentence matters.") {
		t.Error("a long answer was cut short in the coaching prompt")
	}
	for _, want := range []string{
		"Question 1: How would you decide the band?",
		"Question 2: Why does the order of the checks matter?",
		"Answer 2: Because the first true branch wins.",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("coaching prompt missing %q", want)
		}
	}
	if strings.Contains(prompt, "...") {
		t.Error("coaching prompt still contains a truncation marker")
	}
}

func TestD5CoachingPromptSaysAIUseIsExpected(t *testing.T) {
	state, sess := d5TestState(5, modeCode)
	askAndAnswer(state, sess, d5Move{Kind: moveCodeFollowUp, Target: dimAIUse}, "Did you use any AI tools?", "Yes, an AI assistant wrote the first version and I tested it.")
	prompt := d5CoachingSystemPrompt(state, sess)
	for _, want := range []string{"expected and encouraged to use AI tools", "never suggest hiding or downplaying it"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("coaching prompt missing %q", want)
		}
	}
}

func TestD5CoachingPromptOmitsInternalEvaluatorNotes(t *testing.T) {
	state, sess := d5TestState(5, modeConceptual)
	askAndAnswer(state, sess, d5Move{Kind: moveOpenMode, Target: dimConceptual}, "How would you decide?", "Highest band first.")
	sess.Evidence[modeConceptual] = []string{"named the order of checks"}
	sess.ModeBriefs[modeConceptual] = &d5Brief{Gaps: []string{"edge cases at a boundary"}}
	prompt := d5CoachingSystemPrompt(state, sess)
	for _, leak := range []string{"did not get to see", "What the interviewers saw", "named the order of checks", "edge cases at a boundary"} {
		if strings.Contains(prompt, leak) {
			t.Errorf("coaching prompt leaks evaluator notes: %q", leak)
		}
	}
}

func TestD5ClipAnswerMarksTheCut(t *testing.T) {
	short := "print('hello')"
	if got := d5ClipAnswer(short); got != short {
		t.Fatalf("short answer changed: %q", got)
	}
	long := strings.Repeat("word ", d5CoachingAnswerClip)
	got := d5ClipAnswer(long)
	if !strings.Contains(got, "[clipped here") || len(got) > d5CoachingAnswerClip+80 {
		t.Fatalf("long answer not clipped with a note: len %d", len(got))
	}
	// A multi-byte character must never be split by the clip.
	multi := strings.Repeat("é", d5CoachingAnswerClip)
	if clipped := d5ClipAnswer(multi); !utf8.ValidString(clipped) {
		t.Fatal("clip split a multi-byte character")
	}
}

func TestD5LostSessionIsNotTakenAsMajor(t *testing.T) {
	c := newD5TestClient(t)
	c.turn++
	body, _ := json.Marshal(chatCompletionRequest{Messages: []chatMessage{
		{Role: "assistant", Content: "At the dashboard stage, how do we keep a device ID as a label?"},
		{Role: "user", Content: "Keep it as a string, don't convert it."},
	}})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(string(body)))
	req = req.WithContext(context.WithValue(req.Context(), sessionIDContextKey, "session-d5"))
	req.Header.Set(turnIDHeader, "turn-lost")
	rec := httptest.NewRecorder()
	handleChat("test-key", c.states, c.turns)(rec, req)
	if got := extractAssistantContent(rec.Body.Bytes()); got != d5SessionLostMessage {
		t.Fatalf("reply = %q", got)
	}
	if st := c.state(); st.StudentMajor != "" || st.ConversationPhase != phaseAwaitingMajor {
		t.Fatalf("lost-session message was taken as a major: %+v", st.StudentMajor)
	}
}

func TestD5TransitionUsesPreDraftedOpening(t *testing.T) {
	c := newD5TestClient(t)
	c.say("Nutrition Science")
	c.say("5")
	// Let the background draft of the Code opening finish.
	time.Sleep(200 * time.Millisecond)
	c.upstream.mu.Lock()
	drafted := c.upstream.drafts
	c.upstream.mu.Unlock()
	if drafted < 2 {
		t.Fatalf("want the Code and Bug openings pre-drafted after the first opening, got %d drafts", drafted)
	}
	answer := "I would compare the order total against each band threshold, highest first, so every order lands in exactly one band."
	var reply string
	var calls int
	for i := 0; i < 5; i++ {
		reply, calls = c.say(fmt.Sprintf("%s Detail %d.", answer, i))
	}
	if !strings.Contains(reply, "Data available") || calls != 0 {
		t.Fatalf("transition made %d live interviewer calls (want 0, using the draft): %q", calls, reply)
	}
}
