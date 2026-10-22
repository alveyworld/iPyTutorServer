//go:build smoke

package main

// Calibration check: each weekly rubric has a worked example answer at each level. Run
// them through the Evaluator and the levels-only call and compare with the rubric's own
// rating.
//
//	OPENROUTER_API_KEY=... go test -tags smoke -run TestD5Calibration -v -timeout 10m

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

type rubricExample struct {
	week     int
	level    string
	mode     string
	target   string
	question string
	answer   string
}

// rubricExamples pulls every worked example under each level heading of a weekly rubric.
// After "**Rating:**", each example runs until its "**Issues:**"/"**Strengths:**" list;
// a later block of text after that list (e.g. "Multi-branch conditional:") is another example.
func rubricExamples(week int) []rubricExample {
	md := readInstructionFile(fmt.Sprintf("env/rubrics/week%d_rubric.md", week))
	topic := weeklyKeyConceptSelections[week-1].SelectedKeyConcept
	var out []rubricExample
	for _, h := range []struct {
		prefixes []string
		level    string
	}{
		{[]string{"Not Yet Ready", "Not Ready Yet"}, levelNotReady},
		{[]string{"Competent"}, levelCompetent},
		{[]string{"Exceptional"}, levelExceptional},
	} {
		var section string
		for _, p := range h.prefixes {
			if section = markdownSection(md, p); section != "" {
				break
			}
		}
		start := strings.Index(section, "**Rating:**")
		if start < 0 {
			continue
		}
		body := section[start:]
		body = body[strings.Index(body, "\n")+1:]
		var current []string
		inList, inFence := false, false
		flush := func() {
			text := strings.TrimSpace(strings.Join(current, "\n"))
			current = nil
			if text == "" {
				return
			}
			ex := rubricExample{week: week, level: h.level, answer: text, mode: modeConceptual, target: dimConceptual,
				question: fmt.Sprintf("In your own words, how would you explain %s, and when would you use it in your work?", topic)}
			if strings.Contains(text, "```") {
				ex.mode, ex.target = modeCode, dimCorrectness
				ex.question = fmt.Sprintf("Please paste your Python code for a small task that uses %s.", topic)
			}
			out = append(out, ex)
		}
		for _, line := range strings.Split(body, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") {
				inFence = !inFence
			}
			if !inFence && strings.HasPrefix(trimmed, "**") {
				flush()
				inList = true
				continue
			}
			if inList && !inFence && (trimmed == "" || strings.HasPrefix(trimmed, "- ")) {
				continue
			}
			inList = false
			current = append(current, line)
		}
		flush()
	}
	return out
}

func TestD5Calibration(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}
	runs := 3
	if v := os.Getenv("D5_CAL_RUNS"); v != "" {
		fmt.Sscanf(v, "%d", &runs)
	}
	var examples []rubricExample
	for week := 1; week <= lastSyllabusWeek; week++ {
		examples = append(examples, rubricExamples(week)...)
	}

	levelsGot := make([][]string, len(examples))
	evalGot := make([][]string, len(examples))
	for i := range examples {
		levelsGot[i] = make([]string, runs)
		evalGot[i] = make([]string, runs)
	}
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, ex := range examples {
		for r := 0; r < runs; r++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				state := newAgentSessionState()
				state.CurrentWeekNumber = ex.week
				state.SelectedKeyConcept = weeklyKeyConceptSelections[ex.week-1].SelectedKeyConcept
				state.StudentMajor = "Business"
				state.ActiveMode = ex.mode
				sess := newD5Session("calibration")
				sess.Transcript = []d5Message{
					{Role: "assistant", Content: ex.question, Mode: ex.mode},
					{Role: "user", Content: ex.answer, Mode: ex.mode},
				}
				res, _ := runBackgroundCall(apiKey, "calibration", levelsRequest("", d5LevelsMessages(state, ex.mode, ex.target, ex.question, ex.answer, ex.question)), d5LevelsTimeout*2, 1)
				levelsGot[i][r] = "?"
				if labels := parseTargetLevel(res.Content, ex.mode, ex.target, 1); len(labels) > 0 {
					levelsGot[i][r] = labels[0].Level
				}
				res, _ = runBackgroundCall(apiKey, "calibration", evaluatorRequest("", d5EvaluatorMessages(state, sess, ex.mode, 1, ex.target, nil, nil)), d5EvaluatorTimeout, 1)
				brief, _ := parseBrief(res.Content, ex.mode, 1)
				evalGot[i][r] = "?"
				for _, l := range brief.Levels {
					if l.Dimension == ex.target {
						evalGot[i][r] = l.Level
					}
				}
			}()
		}
	}
	wg.Wait()

	short := map[string]string{levelNotReady: "NR", levelCompetent: "C", levelExceptional: "E", "?": "?"}
	type tally struct{ agree, lower, higher, failed, consistent int }
	score := func(want string, got []string, tl *tally) string {
		var parts []string
		allSame := true
		for _, g := range got {
			parts = append(parts, short[g])
			switch {
			case g == "?":
				tl.failed++
			case g == want:
				tl.agree++
			case levelRank(g) < levelRank(want):
				tl.lower++
			default:
				tl.higher++
			}
			if g != got[0] {
				allSame = false
			}
		}
		if allSame {
			tl.consistent++
		}
		return strings.Join(parts, " ")
	}
	var lv, ev tally
	fmt.Printf("\n%-4s %-12s %-6s %-12s %-12s\n", "week", "dimension", "rubric", "levels-only", "evaluator")
	for i, ex := range examples {
		l := score(ex.level, levelsGot[i], &lv)
		e := score(ex.level, evalGot[i], &ev)
		fmt.Printf("%-4d %-12s %-6s %-12s %-12s\n", ex.week, ex.target, short[ex.level], l, e)
	}
	total := len(examples) * runs
	report := func(name string, tl tally) {
		fmt.Printf("%-12s agree %d/%d (%.0f%%) · too low %d · too high %d · failed %d · same answer all %d runs on %d/%d examples\n",
			name, tl.agree, total, 100*float64(tl.agree)/float64(total), tl.lower, tl.higher, tl.failed, runs, tl.consistent, len(examples))
	}
	fmt.Printf("\n%d examples × %d runs\n", len(examples), runs)
	report("levels-only", lv)
	report("evaluator", ev)
}

// TestD5OpeningAccuracy generates Code and Bug openings through the production path
// (draft, scope check, one regeneration if flagged) for weeks 2-8, D5_OPEN_RUNS per week
// and mode, and writes every draft, verdict and final opening to D5_OPEN_OUT for review.
func TestD5OpeningAccuracy(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}
	runs := 3
	if v := os.Getenv("D5_OPEN_RUNS"); v != "" {
		fmt.Sscanf(v, "%d", &runs)
	}
	majors := []string{"Genetics", "Accounting", "Nursing", "Graphic Design", "Agriculture", "Music"}
	type job struct {
		week, run int
		mode      string
	}
	var jobs []job
	modes := []string{modeCode, modeBug}
	if os.Getenv("D5_OPEN_MODES") == "bug" {
		modes = []string{modeBug}
	}
	for week := 2; week <= 8; week++ {
		for _, mode := range modes {
			for r := 0; r < runs; r++ {
				jobs = append(jobs, job{week, r, mode})
			}
		}
	}
	out := make([]string, len(jobs))
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	var mu sync.Mutex
	regenerated := 0
	for i, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			state := newAgentSessionState()
			state.CurrentWeekNumber = j.week
			state.SelectedKeyConcept = weeklyKeyConceptSelections[j.week-1].SelectedKeyConcept
			state.StudentMajor = majors[(i+j.run)%len(majors)]
			state.ActiveMode = j.mode
			sess := newD5Session("accuracy")
			state.D5 = sess
			sess.CompanyName, sess.CompanyDomain = "Northwind Group", "a mid-size company in the "+strings.ToLower(state.StudentMajor)+" field"
			p := chatRunParams{state: state, apiKey: apiKey, sessionID: fmt.Sprintf("acc-%d", i)}
			move := d5OpeningMove(state, sess, j.mode)
			draft := d5DraftOpening(p, sess, j.mode, move)
			final := draft.res.Content
			verified := draft.verifiedDefect
			feedback := strings.Join(draft.firstProblems, " / ")
			first := struct{ Content string }{draft.firstContent}
			if draft.regenerated {
				mu.Lock()
				regenerated++
				mu.Unlock()
			}
			var b strings.Builder
			fmt.Fprintf(&b, "########## #%d Week %d %s run %d (%s)\n", i+1, j.week, modeTitle(j.mode), j.run+1, state.StudentMajor)
			if feedback != "" {
				label := "regenerated"
				if draft.keptFirst {
					label = fmt.Sprintf("first draft kept: retry had %d problems: %s", len(draft.retryProblems), strings.Join(draft.retryProblems, " / "))
				}
				fmt.Fprintf(&b, "--- FIRST DRAFT (flagged: %s)\n%s\n--- FINAL (%s)\n", strings.ReplaceAll(feedback, "\n", " / "), strings.TrimSpace(first.Content), label)
			} else {
				b.WriteString("--- FINAL (first draft passed)\n")
			}
			if j.mode == modeBug {
				rest, defect := splitDefectLine(final)
				fmt.Fprintf(&b, "%s\n[hidden] DEFECT (model): %s\n", rest, defect)
				if verified != "" {
					fmt.Fprintf(&b, "[hidden] DEFECT (checker, used): %s\n", verified)
				}
			} else {
				b.WriteString(strings.TrimSpace(final) + "\n")
			}
			out[i] = b.String()
		}()
	}
	wg.Wait()
	path := os.Getenv("D5_OPEN_OUT")
	if path == "" {
		path = "openings-review.txt"
	}
	_ = os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
	fmt.Printf("%d openings, %d regenerated; written to %s\n", len(jobs), regenerated, path)
}

// TestD5DraftAccuracy runs the Phase 2 background draft job for Bug openings, weeks 2-8,
// D5_OPEN_RUNS per week, and writes each final draft for review.
func TestD5DraftAccuracy(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}
	runs := 6
	if v := os.Getenv("D5_OPEN_RUNS"); v != "" {
		fmt.Sscanf(v, "%d", &runs)
	}
	majors := []string{"Genetics", "Accounting", "Nursing", "Graphic Design", "Agriculture", "Music"}
	type job struct{ week, run int }
	var jobs []job
	for week := 2; week <= 8; week++ {
		for r := 0; r < runs; r++ {
			jobs = append(jobs, job{week, r})
		}
	}
	out := make([]string, len(jobs))
	var times []time.Duration
	var mu sync.Mutex
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			state := newAgentSessionState()
			state.CurrentWeekNumber = j.week
			state.SelectedKeyConcept = weeklyKeyConceptSelections[j.week-1].SelectedKeyConcept
			state.StudentMajor = majors[(i+j.run)%len(majors)]
			state.ActiveMode = modeCode
			sess := newD5Session(fmt.Sprintf("draft-%d", i))
			state.D5 = sess
			sess.CompanyName, sess.CompanyDomain = "Northwind Group", "a mid-size company in the "+strings.ToLower(state.StudentMajor)+" field"
			p := chatRunParams{state: state, apiKey: apiKey, sessionID: fmt.Sprintf("draft-%d", i)}
			started := time.Now()
			d5StartDraft(p, sess, modeBug)
			sess.mu.Lock()
			done := sess.draftDone[modeBug]
			sess.mu.Unlock()
			<-done
			took := time.Since(started)
			sess.mu.Lock()
			d := sess.Drafts[modeBug]
			sess.mu.Unlock()
			var b strings.Builder
			fmt.Fprintf(&b, "########## #%d Week %d Bug draft run %d (%s) · %.0fs\n", i+1, j.week, j.run+1, state.StudentMajor, took.Seconds())
			if d == nil {
				b.WriteString("NO DRAFT\n")
			} else {
				rest, defect := splitDefectLine(d.Content)
				fmt.Fprintf(&b, "attempts %d · problems %d %q\n%s\n[hidden] DEFECT (model): %s\n[hidden] DEFECT (checker): %s\n", d.Attempts, len(d.Problems), d.Problems, rest, defect, d.Verified)
			}
			out[i] = b.String()
			mu.Lock()
			times = append(times, took)
			mu.Unlock()
		}()
	}
	wg.Wait()
	path := os.Getenv("D5_OPEN_OUT")
	if path == "" {
		path = "drafts-review.txt"
	}
	_ = os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
	sort.Slice(times, func(a, b int) bool { return times[a] < times[b] })
	fmt.Printf("%d drafts · time p50 %.0fs · max %.0fs · written to %s\n", len(jobs), times[len(times)/2].Seconds(), times[len(times)-1].Seconds(), path)
}
