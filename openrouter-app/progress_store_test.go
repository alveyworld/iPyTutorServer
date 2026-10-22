package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeStudyCode(t *testing.T) {
	cases := map[string]string{
		"maple-river-4821":      "maple-river-4821",
		"  Maple-River-4821 ":   "maple-river-4821",
		"`maple-river-4821`.":   "maple-river-4821",
		"maple river 4821":      "maple-river-4821",
		"maple_river_4821":      "maple-river-4821",
		"\"maple--river-4821\"": "maple-river-4821",
	}
	for in, want := range cases {
		if got, ok := normalizeStudyCode(in); !ok || got != want {
			t.Errorf("normalizeStudyCode(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "biology", "maple-river", "maple-river-48", "../../etc/passwd", "maple-river-4821-x", "my code is maple-river-4821"} {
		if got, ok := normalizeStudyCode(in); ok {
			t.Errorf("normalizeStudyCode(%q) = %q, want no code", in, got)
		}
	}
}

func TestRandomStudyCodesAreValid(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		code, err := randomStudyCode()
		if err != nil {
			t.Fatal(err)
		}
		if !studyCodePattern.MatchString(code) {
			t.Fatalf("invalid code %q", code)
		}
		seen[code] = true
	}
	if len(seen) < 190 {
		t.Fatalf("only %d distinct codes in 200", len(seen))
	}
}

func TestProgressSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	store, err := newProgressStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.create("Biology")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, p.StudyCode+".json")); err != nil {
		t.Fatalf("profile not written: %v", err)
	}
	if _, err := store.update(p.StudyCode, func(p *studentProfile) {
		p.CurrentWeek = 5
		wp := p.week(5)
		wp.markLessonDone(0)
		wp.markLessonDone(0)
		wp.addRecord(practiceRecord{ID: "r1", Kind: kindCode, Title: "Lab sorter", Attempts: 2, Level: levelCompetent})
	}); err != nil {
		t.Fatal(err)
	}

	reopened, err := newProgressStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.get(p.StudyCode)
	if err != nil {
		t.Fatal(err)
	}
	wp := got.Weeks[5]
	if got.Major != "Biology" || got.CurrentWeek != 5 || wp == nil {
		t.Fatalf("reloaded profile = %+v", got)
	}
	if len(wp.LessonsDone) != 1 || !wp.lessonDone(0) {
		t.Fatalf("lessons done = %v", wp.LessonsDone)
	}
	if rec := wp.record("r1"); rec == nil || rec.Level != levelCompetent || rec.Attempts != 2 {
		t.Fatalf("record = %+v", rec)
	}

	// A copy handed out must not alias the stored profile.
	got.Weeks[5].Practice[0].Level = levelNotReady
	again, _ := reopened.get(p.StudyCode)
	if again.Weeks[5].Practice[0].Level != levelCompetent {
		t.Fatal("changing a returned copy changed the stored profile")
	}
}

func TestUnknownAndInvalidCodes(t *testing.T) {
	store, err := newProgressStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"maple-river-4821", "../secret", "a/b-c-1234"} {
		if _, err := store.get(code); !errors.Is(err, errUnknownStudyCode) {
			t.Errorf("get(%q) err = %v, want errUnknownStudyCode", code, err)
		}
		if _, err := store.update(code, func(*studentProfile) {}); !errors.Is(err, errUnknownStudyCode) {
			t.Errorf("update(%q) err = %v, want errUnknownStudyCode", code, err)
		}
	}
}

func TestPracticeRecordsAreCapped(t *testing.T) {
	wp := &weekProgress{}
	for i := 0; i < maxPracticeRecordsPerWeek+5; i++ {
		wp.addRecord(practiceRecord{ID: string(rune('a' + i%26)), Kind: kindBug, Title: "t"})
	}
	if len(wp.Practice) != maxPracticeRecordsPerWeek {
		t.Fatalf("kept %d records, want %d", len(wp.Practice), maxPracticeRecordsPerWeek)
	}
	wp.addRecord(practiceRecord{ID: "x", Kind: kindCode, Title: "Newest code"})
	if got := wp.recentTitles(kindCode, 3); len(got) != 1 || got[0] != "Newest code" {
		t.Fatalf("recentTitles = %v", got)
	}
}
