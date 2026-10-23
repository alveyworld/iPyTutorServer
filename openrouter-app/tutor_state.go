package main

// stateResponse is the session state the page shows beside the chat: the week being
// studied, its lessons, and what the student is working on, which decides the buttons
// offered. Nothing in it is kept beyond the session.
type stateResponse struct {
	Phase    string       `json:"phase"`
	Major    string       `json:"major,omitempty"`
	Weeks    []weekOption `json:"weeks"`
	Week     *weekState   `json:"week,omitempty"`
	Activity *activityRef `json:"activity,omitempty"`
}

type weekOption struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
}

type weekState struct {
	Number        int            `json:"number"`
	Title         string         `json:"title"`
	Lessons       []string       `json:"lessons"`
	PracticeKinds []practiceKind `json:"practiceKinds"`
}

type practiceKind struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

type activityRef struct {
	Kind  string `json:"kind"`
	Title string `json:"title,omitempty"`
	Stage string `json:"stage,omitempty"`
	// Lesson is the lesson number (from 1) for a lesson.
	Lesson int `json:"lesson,omitempty"`
	// Open is set while a practice problem still takes answers, hints and the solution.
	Open bool `json:"open"`
}

// state builds the page state. The caller holds s.turnMu.
func (s *tutorSession) state() stateResponse {
	st := stateResponse{Phase: string(s.Phase), Major: s.Major}
	for _, sel := range weeklyKeyConceptSelections {
		st.Weeks = append(st.Weeks, weekOption{Number: sel.CurrentWeekNumber, Title: sel.SelectedKeyConcept})
	}
	if w := weekInfo(s.Week); w != nil {
		ws := &weekState{Number: w.Number, Title: w.Title}
		for _, l := range w.Lessons {
			ws.Lessons = append(ws.Lessons, l.Title)
		}
		for _, k := range w.practiceKinds() {
			ws.PracticeKinds = append(ws.PracticeKinds, practiceKind{Kind: k, Label: practiceLabel(w.Number, k)})
		}
		st.Week = ws
	}
	if a := s.Activity; a != nil {
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
