package main

import (
	"strings"
	"testing"
)

func practiceWeek(levels map[string][]string) *weekProgress {
	wp := &weekProgress{}
	for kind, ls := range levels {
		for _, l := range ls {
			wp.addRecord(practiceRecord{Kind: kind, Attempts: 1, Level: l})
		}
	}
	return wp
}

func TestWeekReadinessFollowsInterviewRule(t *testing.T) {
	w := weekInfo(5)
	cases := []struct {
		name  string
		wp    *weekProgress
		label string
		ready bool
	}{
		{"nothing yet", nil, "Not started", false},
		{"one part missing", practiceWeek(map[string][]string{kindConcept: {levelExceptional}, kindCode: {levelExceptional}}), "Keep practicing", false},
		{"one part not ready", practiceWeek(map[string][]string{kindConcept: {levelExceptional}, kindCode: {levelNotReady}, kindBug: {levelExceptional}}), "Not Ready Yet", false},
		{"mixed", practiceWeek(map[string][]string{kindConcept: {levelCompetent}, kindCode: {levelExceptional}, kindBug: {levelExceptional}}), "Likely Competent", true},
		{"all exceptional", practiceWeek(map[string][]string{kindConcept: {levelExceptional}, kindCode: {levelExceptional}, kindBug: {levelExceptional}}), "Likely Exceptional", true},
		{"latest attempt counts", practiceWeek(map[string][]string{kindConcept: {levelNotReady, levelCompetent}, kindCode: {levelNotReady, levelExceptional}, kindBug: {levelCompetent}}), "Likely Competent", true},
	}
	for _, c := range cases {
		r := weekReadiness(w, c.wp)
		if r.Label != c.label || r.Ready != c.ready {
			t.Errorf("%s: got %q ready=%v, want %q ready=%v", c.name, r.Label, r.Ready, c.label, c.ready)
		}
	}
}

func TestUnratedAttemptsStillCount(t *testing.T) {
	wp := &weekProgress{}
	wp.addRecord(practiceRecord{Kind: kindBug, Attempts: 1})
	r := weekReadiness(weekInfo(5), wp)
	if r.Label != "Keep practicing" || r.Parts[2].Attempts != 1 || r.Parts[2].Level != "" {
		t.Fatalf("readiness = %+v", r)
	}
}

func TestWeek1ReadinessIsDecompositionOnly(t *testing.T) {
	w := weekInfo(1)
	r := weekReadiness(w, practiceWeek(map[string][]string{kindConcept: {levelExceptional}}))
	if len(r.Parts) != 1 || r.Label != "Likely Exceptional" || !r.Ready {
		t.Fatalf("week 1 readiness = %+v", r)
	}
	if r.Parts[0].Label != "Decomposition" {
		t.Fatalf("week 1 part label = %q", r.Parts[0].Label)
	}
}

func TestNextStep(t *testing.T) {
	w := weekInfo(5)
	if got := nextStep(w, nil); !strings.Contains(got, "lesson 1") {
		t.Errorf("fresh week: %q", got)
	}
	wp := &weekProgress{}
	wp.markLessonDone(0)
	if got := nextStep(w, wp); !strings.Contains(got, "lesson 2") {
		t.Errorf("after lesson 1: %q", got)
	}
	wp = practiceWeek(map[string][]string{kindConcept: {levelCompetent}})
	if got := nextStep(w, wp); !strings.Contains(got, "code problem") {
		t.Errorf("code never tried: %q", got)
	}
	wp = practiceWeek(map[string][]string{kindConcept: {levelCompetent}, kindCode: {levelExceptional}, kindBug: {levelNotReady}})
	if got := nextStep(w, wp); !strings.Contains(got, "bug hunt") || !strings.Contains(got, "Not Ready Yet") {
		t.Errorf("bug not ready: %q", got)
	}
	if got := suggestedPractice(w, wp); got != kindBug {
		t.Errorf("suggested = %q, want bug", got)
	}
	wp = practiceWeek(map[string][]string{kindConcept: {levelCompetent}, kindCode: {levelExceptional}, kindBug: {levelExceptional}})
	if got := nextStep(w, wp); !strings.Contains(got, "ready") || !strings.Contains(got, "concept question") {
		t.Errorf("concept only competent: %q", got)
	}
}
