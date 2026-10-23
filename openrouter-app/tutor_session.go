package main

import (
	"fmt"
	"sync"
	"time"
)

// A tutorSession is one browser login's conversation with the tutor. It lives in memory
// only: the tutor keeps no record of a student between visits, and a student can study
// any week and lesson at any time.

type tutorPhase string

const (
	// phaseWelcome waits for the student's major or a week to study.
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
	// maxRecentTitles is how many problem titles per week and format are kept, so new
	// problems in the same visit don't repeat them.
	maxRecentTitles = 6
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

	Phase tutorPhase
	// started is set once a page load or a turn has used the session. A turn on a session
	// that never started, from a browser holding a conversation, follows a server restart.
	started bool
	// Major sets the field for examples and problems, as the interview does; optional.
	Major    string
	Week     int
	Activity *activity
	// LastLesson is the index of the latest lesson taught this week, for "next lesson";
	// -1 when none has been.
	LastLesson int
	// LastPracticeKind is the format of the latest problem, for "another one".
	LastPracticeKind string
	// recentTitles holds the latest problem titles by week and format.
	recentTitles map[string][]string
	History      []chatMessage
	UpdatedAt    time.Time

	prepMu   sync.Mutex
	prepared map[string]*preparedProblem
}

func newTutorSession() *tutorSession {
	return &tutorSession{Phase: phaseWelcome, LastLesson: -1, recentTitles: map[string][]string{}, prepared: map[string]*preparedProblem{}}
}

func titlesKey(week int, kind string) string {
	return fmt.Sprintf("%d/%s", week, kind)
}

// addRecentTitle remembers a problem served this visit.
func (s *tutorSession) addRecentTitle(week int, kind, title string) {
	key := titlesKey(week, kind)
	titles := append([]string{title}, s.recentTitles[key]...)
	if len(titles) > maxRecentTitles {
		titles = titles[:maxRecentTitles]
	}
	s.recentTitles[key] = titles
}

// recentProblemTitles lists this visit's problem titles for a week and format, newest first.
func (s *tutorSession) recentProblemTitles(week int, kind string) []string {
	return append([]string(nil), s.recentTitles[titlesKey(week, kind)]...)
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
