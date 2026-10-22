package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const maxUserMessageChars = 6000

type tutor struct {
	llm      llmClient
	store    *progressStore
	sessions *sessionStore
}

func newTutor(llm llmClient, store *progressStore) *tutor {
	return &tutor{llm: llm, store: store, sessions: newSessionStore()}
}

// chatRequest is what the browser sends each turn. Messages is the visible conversation;
// the tutor keeps its own copy, and reads only the latest user message from it.
type chatRequest struct {
	Messages []chatMessage `json:"messages"`
	// Action is set by a UI button, such as "practice:code" or "week:5".
	Action string `json:"action,omitempty"`
	// StudyCode is the browser's remembered study code, used to restore a session lost
	// in a server restart.
	StudyCode string `json:"study_code,omitempty"`
}

type bootstrapRequest struct {
	StudyCode string `json:"study_code,omitempty"`
}

type bootstrapResponse struct {
	Assistant string        `json:"assistant"`
	State     stateResponse `json:"state"`
}

// handleBootstrap starts the conversation for a page load: a welcome back for a known study
// code, otherwise the welcome.
func (t *tutor) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := sessionIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req bootstrapRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	_ = json.NewDecoder(r.Body).Decode(&req) // an empty body is a new student

	sess := t.sessions.getOrCreate(sessionID)
	sess.turnMu.Lock()
	defer sess.turnMu.Unlock()
	t.resetSession(sess)

	reply := welcomeMessage
	if code, ok := normalizeStudyCode(req.StudyCode); ok {
		if p, err := t.resumeStudent(sessionID, sess, code); err == nil {
			reply = welcomeBackMessage(p)
		}
	}
	sess.remember("", reply)
	writeJSON(w, http.StatusOK, bootstrapResponse{Assistant: reply, State: t.state(sess)})
}

// handleReset forgets the session's student, for "use a different study code".
func (t *tutor) handleReset(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := sessionIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	sess := t.sessions.getOrCreate(sessionID)
	sess.turnMu.Lock()
	t.resetSession(sess)
	sess.turnMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (t *tutor) resetSession(sess *tutorSession) {
	sess.Phase = phaseWelcome
	sess.StudyCode = ""
	sess.Week = 0
	sess.Activity = nil
	sess.LastPracticeKind = ""
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
	state := t.state(sess)
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
		state := t.state(sess)
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

// hasPriorConversation reports a request carrying earlier turns.
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
	action    string
	// restored is set when this turn restored a session lost in a restart.
	restored bool
}

func (t *tutor) turn(sessionID string, sess *tutorSession, req chatRequest) string {
	msg := strings.TrimSpace(lastUserMessage(req.Messages))
	if len(msg) > maxUserMessageChars {
		msg = msg[:maxUserMessageChars]
	}
	tc := &turnContext{sessionID: sessionID, sess: sess, msg: msg, action: req.Action}

	// The session is in memory; after a restart the browser still has the conversation
	// and the study code, so the student is put back where their progress says.
	if sess.Phase == phaseWelcome && hasPriorConversation(req.Messages) {
		if code, ok := normalizeStudyCode(req.StudyCode); ok {
			if _, err := t.resumeStudent(sessionID, sess, code); err == nil {
				tc.restored = true
				log.Printf("[tutor] session_restored session=%s", shortID(sessionID))
			}
		}
	}

	var reply string
	switch sess.Phase {
	case phaseWelcome:
		reply = t.welcomeTurn(tc)
	case phaseChooseWeek:
		reply = t.chooseWeekTurn(tc)
	default:
		reply = t.studyTurn(tc)
	}
	return reply
}

var majorPrefix = regexp.MustCompile(`(?i)^(?:(?:hi|hello|hey)[,!. ]+)?(?:my major is|i'?m majoring in|i am majoring in|i major in|i'?m an?|i am an?|i study|i'?m studying|major\s*:|it'?s)\s+`)
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

func (t *tutor) welcomeTurn(tc *turnContext) string {
	sess := tc.sess
	if code, ok := normalizeStudyCode(tc.msg); ok {
		if sess.FailedCodeLookups >= maxFailedCodeTries {
			return t.say(sess, tc.msg, tooManyCodeTriesMessage)
		}
		p, err := t.resumeStudent(tc.sessionID, sess, code)
		if errors.Is(err, errUnknownStudyCode) {
			sess.FailedCodeLookups++
			return t.say(sess, tc.msg, unknownStudyCodeMessage(code))
		}
		if err != nil {
			log.Printf("[tutor] resume_failed session=%s err=%v", shortID(tc.sessionID), err)
			return t.say(sess, tc.msg, "Sorry, I couldn't open that study profile just now. Please try again.")
		}
		return t.say(sess, tc.msg, welcomeBackMessage(p))
	}
	major, ok := cleanMajor(tc.msg)
	if !ok {
		return t.say(sess, tc.msg, askMajorAgainMessage)
	}
	p, err := t.store.create(major)
	if err != nil {
		log.Printf("[tutor] create_profile_failed session=%s err=%v", shortID(tc.sessionID), err)
		return t.say(sess, tc.msg, "Sorry, I couldn't set up your study profile just now. Please try again.")
	}
	log.Printf("[tutor] profile_created session=%s", shortID(tc.sessionID))
	sess.StudyCode = p.StudyCode
	sess.Phase = phaseChooseWeek
	return t.say(sess, tc.msg, studyCodeIssuedMessage(p))
}

// resumeStudent attaches the session to a stored profile and returns to their week.
func (t *tutor) resumeStudent(sessionID string, sess *tutorSession, code string) (studentProfile, error) {
	p, err := t.store.update(code, func(*studentProfile) {})
	if err != nil {
		return studentProfile{}, err
	}
	t.resetSession(sess)
	sess.StudyCode = p.StudyCode
	sess.FailedCodeLookups = 0
	// Problems are prepared on the student's first action (studyTurn), not on a page load.
	if w := weekInfo(p.CurrentWeek); w != nil {
		sess.Phase = phaseStudying
		sess.Week = w.Number
	} else {
		sess.Phase = phaseChooseWeek
	}
	log.Printf("[tutor] student_resumed session=%s week=%d", shortID(sessionID), p.CurrentWeek)
	return p, nil
}

func (t *tutor) chooseWeekTurn(tc *turnContext) string {
	sess := tc.sess
	in, ok := parseAction(tc.action)
	if !ok {
		in = detectIntent(tc.msg, false)
	}
	if in.Kind == intentWeek {
		return t.startWeek(tc, in.Week)
	}
	if sel, ok := matchWeekSelection(strings.TrimPrefix(normalizeUserInput(tc.msg), "week ")); ok {
		return t.startWeek(tc, sel.CurrentWeekNumber)
	}
	if code, ok := normalizeStudyCode(tc.msg); ok && code != sess.StudyCode {
		sess.Phase = phaseWelcome
		return t.welcomeTurn(tc)
	}
	// Someone who was studying a week and asked for the list can carry on with it.
	if sess.Week > 0 && (in.Kind != intentNone || len(strings.Fields(tc.msg)) > 3) {
		sess.Phase = phaseStudying
		return t.studyTurn(tc)
	}
	p, err := t.store.get(sess.StudyCode)
	if err != nil {
		return t.say(sess, tc.msg, tutorUnavailableMessage)
	}
	return t.say(sess, tc.msg, invalidWeekMessage(p))
}

func (t *tutor) startWeek(tc *turnContext, week int) string {
	sess := tc.sess
	w := weekInfo(week)
	p, err := t.store.update(sess.StudyCode, func(p *studentProfile) {
		p.CurrentWeek = week
		p.week(week).LastStudied = time.Now()
	})
	if err != nil {
		log.Printf("[tutor] start_week_failed session=%s err=%v", shortID(tc.sessionID), err)
		return t.say(sess, tc.msg, tutorUnavailableMessage)
	}
	if sess.Week != week {
		sess.clearPrepared()
		sess.LastPracticeKind = ""
	}
	sess.Phase = phaseStudying
	sess.Week = week
	sess.Activity = nil
	t.prepareProblems(tc.sessionID, sess, w, p.Major, p.Weeks[week])
	log.Printf("[tutor] week_started session=%s week=%d", shortID(tc.sessionID), week)
	return t.say(sess, tc.msg, weekPlanMessage(w, p.Weeks[week]))
}

// studyTurn handles a turn while the student is studying a week.
func (t *tutor) studyTurn(tc *turnContext) string {
	sess := tc.sess
	w := weekInfo(sess.Week)
	p, err := t.store.get(sess.StudyCode)
	if err != nil || w == nil {
		log.Printf("[tutor] profile_unavailable session=%s err=%v", shortID(tc.sessionID), err)
		t.resetSession(sess)
		return welcomeMessage
	}
	// Once the turn is answered, have a problem of each kind ready for this week. Doing it
	// after the turn keeps a request for a problem from waiting on a draft started just now.
	defer func() {
		if sess.Phase == phaseStudying && sess.Week == w.Number {
			t.prepareProblems(tc.sessionID, sess, w, p.Major, p.Weeks[w.Number])
		}
	}()
	in, ok := parseAction(tc.action)
	if !ok {
		in = detectIntent(tc.msg, sess.Activity.open())
	}
	if tc.restored && in.Kind == intentNone {
		return t.say(sess, tc.msg, serverRestartedMessage)
	}
	if in.Kind == intentNextLesson && in.MoveOn && sess.Activity.isPractice() {
		in = intent{Kind: intentAnother}
	}
	log.Printf("[tutor] intent session=%s week=%d intent=%q practice=%s activity=%s",
		shortID(tc.sessionID), w.Number, in.Kind, in.Practice, activityName(sess.Activity))

	switch in.Kind {
	case intentWeek:
		return t.startWeek(tc, in.Week)
	case intentWeekMenu:
		sess.Phase = phaseChooseWeek
		return t.say(sess, tc.msg, chooseWeekMessage(p))
	case intentPlan:
		return t.say(sess, tc.msg, weekPlanMessage(w, p.Weeks[w.Number]))
	case intentGuide:
		return t.say(sess, tc.msg, interviewGuideMessage(w))
	case intentProgress:
		return t.say(sess, tc.msg, progressMessage(p, w.Number))
	case intentStudyCode:
		return t.say(sess, tc.msg, studyCodeMessage(p))
	case intentHelp:
		return t.say(sess, tc.msg, helpMessage(w))
	case intentLesson, intentNextLesson:
		return t.teachLesson(tc, w, p, t.lessonToTeach(tc, w, p, in), in.Lesson > 0)
	case intentPractice:
		kind := in.Practice
		if kind == "" && in.Difficulty != difficultyStandard && sess.LastPracticeKind != "" {
			kind = sess.LastPracticeKind
		}
		if kind == "" {
			kind = suggestedPractice(w, p.Weeks[w.Number])
		}
		return t.startPractice(tc, w, p, kind, in.Difficulty)
	case intentAnother:
		kind := sess.LastPracticeKind
		if kind == "" {
			kind = suggestedPractice(w, p.Weeks[w.Number])
		}
		return t.startPractice(tc, w, p, kind, in.Difficulty)
	case intentHint:
		if !sess.Activity.open() {
			return t.say(sess, tc.msg, noProblemOpenMessage(w))
		}
		return t.giveHint(tc, w, p)
	case intentSolution:
		if !sess.Activity.isPractice() {
			return t.say(sess, tc.msg, noProblemOpenMessage(w))
		}
		return t.showSolution(tc, w, p)
	}

	switch a := sess.Activity; {
	case a.isPractice():
		return t.practiceReply(tc, w, p)
	case a != nil && a.Kind == kindLesson:
		return t.lessonReply(tc, w, p)
	}
	return t.chat(tc, w, p)
}

// say records a server-written reply in the conversation history and returns it.
func (t *tutor) say(sess *tutorSession, user, reply string) string {
	sess.remember(user, reply)
	return reply
}

// lessonToTeach picks the lesson for "teach me" or "next lesson": the one asked for, else
// the next one not yet done after the current lesson (which moving on marks done).
func (t *tutor) lessonToTeach(tc *turnContext, w *weekContent, p studentProfile, in intent) int {
	if in.Lesson > 0 {
		return in.Lesson - 1
	}
	a := tc.sess.Activity
	if a == nil || a.Kind != kindLesson {
		return nextLessonIndex(w, p.Weeks[w.Number])
	}
	t.markLessonDone(tc, w, a.Lesson)
	updated, err := t.store.get(tc.sess.StudyCode)
	if err != nil {
		return a.Lesson + 1
	}
	wp := updated.Weeks[w.Number]
	for j := a.Lesson + 1; j < len(w.Lessons); j++ {
		if wp == nil || !wp.lessonDone(j) {
			return j
		}
	}
	return nextLessonIndex(w, wp)
}

// teachLesson teaches lesson i. asked is set when the student named the lesson, so a
// lesson already done is taught again rather than skipped.
func (t *tutor) teachLesson(tc *turnContext, w *weekContent, p studentProfile, i int, asked bool) string {
	sess := tc.sess
	wp := p.Weeks[w.Number]
	if asked && (i < 0 || i >= len(w.Lessons)) {
		return t.say(sess, tc.msg, fmt.Sprintf("This week has %d lessons. Ask for one by number, from `lesson 1` to `lesson %d`.", len(w.Lessons), len(w.Lessons)))
	}
	if i < 0 || i >= len(w.Lessons) {
		sess.Activity = nil
		return t.say(sess, tc.msg, allLessonsDoneMessage(w, wp))
	}
	var done []string
	for j := range w.Lessons {
		if j != i && wp != nil && wp.lessonDone(j) {
			done = append(done, w.Lessons[j].Title)
		}
	}
	base := tutorBasePrompt(w, p.Major)
	res, err := t.llm.call(context.Background(), callLesson, lessonMessages(base, w, i, p.Major, done))
	logCall(tc.sessionID, callLesson, res, err)
	content := strings.TrimSpace(res.Content)
	if err != nil || content == "" {
		content = lessonOutline(w, i)
	}
	sess.Activity = &activity{Kind: kindLesson, Lesson: i, Title: w.Lessons[i].Title}
	sess.Activity.addMessage("assistant", content)
	footer := fmt.Sprintf("*Lesson %d of %d. Answer the quick check, ask a question, or choose **Next lesson**.*", i+1, len(w.Lessons))
	if i == len(w.Lessons)-1 {
		footer = fmt.Sprintf("*Lesson %d of %d, the last one this week. Answer the quick check or ask a question, then try some practice.*", i+1, len(w.Lessons))
	}
	return t.say(sess, tc.msg, content+"\n\n---\n"+footer)
}

func (t *tutor) markLessonDone(tc *turnContext, w *weekContent, i int) {
	if _, err := t.store.update(tc.sess.StudyCode, func(p *studentProfile) {
		wp := p.week(w.Number)
		wp.markLessonDone(i)
		wp.LastStudied = time.Now()
	}); err != nil {
		log.Printf("[tutor] save_failed session=%s err=%v", shortID(tc.sessionID), err)
	}
}

func (t *tutor) lessonReply(tc *turnContext, w *weekContent, p studentProfile) string {
	a := tc.sess.Activity
	t.markLessonDone(tc, w, a.Lesson)
	a.addMessage("user", tc.msg)
	res, err := t.llm.call(context.Background(), callChat, lessonReplyMessages(tutorBasePrompt(w, p.Major), a))
	logCall(tc.sessionID, callChat, res, err)
	reply := strings.TrimSpace(res.Content)
	if err != nil || reply == "" {
		a.Messages = a.Messages[:len(a.Messages)-1]
		return t.say(tc.sess, tc.msg, tutorUnavailableMessage)
	}
	a.addMessage("assistant", reply)
	return t.say(tc.sess, tc.msg, reply)
}

func (t *tutor) chat(tc *turnContext, w *weekContent, p studentProfile) string {
	sess := tc.sess
	var done *activity
	if sess.Activity.isPractice() {
		done = sess.Activity
	}
	history := append(append([]chatMessage(nil), sess.History...), chatMessage{Role: "user", Content: tc.msg})
	res, err := t.llm.call(context.Background(), callChat, chatMessages(tutorBasePrompt(w, p.Major), history, done))
	logCall(tc.sessionID, callChat, res, err)
	reply := strings.TrimSpace(res.Content)
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

func (t *tutor) startPractice(tc *turnContext, w *weekContent, p studentProfile, kind, difficulty string) string {
	sess := tc.sess
	if !w.hasPractice(kind) {
		return t.say(sess, tc.msg, noWeek1PracticeMessage(kind))
	}
	wp := p.Weeks[w.Number]
	var problem generatedProblem
	ok := false
	if difficulty == difficultyStandard {
		problem, ok = sess.takePrepared(kind, w.Number, p.Major)
	}
	source := "prepared"
	if !ok {
		source = "live"
		var avoid []string
		if wp != nil {
			avoid = wp.recentTitles(kind, 6)
		}
		var err error
		problem, err = t.generateProblem(context.Background(), tc.sessionID, callProblem, w, kind, p.Major, difficulty, avoid)
		if err != nil {
			log.Printf("[tutor] problem_failed session=%s kind=%s err=%v", shortID(tc.sessionID), kind, err)
			return t.say(sess, tc.msg, "Sorry, I couldn't put a practice problem together just now. Please try again in a moment.")
		}
	}
	log.Printf("[tutor] problem_served session=%s week=%d kind=%s difficulty=%q source=%s title=%q",
		shortID(tc.sessionID), w.Number, kind, difficulty, source, problem.Title)

	rec := practiceRecord{ID: newRecordID(), Kind: kind, Title: problem.Title, StartedAt: time.Now()}
	updated, err := t.store.update(sess.StudyCode, func(p *studentProfile) {
		wp := p.week(w.Number)
		wp.addRecord(rec)
		wp.LastStudied = time.Now()
	})
	if err != nil {
		log.Printf("[tutor] save_failed session=%s err=%v", shortID(tc.sessionID), err)
	}
	sess.Activity = &activity{
		Kind:      kind,
		Title:     problem.Title,
		Problem:   problem.Problem,
		Reference: problem.Reference,
		Stage:     firstStage(kind),
		RecordID:  rec.ID,
	}
	sess.Activity.addMessage("assistant", problem.Problem)
	sess.LastPracticeKind = kind
	// Have the next one of this kind ready.
	t.prepareProblems(tc.sessionID, sess, w, p.Major, updated.Weeks[w.Number], kind)

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

func newRecordID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return time.Now().Format("150405.000000")
	}
	return hex.EncodeToString(b)
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

func (t *tutor) practiceReply(tc *turnContext, w *weekContent, p studentProfile) string {
	sess := tc.sess
	a := sess.Activity
	codePasted := w.Number > 1 && looksLikeCode(tc.msg)
	a.addMessage("user", tc.msg)
	spec := callFeedback
	res, err := t.llm.call(context.Background(), spec, feedbackMessages(tutorBasePrompt(w, p.Major), w, a, codePasted))
	logCall(tc.sessionID, spec, res, err)
	reply, level := splitLevel(res.Content)
	if err != nil || reply == "" {
		a.Messages = a.Messages[:len(a.Messages)-1]
		return t.say(sess, tc.msg, tutorUnavailableMessage)
	}
	a.addMessage("assistant", reply)

	wasOpen := a.open()
	if wasOpen && (level != "" || (a.Stage == stageCode && codePasted)) {
		a.Stage = nextStage(a, codePasted)
	}
	log.Printf("[tutor] feedback session=%s kind=%s level=%q stage=%s code=%v", shortID(tc.sessionID), a.Kind, level, a.Stage, codePasted)
	if wasOpen {
		if _, err := t.store.update(sess.StudyCode, func(p *studentProfile) {
			wp := p.week(w.Number)
			wp.LastStudied = time.Now()
			// A question or a request for help is not an attempt; only rated answers count.
			if rec := wp.record(a.RecordID); rec != nil && level != "" {
				rec.Attempts++
				rec.Level = level
			}
		}); err != nil {
			log.Printf("[tutor] save_failed session=%s err=%v", shortID(tc.sessionID), err)
		}
	}
	if wasOpen && a.Stage == stageDone {
		updated, err := t.store.get(sess.StudyCode)
		if err == nil {
			reply += "\n\n---\n*" + practiceLabel(w.Number, a.Kind) + " finished. 🎯 Readiness for " + strings.SplitN(w.Title, " - ", 2)[0] + ": **" + weekReadiness(w, updated.Weeks[w.Number]).Label + "**.*"
		}
	}
	return t.say(sess, tc.msg, reply)
}

func (t *tutor) giveHint(tc *turnContext, w *weekContent, p studentProfile) string {
	sess := tc.sess
	a := sess.Activity
	res, err := t.llm.call(context.Background(), callHint, hintMessages(tutorBasePrompt(w, p.Major), w, a))
	logCall(tc.sessionID, callHint, res, err)
	reply := strings.TrimSpace(res.Content)
	if err != nil || reply == "" {
		return t.say(sess, tc.msg, tutorUnavailableMessage)
	}
	a.Hints++
	a.addMessage("user", tc.msg)
	a.addMessage("assistant", reply)
	if _, err := t.store.update(sess.StudyCode, func(p *studentProfile) {
		if rec := p.week(w.Number).record(a.RecordID); rec != nil {
			rec.HintsUsed++
		}
	}); err != nil {
		log.Printf("[tutor] save_failed session=%s err=%v", shortID(tc.sessionID), err)
	}
	return t.say(sess, tc.msg, "💡 "+reply)
}

func (t *tutor) showSolution(tc *turnContext, w *weekContent, p studentProfile) string {
	sess := tc.sess
	a := sess.Activity
	res, err := t.llm.call(context.Background(), callSolution, solutionMessages(tutorBasePrompt(w, p.Major), w, a))
	logCall(tc.sessionID, callSolution, res, err)
	reply := strings.TrimSpace(res.Content)
	if err != nil || reply == "" {
		return t.say(sess, tc.msg, tutorUnavailableMessage)
	}
	a.Stage = stageDone
	a.addMessage("user", tc.msg)
	a.addMessage("assistant", reply)
	if _, err := t.store.update(sess.StudyCode, func(p *studentProfile) {
		if rec := p.week(w.Number).record(a.RecordID); rec != nil {
			rec.SolutionShown = true
		}
	}); err != nil {
		log.Printf("[tutor] save_failed session=%s err=%v", shortID(tc.sessionID), err)
	}
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
