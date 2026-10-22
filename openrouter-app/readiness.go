package main

import (
	"fmt"
	"strings"
)

// partStatus is a student's standing in one interview part, from their practice of it.
type partStatus struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	// Level is the rating of the latest rated attempt; empty when nothing is rated yet.
	Level      string `json:"level,omitempty"`
	LevelLabel string `json:"levelLabel,omitempty"`
	Attempts   int    `json:"attempts"`
}

type readiness struct {
	Parts []partStatus `json:"parts"`
	// Label is a short verdict such as "Likely Competent".
	Label string `json:"label"`
	// Ready is set when every part's latest rating is Competent or better.
	Ready bool `json:"ready"`
}

// weekReadiness estimates the interview rating the student would get today, with the
// interview's own rule: Not Ready Yet in any part makes the whole Not Ready Yet, and
// Exceptional needs Exceptional in every part. Each part counts its latest rated attempt,
// so it tracks improvement rather than old mistakes.
func weekReadiness(w *weekContent, wp *weekProgress) readiness {
	r := readiness{}
	unrated, notReady, exceptional, practised := 0, 0, 0, 0
	for _, kind := range w.practiceKinds() {
		ps := partStatus{Kind: kind, Label: interviewPartLabel(w.Number, kind)}
		if wp != nil {
			for _, rec := range wp.Practice {
				if rec.Kind != kind {
					continue
				}
				if rec.Attempts > 0 {
					ps.Attempts++
				}
				if rec.Level != "" {
					ps.Level = rec.Level
				}
			}
		}
		ps.LevelLabel = levelLabel(ps.Level)
		if ps.Attempts > 0 {
			practised++
		}
		switch ps.Level {
		case "":
			unrated++
		case levelNotReady:
			notReady++
		case levelExceptional:
			exceptional++
		}
		r.Parts = append(r.Parts, ps)
	}
	switch {
	case practised == 0 && unrated == len(r.Parts):
		r.Label = "Not started"
	case unrated > 0:
		r.Label = "Keep practicing"
	case notReady > 0:
		r.Label = "Not Ready Yet"
	case exceptional == len(r.Parts):
		r.Label, r.Ready = "Likely Exceptional", true
	default:
		r.Label, r.Ready = "Likely Competent", true
	}
	return r
}

// nextLessonIndex returns the first lesson not yet done, or -1 when all are done.
func nextLessonIndex(w *weekContent, wp *weekProgress) int {
	for i := range w.Lessons {
		if wp == nil || !wp.lessonDone(i) {
			return i
		}
	}
	return -1
}

func lessonsDone(w *weekContent, wp *weekProgress) int {
	if wp == nil {
		return 0
	}
	n := 0
	for i := range w.Lessons {
		if wp.lessonDone(i) {
			n++
		}
	}
	return n
}

// suggestedPractice picks the practice format that most needs work: one never rated, then
// one rated Not Ready Yet, then one short of Exceptional.
func suggestedPractice(w *weekContent, wp *weekProgress) string {
	r := weekReadiness(w, wp)
	for _, want := range []string{"", levelNotReady, levelCompetent} {
		for _, p := range r.Parts {
			if p.Level == want {
				return p.Kind
			}
		}
	}
	return w.practiceKinds()[0]
}

// nextStep is one sentence telling the student what to do next this week.
func nextStep(w *weekContent, wp *weekProgress) string {
	r := weekReadiness(w, wp)
	var missing, weak, short []string
	for _, p := range r.Parts {
		label := strings.ToLower(practiceLabel(w.Number, p.Kind))
		switch p.Level {
		case "":
			missing = append(missing, label)
		case levelNotReady:
			weak = append(weak, label)
		case levelCompetent:
			short = append(short, label)
		}
	}
	// Lessons come first until the student starts practising.
	if i := nextLessonIndex(w, wp); i >= 0 && len(missing) == len(r.Parts) {
		if i == 0 {
			return fmt.Sprintf("Start with lesson 1, %q: choose **Teach me**.", w.Lessons[0].Title)
		}
		return fmt.Sprintf("Continue with lesson %d, %q, or try a first %s.", i+1, w.Lessons[i].Title, missing[0])
	}
	switch {
	case len(missing) > 0:
		return fmt.Sprintf("Try a %s next; every part of the interview needs practice.", missing[0])
	case len(weak) > 0:
		return fmt.Sprintf("Do another %s: your latest one was Not Ready Yet, and that part decides the overall rating.", weak[0])
	case len(short) > 0:
		return fmt.Sprintf("You look ready for the interview. To aim for Exceptional, practice another %s.", short[0])
	}
	return "You look ready for the interview at the Exceptional level. Take it, or review an earlier week."
}
