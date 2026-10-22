//go:build smoke

package main

// Replay of the Bug-part labelling on recorded probe runs: every judged run's Bug
// conversation is sent to each labeller variant, and the level it returns is scored against
// the persona's expected level and against the probe judge's level for the answers.
//
//	go test -tags smoke -run TestReplayBug -v -timeout 60m
//
// Environment: REPLAY_VARIANTS (comma list, default "flash,sonnet,opus"), REPLAY_SINCE
// (default 2026-10-04T15:36), REPLAY_LIMIT (runs, 0 = all). The key comes from
// OPENROUTER_API_KEY, ~/.openrouter-env or ~/.openrouter.key, as the probe finds it.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

type replayRun struct {
	ID      string `json:"id"`
	Persona string `json:"persona"`
	Week    int    `json:"week"`
	Started string `json:"started"`
	Status  string `json:"status"`
	Buckets map[string]string
	Turns   []replayTurn `json:"turns"`
	Labels  []struct {
		AnswerIndex int    `json:"answerIndex"`
		Dimension   string `json:"dimension"`
		Level       string `json:"level"`
		Source      string `json:"source"`
	} `json:"labels"`
	Judgement struct {
		Answers []struct {
			Answer int    `json:"answer"`
			Level  string `json:"level"`
		} `json:"answers"`
	} `json:"judgement"`
}

type replayTurn struct {
	Index       int    `json:"index"`
	Mode        string `json:"mode"`
	Student     string `json:"student"`
	Interviewer string `json:"interviewer"`
}

var replayKeyPattern = regexp.MustCompile(`OPENROUTER_API_KEY=["']?([^"'\s]+)`)

func replayKey(t *testing.T) string {
	if k := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")); k != "" {
		return k
	}
	home, _ := os.UserHomeDir()
	if data, err := os.ReadFile(filepath.Join(home, ".openrouter-env")); err == nil {
		if m := replayKeyPattern.FindSubmatch(data); m != nil {
			return string(m[1])
		}
	}
	if data, err := os.ReadFile(filepath.Join(home, ".openrouter.key")); err == nil {
		if k := strings.TrimSpace(string(data)); k != "" {
			return k
		}
	}
	t.Skip("no OpenRouter key")
	return ""
}

// replayBugSession rebuilds the Bug part of a recorded run: the opening (the interviewer
// reply that ended the Code part), then each Bug answer and the reply to it, leaving out the
// results message. judgeLevels are the probe judge's levels for the Bug answers.
func replayBugSession(r replayRun) (sess *d5Session, judgeLevels []string, ok bool) {
	last := -1
	for i, tr := range r.Turns {
		if tr.Mode == modeCode {
			last = i
		}
	}
	if last < 0 {
		return nil, nil, false
	}
	sess = newD5Session("replay")
	sess.Transcript = append(sess.Transcript, d5Message{Role: "assistant", Content: r.Turns[last].Interviewer, Mode: modeBug})
	students := 0
	for _, tr := range r.Turns[last+1:] {
		if tr.Mode == modeCoaching || tr.Student == "" {
			break
		}
		sess.Transcript = append(sess.Transcript, d5Message{Role: "user", Content: tr.Student, Mode: modeBug})
		students++
		if strings.Contains(tr.Interviewer, "Assessment Results") {
			break
		}
		sess.Transcript = append(sess.Transcript, d5Message{Role: "assistant", Content: tr.Interviewer, Mode: modeBug})
	}
	n := len(r.Judgement.Answers)
	if students < 2 || n < students {
		return nil, nil, false
	}
	for _, a := range r.Judgement.Answers[n-students:] {
		judgeLevels = append(judgeLevels, a.Level)
	}
	return sess, judgeLevels, true
}

// majorityLevel mirrors the engine's combining rule: the most common level, ties to the higher.
func majorityLevel(levels []string) string {
	count := map[string]int{}
	for _, l := range levels {
		count[l]++
	}
	best := ""
	for _, l := range []string{levelNotReady, levelCompetent, levelExceptional} {
		if count[l] > 0 && (best == "" || count[l] >= count[best]) {
			best = l
		}
	}
	return best
}

var replayLevelWord = regexp.MustCompile(`not[_ ]ready|competent|exceptional`)

// replayParse reads the level word: the first one in a direct answer, the last one when the
// model reasoned in its reply.
func replayParse(content string, reasoned bool) string {
	all := replayLevelWord.FindAllString(strings.ToLower(content), -1)
	if len(all) == 0 {
		return ""
	}
	w := all[0]
	if reasoned {
		w = all[len(all)-1]
	}
	return strings.ReplaceAll(w, " ", "_")
}

const replayStrictRule = `Rate the strategy as a whole across the candidate's answers, but be strict: the default is competent. Award exceptional only when the answers together clearly show at least three of these: reproduces and predicts the expected values before checking; names several candidate causes and says what would confirm or rule each out; chooses test values deliberately, including boundary cases; narrows the cause step by step and revises when evidence contradicts; explains precisely how the concept's behaviour produces the symptom. Award not_ready when there is no real plan or the answers stay vague.`

type replayVariant struct {
	name      string
	model     string
	reasoning bool
	strict    bool
}

func replayVariants() []replayVariant {
	all := map[string]replayVariant{
		"flash":    {name: "flash", model: ""},
		"flash-s":  {name: "flash-s", model: "", strict: true},
		"sonnet":   {name: "sonnet", model: "anthropic/claude-sonnet-5.5"},
		"sonnet-s": {name: "sonnet-s", model: "anthropic/claude-sonnet-5.5", strict: true},
		"sonnet-r": {name: "sonnet-r", model: "anthropic/claude-sonnet-5.5", reasoning: true},
		"opus":     {name: "opus", model: "anthropic/claude-opus-5.5"},
		"opus-r":   {name: "opus-r", model: "anthropic/claude-opus-5.5", reasoning: true},
	}
	list := os.Getenv("REPLAY_VARIANTS")
	if list == "" {
		list = "flash,sonnet,opus"
	}
	var out []replayVariant
	for _, n := range strings.Split(list, ",") {
		if v, ok := all[strings.TrimSpace(n)]; ok {
			if m := os.Getenv("REPLAY_MODEL_" + strings.ToUpper(strings.ReplaceAll(v.name, "-", "_"))); m != "" {
				v.model = m
			}
			out = append(out, v)
		}
	}
	return out
}

func (v replayVariant) request(msgs []chatMessage) d5Request {
	if v.strict {
		msgs = append([]chatMessage(nil), msgs...)
		msgs[0].Content = strings.Replace(msgs[0].Content,
			"read all of the candidate's answers together and rate the strategy they showed as a whole. A later short or narrow answer does not cancel what an earlier one showed.",
			replayStrictRule, 1)
	}
	// Claude models answer in a sentence unless the last thing they read asks for one word.
	msgs = append([]chatMessage(nil), msgs...)
	msgs[len(msgs)-1].Content += "\n\nAnswer with only one word: not_ready, competent or exceptional."
	req := levelsRequest(v.model, msgs)
	if v.model != "" {
		req.Provider = nil // the throughput/require_parameters routing is for the default model
	}
	// The Claude 5.5 endpoints cannot run without reasoning, so those variants differ in effort.
	if v.model != "" {
		effort := "low"
		if v.reasoning {
			effort = "high"
		}
		req.Reasoning = map[string]any{"effort": effort}
		req.MaxTokens = 2500
	}
	return req
}

func expectedBug(persona string) string {
	switch persona {
	case "not_ready":
		return levelNotReady
	case "competent":
		return levelCompetent
	}
	return levelExceptional
}

type replayOutcome struct {
	run     replayRun
	variant string
	got     string
	persona string
	judge   string
	ms      int64
	tokens  int
	err     string
}

func TestReplayModelIDs(t *testing.T) {
	key := replayKey(t)
	for _, v := range replayVariants() {
		if v.model == "" {
			continue
		}
		res, err := runBackgroundCall(key, "replay", v.request([]chatMessage{{Role: "user", Content: "Reply with exactly one word: ok"}}), 30*time.Second, 1)
		fmt.Printf("%-9s model=%s reply=%q err=%v\n", v.name, v.model, strings.TrimSpace(res.Content), err)
	}
}

func TestReplayBug(t *testing.T) {
	key := replayKey(t)
	since := os.Getenv("REPLAY_SINCE")
	if since == "" {
		since = "2026-10-04T15:36"
	}
	files, _ := filepath.Glob("../probes/persona-assessment/results/runs/*.json")
	sort.Strings(files)
	var runs []replayRun
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var r replayRun
		if json.Unmarshal(data, &r) != nil || r.Status != "judged" || r.Started < since {
			continue
		}
		if only := os.Getenv("REPLAY_PERSONAS"); only != "" && !strings.Contains(","+only+",", ","+r.Persona+",") {
			continue
		}
		runs = append(runs, r)
	}
	if n := 0; os.Getenv("REPLAY_LIMIT") != "" {
		fmt.Sscanf(os.Getenv("REPLAY_LIMIT"), "%d", &n)
		if n > 0 && n < len(runs) {
			runs = runs[:n]
		}
	}
	variants := replayVariants()
	var jobs []func() replayOutcome
	for _, r := range runs {
		for _, v := range variants {
			r, v := r, v
			sess, judgeLevels, ok := replayBugSession(r)
			if !ok || r.Week < 1 || r.Week > 9 {
				continue
			}
			state, _ := d5TestState(r.Week, modeBug)
			state.SelectedKeyConcept = weeklyKeyConceptSelections[r.Week-1].SelectedKeyConcept
			msgs := d5BugStrategyMessages(state, sess)
			jobs = append(jobs, func() replayOutcome {
				out := replayOutcome{run: r, variant: v.name, persona: expectedBug(r.Persona), judge: majorityLevel(judgeLevels)}
				res, err := runBackgroundCall(key, "replay", v.request(msgs), 90*time.Second, 2)
				out.ms = res.Elapsed.Milliseconds()
				out.tokens = res.Usage.PromptTokens + res.Usage.CompletionTokens
				if err != nil {
					out.err = err.Error()
					return out
				}
				out.got = replayParse(res.Content, v.reasoning)
				return out
			})
		}
	}
	fmt.Printf("replaying %d calls (%d runs, variants %d)\n", len(jobs), len(runs), len(variants))
	results := make([]replayOutcome, len(jobs))
	var wg sync.WaitGroup
	parallel := 6
	if v := os.Getenv("REPLAY_PARALLEL"); v != "" {
		fmt.Sscanf(v, "%d", &parallel)
	}
	sem := make(chan struct{}, parallel)
	for i, job := range jobs {
		wg.Add(1)
		go func(i int, job func() replayOutcome) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = job()
		}(i, job)
	}
	wg.Wait()
	replayReport(results, runs)
}

func replayReport(results []replayOutcome, runs []replayRun) {
	type tally struct{ n, errs, persona, judge, over, under, sumMs, tokens int }
	rank := map[string]int{levelNotReady: 0, levelCompetent: 1, levelExceptional: 2}
	by := map[string]map[string]*tally{} // variant -> persona|ALL
	add := func(variant, key string, o replayOutcome) {
		if by[variant] == nil {
			by[variant] = map[string]*tally{}
		}
		tl := by[variant][key]
		if tl == nil {
			tl = &tally{}
			by[variant][key] = tl
		}
		tl.n++
		tl.sumMs += int(o.ms)
		tl.tokens += o.tokens
		if o.err != "" || o.got == "" {
			tl.errs++
			return
		}
		if o.got == o.persona {
			tl.persona++
		} else if rank[o.got] > rank[o.persona] {
			tl.over++
		} else {
			tl.under++
		}
		if o.got == o.judge {
			tl.judge++
		}
	}
	for _, o := range results {
		add(o.variant, o.run.Persona, o)
		add(o.variant, "ALL", o)
	}
	// Reference rows: what the engine's recorded Bug ratings were in the live sweeps.
	live := map[string]map[string]*tally{}
	for _, r := range runs {
		_, judgeLevels, ok := replayBugSession(r)
		if !ok {
			continue
		}
		got := map[string]string{"Exceptional": levelExceptional, "Competent": levelCompetent, "Not Ready Yet": levelNotReady}[r.Buckets["BugHunting"]]
		if got == "" {
			continue
		}
		name := "live-old(per-answer)"
		if r.Started >= "2026-10-05T02:10" {
			name = "live-new(holistic flash)"
		}
		o := replayOutcome{run: r, variant: name, got: got, persona: expectedBug(r.Persona), judge: majorityLevel(judgeLevels)}
		add(o.variant, o.run.Persona, o)
		add(o.variant, "ALL", o)
	}
	_ = live
	var names []string
	for n := range by {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Printf("\n%-26s %-11s %4s %8s %8s %6s %6s %7s %9s\n", "variant", "persona", "n", "persona%", "judge%", "over", "under", "errors", "avg ms")
	for _, n := range names {
		for _, p := range []string{"not_ready", "competent", "exceptional", "engineer", "ALL"} {
			tl := by[n][p]
			if tl == nil {
				continue
			}
			ok := tl.n - tl.errs
			if ok == 0 {
				ok = 1
			}
			fmt.Printf("%-26s %-11s %4d %7d%% %7d%% %6d %6d %7d %9d\n", n, p, tl.n, 100*tl.persona/ok, 100*tl.judge/ok, tl.over, tl.under, tl.errs, tl.sumMs/maxInt(tl.n, 1))
		}
	}
	for _, n := range names {
		if tl := by[n]["ALL"]; tl != nil && tl.tokens > 0 {
			fmt.Printf("%s: %d tokens in total\n", n, tl.tokens)
		}
	}
	for _, o := range results {
		if o.err != "" {
			fmt.Printf("error %s %s: %s\n", o.variant, o.run.ID, truncateSummary(o.err, 160))
			break
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ---- per-answer labels (Conceptual and Code) ----

type replayAnswer struct {
	run      replayRun
	index    int
	mode     string
	target   string
	question string
	answer   string
	judge    string
	live     string // the label the engine recorded for this dimension ("" if none)
}

var replayAIWord = regexp.MustCompile(`\bAI\b`)

// replayAnswers lists a run's Conceptual answers and its Code answers other than the code
// paste. Answer a is the student message of turn a+1; the question it replied to is the
// interviewer's reply in turn a. A Code answer's target is the planning answer's decomposition,
// then AI use when the question mentions AI, otherwise understanding.
func replayAnswers(r replayRun) []replayAnswer {
	judged := map[int]string{}
	for _, a := range r.Judgement.Answers {
		judged[a.Answer] = a.Level
	}
	var out []replayAnswer
	codeSeen := 0
	for a := 1; a+1 < len(r.Turns); a++ {
		tr := r.Turns[a+1]
		if judged[a] == "" || tr.Student == "" {
			continue
		}
		question := lastQuestionSentence(r.Turns[a].Interviewer)
		if question == "" {
			question = r.Turns[a].Interviewer
		}
		ans := replayAnswer{run: r, index: a, mode: tr.Mode, question: question, answer: tr.Student, judge: judged[a]}
		switch tr.Mode {
		case modeConceptual:
			ans.target = dimConceptual
		case modeCode:
			codeSeen++
			switch {
			case codeSeen == 1:
				ans.target = dimDecomposition
			case codeSeen == 2:
				continue // the code paste: correctness is a pass/fail check, not replayed here
			case replayAIWord.MatchString(r.Turns[a].Interviewer):
				ans.target = dimAIUse
			default:
				ans.target = dimUnderstanding
			}
		default:
			continue
		}
		for _, l := range r.Labels {
			if l.AnswerIndex == a && l.Dimension == ans.target {
				ans.live = l.Level
			}
		}
		out = append(out, ans)
	}
	return out
}

func TestReplayAnswers(t *testing.T) {
	key := replayKey(t)
	since := os.Getenv("REPLAY_SINCE")
	if since == "" {
		since = "2026-10-04T15:36"
	}
	files, _ := filepath.Glob("../probes/persona-assessment/results/runs/*.json")
	sort.Strings(files)
	byPersona := map[string][]replayRun{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var r replayRun
		if json.Unmarshal(data, &r) != nil || r.Status != "judged" || r.Started < since || r.Week < 1 || r.Week > 9 {
			continue
		}
		byPersona[r.Persona] = append(byPersona[r.Persona], r)
	}
	perPersona := 20
	if v := os.Getenv("REPLAY_PER_PERSONA"); v != "" {
		fmt.Sscanf(v, "%d", &perPersona)
	}
	var runs []replayRun
	for _, rs := range byPersona {
		sort.Slice(rs, func(i, j int) bool {
			if rs[i].Week != rs[j].Week {
				return rs[i].Week < rs[j].Week
			}
			return rs[i].ID < rs[j].ID
		})
		step := float64(len(rs)) / float64(perPersona)
		for k := 0; k < perPersona && k < len(rs); k++ {
			runs = append(runs, rs[int(float64(k)*step)])
		}
	}
	type job struct {
		a       replayAnswer
		variant replayVariant
	}
	var jobs []job
	variants := replayVariants()
	for _, r := range runs {
		for _, a := range replayAnswers(r) {
			for _, v := range variants {
				jobs = append(jobs, job{a, v})
			}
		}
	}
	fmt.Printf("replaying %d calls (%d runs, %d variants)\n", len(jobs), len(runs), len(variants))
	type result struct {
		job
		got string
		ms  int64
		err string
	}
	results := make([]result, len(jobs))
	parallel := 6
	if v := os.Getenv("REPLAY_PARALLEL"); v != "" {
		fmt.Sscanf(v, "%d", &parallel)
	}
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		go func(i int, j job) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			state, _ := d5TestState(j.a.run.Week, j.a.mode)
			state.SelectedKeyConcept = weeklyKeyConceptSelections[j.a.run.Week-1].SelectedKeyConcept
			v := j.variant
			v.strict = false
			msgs := d5LevelsMessages(state, j.a.mode, j.a.target, j.a.question, j.a.answer, "")
			res, err := runBackgroundCall(key, "replay", v.request(msgs), 90*time.Second, 2)
			r := result{job: j, ms: res.Elapsed.Milliseconds()}
			if err != nil {
				r.err = err.Error()
			} else {
				r.got = replayParse(res.Content, false)
			}
			results[i] = r
		}(i, j)
	}
	wg.Wait()

	rank := map[string]int{levelNotReady: 0, levelCompetent: 1, levelExceptional: 2}
	type tally struct{ n, errs, judge, over, under, ms int }
	cells := map[string]*tally{}
	add := func(key string, got, judge string, ms int64, isErr bool) {
		tl := cells[key]
		if tl == nil {
			tl = &tally{}
			cells[key] = tl
		}
		tl.n++
		tl.ms += int(ms)
		if isErr || got == "" {
			tl.errs++
			return
		}
		switch {
		case got == judge:
			tl.judge++
		case rank[got] > rank[judge]:
			tl.over++
		default:
			tl.under++
		}
	}
	seenLive := map[string]bool{}
	for _, r := range results {
		add(r.variant.name+"|dim|"+r.a.target, r.got, r.a.judge, r.ms, r.err != "")
		add(r.variant.name+"|dim|ALL", r.got, r.a.judge, r.ms, r.err != "")
		add(r.variant.name+"|persona|"+r.a.run.Persona, r.got, r.a.judge, r.ms, r.err != "")
		k := fmt.Sprintf("%s/%d/%s", r.a.run.ID, r.a.index, r.a.target)
		if r.a.live != "" && !seenLive[k] {
			seenLive[k] = true
			add("live (recorded)|dim|"+r.a.target, r.a.live, r.a.judge, 0, false)
			add("live (recorded)|dim|ALL", r.a.live, r.a.judge, 0, false)
			add("live (recorded)|persona|"+r.a.run.Persona, r.a.live, r.a.judge, 0, false)
		}
	}
	names := []string{"live (recorded)"}
	for _, v := range variants {
		names = append(names, v.name)
	}
	fmt.Printf("\nAgreement with the judge's level for the answer (over = label above the judge's)\n")
	fmt.Printf("%-16s %-14s %5s %9s %5s %6s %6s %8s\n", "variant", "slice", "n", "judge%", "over", "under", "errors", "avg ms")
	for _, kind := range []string{"dim", "persona"} {
		slices := []string{dimConceptual, dimDecomposition, dimUnderstanding, dimAIUse, "ALL"}
		if kind == "persona" {
			slices = []string{"not_ready", "competent", "exceptional", "engineer"}
		}
		for _, n := range names {
			for _, sl := range slices {
				tl := cells[n+"|"+kind+"|"+sl]
				if tl == nil {
					continue
				}
				ok := maxInt(tl.n-tl.errs, 1)
				fmt.Printf("%-16s %-14s %5d %8d%% %5d %6d %6d %8d\n", n, sl, tl.n, 100*tl.judge/ok, tl.over, tl.under, tl.errs, tl.ms/maxInt(tl.n, 1))
			}
		}
		fmt.Println()
	}
}
