//go:build smoke

package main

// Full-assessment runs: setup, Conceptual, Code, Bug, results, then a switch to coach
// mode, driven through the real /api/chat handler against the live model. A simulated
// student (a separate model call) answers each interviewer message with an exceptional,
// competent, not_ready or engineer profile. Stats come from the session state and the [d5] log lines.
//
//	OPENROUTER_API_KEY=... go test -tags smoke -run TestD5FullRuns -v -timeout 60m
//
// D5_FULL_RUNS (default 32) runs, D5_FULL_PAR (default 8) at a time, D5_FULL_OUT for the
// transcript file (default fullruns-transcripts.txt).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// studentProfiles follow the rubric's levels (exceptional, competent, not_ready), plus
// working software engineers.
var studentProfiles = map[string]string{
	"engineer":    "You are a professional Python software engineer with eight years of experience. Your answers are precise, complete and well reasoned: you state the answer first, explain why, give a concrete example or edge case, and mention trade-offs where they matter. Your code is clean, correct, uses clear names, handles invalid input, and you can explain every line. You used AI to help write your code, as you do at work, and you always say so: say what you asked it, how you reviewed and tested what it gave you, and what you changed.",
	"exceptional": "You are an exceptional student who has mastered everything you have learned. In every answer: state the answer, explain precisely why it works (how Python behaves), give a concrete example with actual values, and mention an edge case or boundary. When breaking a problem down, list the steps in order with what goes in and comes out of each, and the edge cases you would test. When explaining your code, say what each line does and why you chose it over an alternative. When looking for a bug, give a deliberate plan: reproduce it with a specific input, predict the expected and the wrong values, name two or three likely causes ranked, say what would confirm or rule out each, test boundary values, and explain how the Python behaviour produces the symptom. You used AI to help write your code, and you always say so: say what you asked it, how you checked its code (running it with specific test values, including edge cases), and what you changed or learned as a result.",
	"competent":   "You are a competent student: your answers are correct and adequate, with a short reason, but you do not always give an example or an edge case. When breaking a problem down you give the main steps in order. When explaining your code you say what the lines do, with less on why. When looking for a bug you reproduce it with a concrete input, name one plausible cause, and add a print at a sensible point, with a next step if that does not explain it. You used AI to help write your code; if asked, say so and that you ran it a couple of times to check it worked.",
	"not_ready":   "You are a struggling beginner who leans on AI and has not really understood the material.",
}

// weakMisconceptions are real beginner misunderstandings, by week, separated by "; ".
var weakMisconceptions = map[int]string{
	2: "dividing two whole numbers with / always gives a whole number; = and == mean the same thing; a variable name and the text in quotes are the same thing",
	3: "input() already gives you a number, so converting with int() or float() is not needed; when a calculation fails it is print()'s fault",
	4: ".upper() and .strip() change the original string without assigning the result back; split() removes all spaces from a string",
	5: "every elif is checked even after an earlier condition matched, so order does not matter; and and or mean the same thing; = compares two values",
	6: "range(5) gives the numbers 1 to 5; a running total should be reset inside the loop; the loop variable keeps its first value",
	7: "a while loop runs once and then stops by itself; you do not need to change the loop variable inside a while loop",
	8: "list indexes start at 1; .append() returns the new list; it is safe to remove items from a list while looping over it",
	9: "open() reads the whole file into numbers automatically; lines read from a file have no newline at the end; a file must be read twice to loop over it",
}

// weakTurnBehaviour tells the weak student how to answer this turn: the code they paste
// works (as if from AI), but they cannot explain it, and their own reasoning rests on a
// misconception. One concrete instruction per turn.
func weakTurnBehaviour(week int, mode string, codePasted bool, turn int) string {
	misconceptions := strings.Split(weakMisconceptions[week], "; ")
	m := misconceptions[turn%len(misconceptions)]
	switch {
	case mode == modeCode && !codePasted:
		return "If you are asked to write or paste code, paste correct, working Python for the task, as if an AI wrote it for you. Otherwise, when asked how you would break the problem down, give a vague answer that skips steps."
	case mode == modeCode && codePasted:
		return "The code you pasted came from an AI and you do not understand it. If asked why you did something or what a line does, either say the AI wrote that part and you are not sure, or give a confidently wrong explanation (say a line does something it does not). If asked how you used or checked AI, say you pasted the problem into an AI and ran the code once."
	case mode == modeBug:
		return "When asked how you would find the bug, guess at a cause without checking, or suggest rewriting the code or asking someone. Never describe testing step by step."
	default:
		return "In this answer you firmly believe that " + m + ". Build your answer on that belief, or give a vague answer that does not really address the question."
	}
}

// simulateStudent answers the interviewer's latest message as a student with the profile.
func simulateStudent(apiKey string, week int, profile string, history []chatMessage, mode string, codePasted bool, turn int) (string, error) {
	background := fmt.Sprintf("You are a student in an introductory Python course, in a mock job interview. You have learned only: %s.", learnedConceptsSummary(week))
	if profile == "engineer" {
		background = "You are in a mock job interview for an entry-level role that uses Python."
	}
	if (profile == "exceptional" || profile == "engineer") && mode != modeConceptual {
		background += "\nIn this answer, mention that you used AI and how you checked or questioned what it gave you."
	}
	if profile == "not_ready" {
		background += "\n" + weakTurnBehaviour(week, mode, codePasted, turn) +
			"\nNever correct yourself, even if the interviewer hints, and never say \"actually\" or \"oh, right\"."
	}
	system := fmt.Sprintf(`%s
%s
Reply to the interviewer's latest message as the candidate, in %s of plain conversational text. If they ask you to write or paste code, reply with only Python code in a single python code block, using only what you have learned. Never write the interviewer's lines.`, background, studentProfiles[profile], answerLength(profile))
	msgs := []chatMessage{{Role: "system", Content: system}}
	if len(history) > 8 {
		history = history[len(history)-8:]
	}
	// From the student's side, the interviewer is the "user".
	for _, m := range history {
		role := "user"
		if m.Role == "user" {
			role = "assistant"
		}
		msgs = append(msgs, chatMessage{Role: role, Content: m.Content})
	}
	req := d5Request{Model: resolveChatModel(""), Messages: msgs, MaxTokens: 400, Reasoning: map[string]any{"enabled": false}, Usage: map[string]any{"include": true}}
	res, err := runBackgroundCall(apiKey, "student", req, 30*time.Second, 2)
	return strings.TrimSpace(res.Content), err
}

type fullRun struct {
	id, major, profile string
	week               int
	completed          bool
	stoppedAt          string
	failure            string
	turns              int
	replyTimes         []time.Duration
	conceptualQs       int
	codeQs             int
	bugQs              int
	codePasted         bool
	buckets            [4]string
	coachOK            bool
	coachHasCode       bool
	transcript         strings.Builder
}

func TestD5FullRuns(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}
	profiles := []string{"exceptional", "competent", "not_ready", "engineer"}
	if v := os.Getenv("D5_FULL_PROFILES"); v != "" {
		profiles = strings.Split(v, ",")
	}
	runs, par := 8*len(profiles), 8
	if v := os.Getenv("D5_FULL_RUNS"); v != "" {
		fmt.Sscanf(v, "%d", &runs)
	}
	if v := os.Getenv("D5_FULL_PAR"); v != "" {
		fmt.Sscanf(v, "%d", &par)
	}
	saved := chatEngine
	chatEngine = "d5"
	defer func() { chatEngine = saved }()
	logs := &lockedBuffer{}
	log.SetOutput(logs)
	defer log.SetOutput(os.Stderr)

	majors := []string{"Nursing", "Accounting", "Biology", "Graphic Design", "Agriculture", "Music", "Exercise Science", "History"}
	states, turns := newAgentStateStore(), newTurnStore()
	started := time.Now()
	var doneMu sync.Mutex
	done := 0
	results := make([]*fullRun, runs)
	sem := make(chan struct{}, par)
	var wg sync.WaitGroup
	for i := 0; i < runs; i++ {
		run := &fullRun{
			id:      fmt.Sprintf("r%02dxxxxx-full", i),
			week:    2 + i%8,
			profile: profiles[(i/8)%len(profiles)],
			major:   majors[(i*3)%len(majors)],
		}
		results[i] = run
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			runStart := time.Now()
			driveFullRun(t, apiKey, states, turns, run)
			doneMu.Lock()
			done++
			status := "completed"
			if !run.completed {
				status = "FAILED: " + run.failure
			}
			fmt.Printf("[progress] %d/%d done · %s week %d %s student · %s · overall %s · %.0fs (elapsed %.0fs)\n",
				done, runs, run.id[:3], run.week, run.profile, status, run.buckets[3], time.Since(runStart).Seconds(), time.Since(started).Seconds())
			doneMu.Unlock()
		}()
	}
	wg.Wait()
	time.Sleep(5 * time.Second) // let trailing Evaluator jobs log

	path := os.Getenv("D5_FULL_OUT")
	if path == "" {
		path = "fullruns-transcripts.txt"
	}
	var all strings.Builder
	for _, r := range results {
		all.WriteString(r.transcript.String() + "\n")
	}
	_ = os.WriteFile(path, []byte(all.String()), 0o644)
	reportFullRuns(results, logs.String(), path)
}

func driveFullRun(t *testing.T, apiKey string, states *agentStateStore, turns *turnStore, run *fullRun) {
	turn := 0
	var history []chatMessage
	post := func(text string) (string, int, time.Duration) {
		turn++
		body, _ := json.Marshal(chatCompletionRequest{Messages: []chatMessage{{Role: "user", Content: text}}})
		req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(string(body)))
		req = req.WithContext(context.WithValue(req.Context(), sessionIDContextKey, run.id))
		req.Header.Set(turnIDHeader, fmt.Sprintf("%s-%d", run.id, turn))
		rec := httptest.NewRecorder()
		started := time.Now()
		handleChat(apiKey, states, turns)(rec, req)
		return extractAssistantContent(rec.Body.Bytes()), rec.Code, time.Since(started)
	}
	fmt.Fprintf(&run.transcript, "########## %s · week %d · %s student · %s\n", run.id[:3], run.week, run.profile, run.major)

	// Bootstrap like the browser does, then setup.
	rec := httptest.NewRecorder()
	handleBootstrap(states, apiKey)(rec, httptest.NewRequest(http.MethodPost, "/api/session/bootstrap", nil).WithContext(context.WithValue(context.Background(), sessionIDContextKey, run.id)))
	post(run.major)
	reply, code, took := post(fmt.Sprint(run.week))
	history = append(history, chatMessage{Role: "assistant", Content: reply})
	fmt.Fprintf(&run.transcript, "\n[interviewer · %.1fs]\n%s\n", took.Seconds(), reply)
	if code != http.StatusOK || reply == "" {
		run.failure = fmt.Sprintf("opening returned HTTP %d", code)
		return
	}
	run.replyTimes = append(run.replyTimes, took)

	for run.turns = 0; run.turns < 40; run.turns++ {
		st, _ := states.get(run.id)
		if st.ConversationPhase != phaseAssessmentInProgress {
			break
		}
		answer, err := simulateStudent(apiKey, run.week, run.profile, history, st.ActiveMode, st.D5 != nil && st.D5.CodePasted, run.turns)
		if err != nil || answer == "" {
			run.failure = fmt.Sprintf("student simulator failed: %v", err)
			run.stoppedAt = st.ActiveMode
			return
		}
		time.Sleep(3 * time.Second) // typing time, during which the Evaluator works
		history = append(history, chatMessage{Role: "user", Content: answer})
		fmt.Fprintf(&run.transcript, "\n[student]\n%s\n", answer)
		reply, code, took = post(answer)
		fmt.Fprintf(&run.transcript, "\n[interviewer · %.1fs]\n%s\n", took.Seconds(), reply)
		if code != http.StatusOK || reply == "" {
			run.failure = fmt.Sprintf("turn returned HTTP %d", code)
			run.stoppedAt = st.ActiveMode
			return
		}
		run.replyTimes = append(run.replyTimes, took)
		history = append(history, chatMessage{Role: "assistant", Content: reply})
	}
	st, _ := states.get(run.id)
	if st.ConversationPhase != phaseAssessmentResults {
		run.failure = "did not reach results within 40 answers"
		run.stoppedAt = st.ActiveMode
		return
	}
	reply, code, took = post("switch to coach mode")
	fmt.Fprintf(&run.transcript, "\n[student]\nswitch to coach mode\n\n[coach · %.1fs]\n%s\n", took.Seconds(), reply)
	run.coachOK = code == http.StatusOK && strings.Contains(reply, "recruiter on the HR team")
	run.coachHasCode = strings.Contains(reply, "```")
	run.completed = run.coachOK

	st, _ = states.get(run.id)
	run.buckets = [4]string{st.ConceptualAssessmentBucket, st.CodeAssessmentBucket, st.BugAssessmentBucket, st.FinalRating}
	run.codePasted = st.D5.CodePasted
	for _, m := range st.D5.Transcript {
		if m.Role != "assistant" {
			continue
		}
		switch m.Mode {
		case modeConceptual:
			run.conceptualQs++
		case modeCode:
			run.codeQs++
		case modeBug:
			run.bugQs++
		}
	}
	if !run.coachOK {
		run.failure = "coach reply missing the HR recruiter introduction"
	}
}

var (
	logSessionPattern = regexp.MustCompile(`session=(\S+)`)
	issuePattern      = regexp.MustCompile(`"([^"]+)"`)
	closeReason       = regexp.MustCompile(`mode=(\S+) move=CLOSE_MODE reason="([^"]*)"`)
	defectPattern     = regexp.MustCompile(`bug_defect session=\S+ defect="(.*)"`)
	labelsLine        = regexp.MustCompile(`\[d5\] labels session=\S+ mode=(\S+) labels="(.*)"`)
	labelEntry        = regexp.MustCompile(`#\d+ (\w+)=(\w+)`)
	selfAdmitPattern  = regexp.MustCompile(`(?i)\bactually\b|\bwait\b|no defect|is correct|works fine|that's fine|nothing is wrong`)
)

func pct(n, d int) string {
	if d == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d/%d (%.0f%%)", n, d, 100*float64(n)/float64(d))
}

func durPct(ds []time.Duration, p float64) float64 {
	if len(ds) == 0 {
		return 0
	}
	s := append([]time.Duration(nil), ds...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[int(float64(len(s)-1)*p)].Seconds()
}

func reportFullRuns(results []*fullRun, logs, transcriptPath string) {
	ids := map[string]*fullRun{}
	for _, r := range results {
		ids[truncateSessionID(r.id)] = r
	}
	type agg struct{ evalOK, evalAll, ready, waits, interviewerFail, levelsUnparsed, regenerated, outOfScope, defects, defectsBad int }
	var a agg
	issues := map[string]int{}
	closes := map[string]int{}
	runLabels := map[string]map[string]string{} // session → mode → labels
	// profile → dimension → level → count
	levelCounts := map[string]map[string]map[string]int{}
	for _, line := range strings.Split(logs, "\n") {
		m := logSessionPattern.FindStringSubmatch(line)
		if m == nil || ids[m[1]] == nil {
			continue
		}
		switch {
		case strings.Contains(line, "[d5] evaluator_done"):
			a.evalAll++
			if strings.Contains(line, "parsed=true") {
				a.evalOK++
			}
		case strings.Contains(line, "[d5] brief_wait"):
			a.waits++
			if strings.Contains(line, "ready_before_next=true") {
				a.ready++
			}
		case strings.Contains(line, "[d5] interviewer_failed"):
			a.interviewerFail++
		case strings.Contains(line, "[d5] levels_unparsed"):
			a.levelsUnparsed++
		case strings.Contains(line, "[d5] opening_check") && !strings.Contains(line, "problems=0 out_of_scope=[]"):
			a.regenerated++
			if !strings.Contains(line, "out_of_scope=[]") {
				a.outOfScope++
			}
		case strings.Contains(line, "[d5] postcheck"):
			if i := strings.Index(line, "issues=["); i >= 0 {
				for _, is := range issuePattern.FindAllStringSubmatch(line[i:], -1) {
					key := is[1]
					if j := strings.Index(key, ":"); j > 0 {
						key = key[:j]
					}
					issues[key]++
				}
			}
		case strings.Contains(line, "[d5] bug_defect"):
			a.defects++
			if d := defectPattern.FindStringSubmatch(line); d == nil || d[1] == "" || selfAdmitPattern.MatchString(d[1]) || len(d[1]) > 300 {
				a.defectsBad++
			}
		}
		if l := labelsLine.FindStringSubmatch(line); l != nil {
			r := ids[m[1]]
			if runLabels[m[1]] == nil {
				runLabels[m[1]] = map[string]string{}
			}
			runLabels[m[1]][l[1]] = l[2]
			for _, e := range labelEntry.FindAllStringSubmatch(l[2], -1) {
				if levelCounts[r.profile] == nil {
					levelCounts[r.profile] = map[string]map[string]int{}
				}
				if levelCounts[r.profile][e[1]] == nil {
					levelCounts[r.profile][e[1]] = map[string]int{}
				}
				levelCounts[r.profile][e[1]][e[2]]++
			}
		}
		if c := closeReason.FindStringSubmatch(line); c != nil {
			closes[modeTitle(c[1])+": "+c[2]]++
		}
	}

	completed := 0
	var times []time.Duration
	var turns []int
	gradeBy := map[string]map[string]int{}
	var b strings.Builder
	fmt.Fprintf(&b, "\n%-4s %-4s %-11s %-17s %-5s %-5s %-6s %-6s %-5s %-36s %s\n", "run", "week", "student", "major", "done", "turns", "C/Co/B", "pasted", "coach", "grades (conceptual/code/bug → overall)", "failure")
	for _, r := range results {
		if r.completed {
			completed++
		}
		times = append(times, r.replyTimes...)
		turns = append(turns, r.turns)
		if gradeBy[r.profile] == nil {
			gradeBy[r.profile] = map[string]int{}
		}
		if r.buckets[3] != "" {
			gradeBy[r.profile][r.buckets[3]]++
		}
		coach := "ok"
		if !r.coachOK {
			coach = "—"
		} else if r.coachHasCode {
			coach = "code!"
		}
		grades := fmt.Sprintf("%s/%s/%s → %s", short3(r.buckets[0]), short3(r.buckets[1]), short3(r.buckets[2]), r.buckets[3])
		fmt.Fprintf(&b, "%-4s %-4d %-11s %-17s %-5v %-5d %-6s %-6v %-5s %-36s %s\n", r.id[:3], r.week, r.profile, r.major, r.completed, r.turns,
			fmt.Sprintf("%d/%d/%d", r.conceptualQs, r.codeQs, r.bugQs), r.codePasted, coach, grades, r.failure)
	}
	fmt.Print(b.String())

	fmt.Printf("\n===== Summary over %d full runs\n", len(results))
	fmt.Printf("Completed (all three parts, results, coach): %s\n", pct(completed, len(results)))
	fmt.Printf("Reply time per turn (%d turns): p50 %.1fs · p90 %.1fs · p95 %.1fs · max %.1fs · over 10s: %d\n",
		len(times), durPct(times, 0.5), durPct(times, 0.9), durPct(times, 0.95), durPct(times, 1), countOver(times, 10*time.Second))
	sort.Ints(turns)
	if len(turns) > 0 {
		fmt.Printf("Student answers per run: min %d · median %d · max %d\n", turns[0], turns[len(turns)/2], turns[len(turns)-1])
	}
	fmt.Printf("Overall rating by student profile:\n")
	for _, p := range []string{"exceptional", "competent", "not_ready", "engineer"} {
		if gradeBy[p] == nil {
			continue
		}
		fmt.Printf("  %-11s Exceptional %d · Competent %d · Not Ready Yet %d\n", p, gradeBy[p][bucketExceptional], gradeBy[p][bucketCompetent], gradeBy[p][bucketNotReady])
	}
	fmt.Printf("Evaluator briefs parsed: %s · ready before the next answer: %s\n", pct(a.evalOK, a.evalAll), pct(a.ready, a.waits))
	fmt.Printf("Interviewer call failures (fallback question used): %d · levels-only replies unparsed: %d\n", a.interviewerFail, a.levelsUnparsed)
	fmt.Printf("Openings regenerated: %d (out-of-scope words: %d)\n", a.regenerated, a.outOfScope)
	fmt.Printf("Bug DEFECT lines: %d · missing or self-admitting: %d\n", a.defects, a.defectsBad)
	fmt.Printf("Why parts closed:\n")
	printCounts(closes)
	fmt.Printf("Post-check issues:\n")
	printCounts(issues)
	fmt.Printf("Labels by student profile and dimension (Exceptional / Competent / Not Ready):\n")
	for _, prof := range []string{"exceptional", "competent", "not_ready", "engineer"} {
		if levelCounts[prof] == nil {
			continue
		}
		fmt.Printf("  %s\n", prof)
		for _, dim := range []string{dimConceptual, dimDecomposition, dimCorrectness, dimUnderstanding, dimAIUse, dimStrategy} {
			c := levelCounts[prof][dim]
			if c == nil {
				continue
			}
			fmt.Printf("    %-14s E %2d · C %2d · NR %2d\n", dim, c[levelExceptional], c[levelCompetent], c[levelNotReady])
		}
	}
	fmt.Printf("Labels per run:\n")
	for _, r := range results {
		sid := truncateSessionID(r.id)
		fmt.Printf("  %s week %d %-11s", r.id[:3], r.week, r.profile)
		for _, mode := range []string{modeConceptual, modeCode, modeBug} {
			fmt.Printf(" | %s: %s", modeTitle(mode), compactLabels(runLabels[sid][mode]))
		}
		fmt.Println()
	}
	fmt.Printf("Transcripts: %s\n", transcriptPath)
}

func short3(bucket string) string {
	switch bucket {
	case bucketExceptional:
		return "E"
	case bucketCompetent:
		return "C"
	case bucketNotReady:
		return "NR"
	case bucketNA:
		return "NA"
	}
	return "-"
}

func countOver(ds []time.Duration, limit time.Duration) int {
	n := 0
	for _, d := range ds {
		if d > limit {
			n++
		}
	}
	return n
}

func printCounts(m map[string]int) {
	type kv struct {
		k string
		v int
	}
	var list []kv
	for k, v := range m {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })
	for _, e := range list {
		fmt.Printf("  %3d  %s\n", e.v, e.k)
	}
}

// compactLabels turns "#1 conceptual=competent (evaluator), …" into "C E C".
func compactLabels(labels string) string {
	var parts []string
	for _, e := range labelEntry.FindAllStringSubmatch(labels, -1) {
		lvl := map[string]string{levelExceptional: "E", levelCompetent: "C", levelNotReady: "NR"}[e[2]]
		if e[1] == dimConceptual || e[1] == dimStrategy {
			parts = append(parts, lvl)
		} else {
			parts = append(parts, map[string]string{dimDecomposition: "dec", dimCorrectness: "cor", dimUnderstanding: "und", dimAIUse: "ai"}[e[1]]+"="+lvl)
		}
	}
	return strings.Join(parts, " ")
}

// answerLength lets the exceptional student answer fully: the rubric's Exceptional asks for
// complete answers with an example or edge case, which 1-4 sentences did not allow.
func answerLength(profile string) string {
	switch profile {
	case "engineer":
		return "3-8 sentences (precise, with a concrete example or edge case)"
	case "exceptional":
		return "2-6 sentences (with a concrete example or edge case where it helps)"
	case "not_ready":
		return "1-2 short sentences"
	}
	return "1-4 sentences"
}
