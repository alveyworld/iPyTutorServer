package main

import (
	"sync"
	"time"
)

// A tutorSession is one browser login's conversation with the tutor. It lives in memory
// and is lost on a restart; the student's progress is in the progressStore, and the
// browser sends the study code with each request so the session can be restored.

type tutorPhase string

const (
	// phaseWelcome waits for a major (new student) or a study code (returning student).
	phaseWelcome    tutorPhase = "welcome"
	phaseChooseWeek tutorPhase = "choose_week"
	phaseStudying   tutorPhase = "studying"
)

// Practice stages. A code problem goes decompose → code → explain, as in the interview's
// code part; a concept question gets one follow-up; a bug hunt asks for the debugging
// approach, then for the exact bug and its fix.
const (
	stageAnswer    = "answer"
	stageFollowUp  = "followup"
	stageDecompose = "decompose"
	stageCode      = "code"
	stageExplain   = "explain"
	stageStrategy  = "strategy"
	stageFix       = "fix"
	stageDone      = "done"
)

const (
	maxHistoryMessages  = 16
	maxActivityMessages = 24
	maxFailedCodeTries  = 5
)

// activity is what the student is working on: a lesson or a practice problem.
type activity struct {
	Kind string
	// Lesson is the lesson index for a lesson.
	Lesson int
	Title  string
	// Problem is what the student sees; Reference is the tutor's hidden answer key.
	Problem   string
	Reference string
	Stage     string
	Hints     int
	RecordID  string
	// Messages is the activity's own conversation, starting with the lesson or problem.
	Messages []chatMessage
}

func (a *activity) isPractice() bool {
	return a != nil && a.Kind != kindLesson
}

func (a *activity) open() bool {
	return a.isPractice() && a.Stage != stageDone
}

func (a *activity) addMessage(role, content string) {
	a.Messages = append(a.Messages, chatMessage{Role: role, Content: content})
	if over := len(a.Messages) - maxActivityMessages; over > 0 {
		// Keep the lesson or problem itself; drop the oldest exchanges after it.
		a.Messages = append(a.Messages[:1:1], a.Messages[1+over:]...)
	}
}

type tutorSession struct {
	// turnMu serializes the session's turns.
	turnMu sync.Mutex

	Phase     tutorPhase
	StudyCode string
	Week      int
	Activity  *activity
	// LastPracticeKind is the format of the latest problem, for "another one".
	LastPracticeKind  string
	History           []chatMessage
	FailedCodeLookups int
	UpdatedAt         time.Time

	prepMu   sync.Mutex
	prepared map[string]*preparedProblem
}

func newTutorSession() *tutorSession {
	return &tutorSession{Phase: phaseWelcome, prepared: map[string]*preparedProblem{}}
}

func (s *tutorSession) remember(user, assistant string) {
	if user != "" {
		s.History = append(s.History, chatMessage{Role: "user", Content: user})
	}
	if assistant != "" {
		s.History = append(s.History, chatMessage{Role: "assistant", Content: assistant})
	}
	if over := len(s.History) - maxHistoryMessages; over > 0 {
		s.History = append([]chatMessage(nil), s.History[over:]...)
	}
}

type sessionStore struct {
	mu    sync.Mutex
	items map[string]*tutorSession
}

func newSessionStore() *sessionStore {
	store := &sessionStore{items: map[string]*tutorSession{}}
	go store.cleanupLoop()
	return store
}

func (s *sessionStore) getOrCreate(sessionID string) *tutorSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.items[sessionID]
	if !ok {
		sess = newTutorSession()
		s.items[sessionID] = sess
	}
	sess.UpdatedAt = time.Now()
	return sess
}

func (s *sessionStore) get(sessionID string) (*tutorSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.items[sessionID]
	return sess, ok
}

func (s *sessionStore) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-25 * time.Hour)
		s.mu.Lock()
		for id, sess := range s.items {
			if sess.UpdatedAt.Before(cutoff) {
				delete(s.items, id)
			}
		}
		s.mu.Unlock()
	}
}
