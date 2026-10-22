package main

// stateResponse is the session state the page shows beside the chat: the student, their
// week's progress, and what they are working on, which decides the buttons offered.
type stateResponse struct {
	Phase     string       `json:"phase"`
	StudyCode string       `json:"studyCode,omitempty"`
	Major     string       `json:"major,omitempty"`
	Weeks     []weekOption `json:"weeks"`
	Week      *weekState   `json:"week,omitempty"`
	Activity  *activityRef `json:"activity,omitempty"`
}

type weekOption struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Readiness string `json:"readiness,omitempty"`
}

type weekState struct {
	Number        int            `json:"number"`
	Title         string         `json:"title"`
	LessonsDone   int            `json:"lessonsDone"`
	LessonsTotal  int            `json:"lessonsTotal"`
	NextLesson    string         `json:"nextLesson,omitempty"`
	Readiness     readiness      `json:"readiness"`
	NextStep      string         `json:"nextStep"`
	PracticeKinds []practiceKind `json:"practiceKinds"`
}

type practiceKind struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

type activityRef struct {
	Kind   string `json:"kind"`
	Title  string `json:"title,omitempty"`
	Stage  string `json:"stage,omitempty"`
	Lesson int    `json:"lesson,omitempty"`
	// Open is set while a practice problem still takes answers, hints and the solution.
	Open bool `json:"open"`
}

// state builds the page state. The caller holds sess.turnMu.
func (t *tutor) state(sess *tutorSession) stateResponse {
	st := stateResponse{Phase: string(sess.Phase)}
	var p studentProfile
	if sess.StudyCode != "" {
		if got, err := t.store.get(sess.StudyCode); err == nil {
			p = got
			st.StudyCode, st.Major = p.StudyCode, p.Major
		}
	}
	for _, sel := range weeklyKeyConceptSelections {
		opt := weekOption{Number: sel.CurrentWeekNumber, Title: sel.SelectedKeyConcept}
		if wp := p.Weeks[sel.CurrentWeekNumber]; wp != nil {
			opt.Readiness = weekReadiness(weekInfo(sel.CurrentWeekNumber), wp).Label
		}
		st.Weeks = append(st.Weeks, opt)
	}
	if w := weekInfo(sess.Week); w != nil && st.StudyCode != "" {
		wp := p.Weeks[w.Number]
		ws := &weekState{
			Number:       w.Number,
			Title:        w.Title,
			LessonsDone:  lessonsDone(w, wp),
			LessonsTotal: len(w.Lessons),
			Readiness:    weekReadiness(w, wp),
			NextStep:     nextStep(w, wp),
		}
		if i := nextLessonIndex(w, wp); i >= 0 {
			ws.NextLesson = w.Lessons[i].Title
		}
		for _, k := range w.practiceKinds() {
			ws.PracticeKinds = append(ws.PracticeKinds, practiceKind{Kind: k, Label: practiceLabel(w.Number, k)})
		}
		st.Week = ws
	}
	if a := sess.Activity; a != nil {
		ref := &activityRef{Kind: a.Kind, Title: a.Title, Open: a.open()}
		if a.isPractice() {
			ref.Stage = a.Stage
		} else {
			ref.Lesson = a.Lesson + 1
		}
		st.Activity = ref
	}
	return st
}
