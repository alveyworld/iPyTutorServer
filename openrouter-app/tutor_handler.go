package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const maxUserMessageChars = 6000

// tutor answers students' turns. It keeps no record of students between visits: each
// browser session starts fresh, and the student can study any week and lesson at any time.
type tutor struct {
	llm      llmClient
	sessions *sessionStore
}

func newTutor(llm llmClient) *tutor {
	return &tutor{llm: llm, sessions: newSessionStore()}
}

// chatRequest is what the browser sends each turn. Messages is the visible conversation;
// the tutor keeps its own copy, and reads only the latest user message from it.
type chatRequest struct {
	Messages []chatMessage `json:"messages"`
	// Action is set by a UI button, such as "practice:code", "week:5" or "lesson:3".
	Action string `json:"action,omitempty"`
}

type bootstrapResponse struct {
	Assistant string        `json:"assistant"`
	State     stateResponse `json:"state"`
}

// handleBootstrap starts a fresh conversation for a page load.
func (t *tutor) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := sessionIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	sess := t.sessions.getOrCreate(sessionID)
	sess.turnMu.Lock()
	defer sess.turnMu.Unlock()
	resetSession(sess)
	sess.started = true
	reply := welcomeMessage()
	sess.remember("", reply)
	writeJSON(w, http.StatusOK, bootstrapResponse{Assistant: reply, State: sess.state()})
}

func resetSession(sess *tutorSession) {
	sess.Phase = phaseWelcome
	sess.Major = ""
	sess.Week = 0
	sess.Activity = nil
	sess.LastLesson = -1
	sess.LastPracticeKind = ""
	sess.recentTitles = map[string][]string{}
	sess.History = nil
	sess.clearPrepared()
}

func (t *tutor) handleState(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := sessionIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	sess, ok := t.sessions.get(sessionID)
	if !ok {
		sess = newTutorSession()
	}
	sess.turnMu.Lock()
	state := sess.state()
	sess.turnMu.Unlock()
	writeJSON(w, http.StatusOK, state)
}

// handleChat runs one turn. A turn id (X-Turn-Id) makes retries safe: a repeated id gets
// the first reply instead of running the turn again.
func (t *tutor) handleChat(turns *turnStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID, ok := sessionIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		var req chatRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		turnID := strings.TrimSpace(r.Header.Get(turnIDHeader))
		var rec *chatTurnRecord
		if turnID != "" {
			acquired := turns.acquire(sessionID, turnID)
			rec = acquired.record
			switch acquired.role {
			case turnRoleFollower:
				if rec.wait.wait(r.Context()) {
					writeTurnResponse(w, rec)
				}
				return
			case turnRoleReplayCompleted, turnRoleReplayFailed:
				writeTurnResponse(w, rec)
				return
			}
		}

		sess := t.sessions.getOrCreate(sessionID)
		sess.turnMu.Lock()
		started := time.Now()
		reply := t.turn(sessionID, sess, req)
		state := sess.state()
		sess.turnMu.Unlock()
		log.Printf("[tutor] turn session=%s turn_id=%s phase=%s week=%d activity=%s ms=%d",
			shortID(sessionID), shortID(turnID), sess.Phase, sess.Week, activityName(sess.Activity), time.Since(started).Milliseconds())

		respBody, err := json.Marshal(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": reply}}},
			"state":   state,
		})
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			if rec != nil {
				turns.failTurn(sessionID, turnID, reply, reply, nil, http.StatusInternalServerError)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBody)
		if rec != nil {
			turns.completeTurn(sessionID, turnID, reply, reply, respBody, http.StatusOK)
		}
	}
}

func activityName(a *activity) string {
	if a == nil {
		return "none"
	}
	if a.isPractice() {
		return a.Kind + "/" + a.Stage
	}
	return a.Kind
}

func lastUserMessage(messages []chatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

// hasPriorConversation reports a request carrying earlier turns. The page's own "start"
// message counts, so a first real message after it makes two.
func hasPriorConversation(msgs []chatMessage) bool {
	users := 0
	for _, m := range msgs {
		if m.Role == "user" {
			users++
		}
	}
	return users > 1
}

// turnContext carries one turn's inputs.
type turnContext struct {
	sessionID string
	sess      *tutorSession
	msg       string
	in        intent
	// lost is set when the browser has a conversation the server no longer knows, after a
	// restart.
	lost bool
}

func (t *tutor) turn(sessionID string, sess *tutorSession, req chatRequest) string {
	msg := strings.TrimSpace(lastUserMessage(req.Messages))
	if len(msg) > maxUserMessageChars {
		msg = msg[:maxUserMessageChars]
	}
	tc := &turnContext{sessionID: sessionID, sess: sess, msg: msg}
	in, ok := parseAction(req.Action)
	if !ok {
		in = detectIntent(msg, sess.Activity.open())
	}
	tc.in = in
	tc.lost = !sess.started && hasPriorConversation(req.Messages)
	sess.started = true
	log.Printf("[tutor] intent session=%s phase=%s week=%d intent=%q practice=%s activity=%s",
		shortID(sessionID), sess.Phase, sess.Week, in.Kind, in.Practice, activityName(sess.Activity))

	// Choosing a week, a lesson or a major works from anywhere.
	if reply, ok := t.navigate(tc); ok {
		return reply
	}
	switch sess.Phase {
	case phaseWelcome:
		return t.welcomeTurn(tc)
	case phaseChooseWeek:
		return t.chooseWeekTurn(tc)
	}
	return t.studyTurn(tc)
}

// navigate handles the commands that work in every phase: studying a week (optionally at
// a lesson), a lesson of the current week, the week list, and setting the major.
func (t *tutor) navigate(tc *turnContext) (string, bool) {
	sess, in := tc.sess, tc.in
	switch in.Kind {
	case intentWeek:
		// "teach me about elif" during week 5 is a question about this week, not a switch.
		if in.Topic && in.Week == sess.Week && sess.Phase == phaseStudying {
			return "", false
		}
		lesson := in.Lesson - 1
		if in.Teach {
			lesson = 0
		}
		if lesson < 0 {
			return t.startWeek(tc, in.Week), true
		}
		changed := sess.Week != in.Week
		t.switchWeek(tc, in.Week)
		reply := t.teachLesson(tc, weekInfo(in.Week), lesson, true)
		if changed {
			reply = "*📚 Now studying **" + weekInfo(in.Week).Title + "**.*\n\n" + reply
		}
		return reply, true
	case intentLesson:
		if sess.Week == 0 && in.Lesson > 0 {
			return t.say(sess, tc.msg, pickWeekFirstMessage("Lessons belong to a week, so pick the week first. For example, type `week 5 lesson 2`.")), true
		}
	case intentWeekMenu:
		sess.Phase = phaseChooseWeek
		return t.say(sess, tc.msg, chooseWeekMessage()), true
	case intentSetMajor:
		major, ok := cleanMajor(tc.msg)
		if !ok {
			return "", false
		}
		return t.setMajor(tc, major), true
	}
	return "", false
}

var majorPrefix = regexp.MustCompile(`(?i)^(?:(?:hi|hello|hey)[,!. ]+)?(?:my major is|i'?m majoring in|i am majoring in|i major in|i'?m an?|i am an?|i study|i'?m studying|major\s*:?|set my major to|change my major to|it'?s)\s+`)
var majorSuffix = regexp.MustCompile(`(?i)\s+(?:major|student)$`)

// cleanMajor extracts the major from a reply such as "I'm a biology major".
func cleanMajor(msg string) (string, bool) {
	s := strings.Trim(strings.TrimSpace(msg), ".!?\"'")
	s = majorPrefix.ReplaceAllString(s, "")
	s = majorSuffix.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 60 || len(strings.Fields(s)) > 6 || strings.ContainsAny(s, "`{}[]<>") || isGreetingMessage(strings.Trim(s, "!.,")) {
		return "", false
	}
	return s, true
}

// setMajor changes the field examples and problems are set in. Problems drafted for the
// old field are dropped.
func (t *tutor) setMajor(tc *turnContext, major string) string {
	sess := tc.sess
	sess.Major = major
	sess.clearPrepared()
	if sess.Week == 0 {
		sess.Phase = phaseChooseWeek
	}
	log.Printf("[tutor] major_set session=%s", shortID(tc.sessionID))
	return t.say(sess, tc.msg, majorSetMessage(major, sess.Week))
}

// weekFromMessage reads a bare week choice such as "5", "week 5" or "lists".
func weekFromMessage(msg string) (int, bool) {
	sel, ok := matchWeekSelection(strings.TrimPrefix(normalizeUserInput(strings.Trim(msg, ".!?")), "week "))
	return sel.CurrentWeekNumber, ok
}

// studyCommand reports commands that need a week to work on.
func studyCommand(k intentKind) bool {
	switch k {
	case intentLesson, intentNextLesson, intentPractice, intentAnother, intentHint, intentSolution, intentGuide, intentOverview:
		return true
	}
	return false
}

func (t *tutor) welcomeTurn(tc *turnContext) string {
	sess := tc.sess
	if week, ok := weekFromMessage(tc.msg); ok {
		return t.startWeek(tc, week)
	}
	if tc.lost {
		// The server restarted and lost this conversation; carry on from a week choice.
		sess.Phase = phaseChooseWeek
		return t.say(sess, tc.msg, serverRestartedMessage+weekListMarkdown())
	}
	switch {
	case tc.in.Kind == intentHelp:
		return t.say(sess, tc.msg, helpMessage(nil))
	case studyCommand(tc.in.Kind):
		sess.Phase = phaseChooseWeek
		return t.say(sess, tc.msg, pickWeekFirstMessage(""))
	}
	if major, ok := cleanMajor(tc.msg); ok {
		return t.setMajor(tc, major)
	}
	return t.say(sess, tc.msg, askMajorOrWeekMessage)
}

func (t *tutor) chooseWeekTurn(tc *turnContext) string {
	sess := tc.sess
	if week, ok := weekFromMessage(tc.msg); ok {
		return t.startWeek(tc, week)
	}
	// Someone who was studying a week and asked for the list can carry on with it.
	if sess.Week > 0 && (tc.in.Kind != intentNone || len(strings.Fields(tc.msg)) > 3) {
		sess.Phase = phaseStudying
		return t.studyTurn(tc)
	}
	if tc.in.Kind == intentHelp {
		return t.say(sess, tc.msg, helpMessage(nil))
	}
	if sess.Major == "" && tc.in.Kind == intentNone {
		if major, ok := cleanMajor(tc.msg); ok {
			return t.setMajor(tc, major)
		}
	}
	return t.say(sess, tc.msg, invalidWeekMessage())
}

// startWeek switches to a week and shows its overview.
func (t *tutor) startWeek(tc *turnContext, week int) string {
	t.switchWeek(tc, week)
	return t.say(tc.sess, tc.msg, weekOverviewMessage(weekInfo(week), tc.sess.Major))
}

// switchWeek makes week the one being studied and starts preparing its problems.
func (t *tutor) switchWeek(tc *turnContext, week int) {
	sess := tc.sess
	w := weekInfo(week)
	if sess.Week != week {
		sess.clearPrepared()
		sess.LastPracticeKind = ""
		sess.LastLesson = -1
	}
	sess.Phase = phaseStudying
	sess.Week = week
	sess.Activity = nil
	t.prepareProblems(tc.sessionID, sess, w, sess.Major)
	log.Printf("[tutor] week_started session=%s week=%d", shortID(tc.sessionID), week)
}

// studyTurn handles a turn while the student is studying a week.
func (t *tutor) studyTurn(tc *turnContext) string {
	sess := tc.sess
	w := weekInfo(sess.Week)
	if w == nil {
		resetSession(sess)
		return t.say(sess, tc.msg, welcomeMessage())
	}
	// Once the turn is answered, have a problem of each kind ready for this week. Doing it
	// after the turn keeps a request for a problem from waiting on a draft started just now.
	defer func() {
		if sess.Phase == phaseStudying && sess.Week == w.Number {
			t.prepareProblems(tc.sessionID, sess, w, sess.Major)
		}
	}()
	in := tc.in
	if in.Kind == intentNextLesson && in.MoveOn && sess.Activity.isPractice() {
		in = intent{Kind: intentAnother}
	}

	switch in.Kind {
	case intentOverview:
		return t.say(sess, tc.msg, weekOverviewMessage(w, sess.Major))
	case intentGuide:
		return t.say(sess, tc.msg, interviewGuideMessage(w))
	case intentHelp:
		return t.say(sess, tc.msg, helpMessage(w))
	case intentLesson, intentNextLesson:
		if in.Lesson > 0 {
			return t.teachLesson(tc, w, in.Lesson-1, true)
		}
		return t.teachLesson(tc, w, sess.LastLesson+1, false)
	case intentPractice:
		kind := in.Practice
		if kind == "" {
			kind = sess.LastPracticeKind
		}
		if kind == "" {
			kind = kindConcept
		}
		return t.startPractice(tc, w, kind, in.Difficulty)
	case intentAnother:
		kind := sess.LastPracticeKind
		if kind == "" {
			kind = kindConcept
		}
		return t.startPractice(tc, w, kind, in.Difficulty)
	case intentHint:
		if !sess.Activity.open() {
			return t.say(sess, tc.msg, noProblemOpenMessage(w))
		}
		return t.giveHint(tc, w)
	case intentSolution:
		if !sess.Activity.isPractice() {
			return t.say(sess, tc.msg, noProblemOpenMessage(w))
		}
		return t.showSolution(tc, w)
	}

	switch a := sess.Activity; {
	case a.isPractice():
		return t.practiceReply(tc, w)
	case a != nil && a.Kind == kindLesson:
		return t.lessonReply(tc, w)
	}
	return t.chat(tc, w)
}

// say records a server-written reply in the conversation history and returns it.
func (t *tutor) say(sess *tutorSession, user, reply string) string {
	sess.remember(user, reply)
	return reply
}

// teachLesson teaches lesson i of the week. asked is set when the student named the
// lesson; otherwise i is the one after the last lesson taught.
func (t *tutor) teachLesson(tc *turnContext, w *weekContent, i int, asked bool) string {
	sess := tc.sess
	if i < 0 || i >= len(w.Lessons) {
		if asked {
			return t.say(sess, tc.msg, fmt.Sprintf("%s has %d lessons. Ask for one by number, from `lesson 1` to `lesson %d`.", w.Title, len(w.Lessons), len(w.Lessons)))
		}
		sess.Activity = nil
		return t.say(sess, tc.msg, lastLessonMessage(w))
	}
	base := tutorBasePrompt(w, sess.Major)
	res, err := t.llm.call(context.Background(), callLesson, lessonMessages(base, w, i, sess.Major))
	logCall(tc.sessionID, callLesson, res, err)
	content := cleanReply(tc.sessionID, callLesson, res.Content)
	if err != nil || content == "" {
		content = lessonOutline(w, i)
	}
	sess.Activity = &activity{Kind: kindLesson, Lesson: i, Title: w.Lessons[i].Title}
	sess.Activity.addMessage("assistant", content)
	sess.LastLesson = i
	footer := fmt.Sprintf("*Lesson %d of %d. Answer the quick check, ask a question, or choose **Next lesson**.*", i+1, len(w.Lessons))
	if i == len(w.Lessons)-1 {
		footer = fmt.Sprintf("*Lesson %d of %d, the last one this week. Answer the quick check or ask a question, then try some practice.*", i+1, len(w.Lessons))
	}
	return t.say(sess, tc.msg, content+"\n\n---\n"+footer)
}

func (t *tutor) lessonReply(tc *turnContext, w *weekContent) string {
	a := tc.sess.Activity
	a.addMessage("user", tc.msg)
	res, err := t.llm.call(context.Background(), callChat, lessonReplyMessages(tutorBasePrompt(w, tc.sess.Major), a))
	logCall(tc.sessionID, callChat, res, err)
	reply := cleanReply(tc.sessionID, callChat, res.Content)
	if err != nil || reply == "" {
		a.Messages = a.Messages[:len(a.Messages)-1]
		return t.say(tc.sess, tc.msg, tutorUnavailableMessage)
	}
	a.addMessage("assistant", reply)
	return t.say(tc.sess, tc.msg, reply)
}

func (t *tutor) chat(tc *turnContext, w *weekContent) string {
	sess := tc.sess
	var done *activity
	if sess.Activity.isPractice() {
		done = sess.Activity
	}
	history := append(append([]chatMessage(nil), sess.History...), chatMessage{Role: "user", Content: tc.msg})
	res, err := t.llm.call(context.Background(), callChat, chatMessages(tutorBasePrompt(w, sess.Major), history, done))
	logCall(tc.sessionID, callChat, res, err)
	reply := cleanReply(tc.sessionID, callChat, res.Content)
	if err != nil || reply == "" {
		return t.say(sess, tc.msg, tutorUnavailableMessage)
	}
	return t.say(sess, tc.msg, reply)
}

// firstStage is where a practice problem of a kind starts.
func firstStage(kind string) string {
	switch kind {
	case kindCode:
		return stageDecompose
	case kindBug:
		return stageStrategy
	}
	return stageAnswer
}

func (t *tutor) startPractice(tc *turnContext, w *weekContent, kind, difficulty string) string {
	sess := tc.sess
	if !w.hasPractice(kind) {
		return t.say(sess, tc.msg, noWeek1PracticeMessage(kind))
	}
	var problem generatedProblem
	ok := false
	if difficulty == difficultyStandard {
		problem, ok = sess.takePrepared(kind, w.Number, sess.Major)
	}
	source := "prepared"
	if !ok {
		source = "live"
		var err error
		problem, err = t.generateProblem(context.Background(), tc.sessionID, callProblem, w, kind, sess.Major, difficulty, sess.recentProblemTitles(w.Number, kind))
		if err != nil {
			log.Printf("[tutor] problem_failed session=%s kind=%s err=%v", shortID(tc.sessionID), kind, err)
			return t.say(sess, tc.msg, "Sorry, I couldn't put a practice problem together just now. Please try again in a moment.")
		}
	}
	log.Printf("[tutor] problem_served session=%s week=%d kind=%s difficulty=%q source=%s title=%q",
		shortID(tc.sessionID), w.Number, kind, difficulty, source, problem.Title)

	sess.addRecentTitle(w.Number, kind, problem.Title)
	sess.Activity = &activity{
		Kind:      kind,
		Title:     problem.Title,
		Problem:   problem.Problem,
		Reference: problem.Reference,
		Stage:     firstStage(kind),
	}
	sess.Activity.addMessage("assistant", problem.Problem)
	sess.LastPracticeKind = kind
	// Have the next one of this kind ready.
	t.prepareProblems(tc.sessionID, sess, w, sess.Major, kind)

	heading := "### " + practiceEmoji(kind) + " " + practiceLabel(w.Number, kind) + ": " + problem.Title
	switch difficulty {
	case difficultyEasier:
		heading += " (warm-up)"
	case difficultyHarder:
		heading += " (challenge)"
	}
	footer := "*Ask for a **hint** any time, or the **solution** if you get stuck.*"
	return t.say(sess, tc.msg, heading+"\n\n"+problem.Problem+"\n\n---\n"+footer)
}

// nextStage moves a practice problem on after a rated answer.
func nextStage(a *activity, codePasted bool) string {
	switch a.Stage {
	case stageAnswer:
		return stageFollowUp
	case stageDecompose:
		if codePasted {
			return stageExplain
		}
		return stageCode
	case stageCode:
		if codePasted {
			return stageExplain
		}
		return stageCode
	case stageStrategy:
		return stageFix
	}
	return stageDone
}

func (t *tutor) practiceReply(tc *turnContext, w *weekContent) string {
	sess := tc.sess
	a := sess.Activity
	codePasted := w.Number > 1 && looksLikeCode(tc.msg)
	a.addMessage("user", tc.msg)
	res, err := t.llm.call(context.Background(), callFeedback, feedbackMessages(tutorBasePrompt(w, sess.Major), w, a, codePasted))
	logCall(tc.sessionID, callFeedback, res, err)
	reply, level := splitLevel(res.Content)
	reply = cleanReply(tc.sessionID, callFeedback, reply)
	if err != nil || reply == "" {
		a.Messages = a.Messages[:len(a.Messages)-1]
		return t.say(sess, tc.msg, tutorUnavailableMessage)
	}
	a.addMessage("assistant", reply)

	// A question or a request for help isn't rated, and leaves the problem where it was.
	wasOpen := a.open()
	if wasOpen && (level != "" || (a.Stage == stageCode && codePasted)) {
		a.Stage = nextStage(a, codePasted)
	}
	log.Printf("[tutor] feedback session=%s kind=%s level=%q stage=%s code=%v", shortID(tc.sessionID), a.Kind, level, a.Stage, codePasted)
	if wasOpen && a.Stage == stageDone {
		reply += "\n\n---\n*✅ " + practiceLabel(w.Number, a.Kind) + " finished. Try **Another one**, a different part of the interview, or any lesson.*"
	}
	return t.say(sess, tc.msg, reply)
}

func (t *tutor) giveHint(tc *turnContext, w *weekContent) string {
	sess := tc.sess
	a := sess.Activity
	// The student's request goes last, so the model answers it (see endWithUserTurn).
	a.addMessage("user", tc.msg)
	res, err := t.llm.call(context.Background(), callHint, hintMessages(tutorBasePrompt(w, sess.Major), w, a))
	logCall(tc.sessionID, callHint, res, err)
	reply := cleanReply(tc.sessionID, callHint, res.Content)
	if err != nil || reply == "" {
		a.Messages = a.Messages[:len(a.Messages)-1]
		return t.say(sess, tc.msg, tutorUnavailableMessage)
	}
	a.Hints++
	a.addMessage("assistant", reply)
	return t.say(sess, tc.msg, "💡 "+reply)
}

func (t *tutor) showSolution(tc *turnContext, w *weekContent) string {
	sess := tc.sess
	a := sess.Activity
	a.addMessage("user", tc.msg)
	res, err := t.llm.call(context.Background(), callSolution, solutionMessages(tutorBasePrompt(w, sess.Major), w, a))
	logCall(tc.sessionID, callSolution, res, err)
	reply := cleanReply(tc.sessionID, callSolution, res.Content)
	if err != nil || reply == "" {
		a.Messages = a.Messages[:len(a.Messages)-1]
		return t.say(sess, tc.msg, tutorUnavailableMessage)
	}
	a.Stage = stageDone
	a.addMessage("assistant", reply)
	return t.say(sess, tc.msg, reply)
}

func logCall(sessionID string, spec callSpec, res llmResult, err error) {
	log.Printf("[tutor] llm call=%s session=%s ms=%d ttft_ms=%d in_tokens=%d out_tokens=%d reasoning_tokens=%d cached_tokens=%d provider=%s attempts=%d finish=%s err=%v",
		spec.Name, shortID(sessionID), res.Elapsed.Milliseconds(), res.TTFT.Milliseconds(), res.Usage.PromptTokens, res.Usage.CompletionTokens,
		res.Usage.CompletionTokensDetails.ReasoningTokens, res.Usage.PromptTokensDetails.CachedTokens, res.Provider, res.Attempts, res.Finish, err)
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
