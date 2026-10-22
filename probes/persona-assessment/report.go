package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// rate is a count of matches out of n, reported with a Wilson 95% interval so repeated
// sessions show how much each figure can still move.
type rate struct{ hit, n int }

func (r *rate) add(ok bool) {
	r.n++
	if ok {
		r.hit++
	}
}

func (r rate) String() string {
	if r.n == 0 {
		return "–"
	}
	const z = 1.96
	n, p := float64(r.n), float64(r.hit)/float64(r.n)
	den := 1 + z*z/n
	mid := (p + z*z/(2*n)) / den
	half := z * math.Sqrt(p*(1-p)/n+z*z/(4*n*n)) / den
	return fmt.Sprintf("%d/%d = %.0f%% (95%% CI %.0f–%.0f%%)", r.hit, r.n, 100*p, 100*math.Max(0, mid-half), 100*math.Min(1, mid+half))
}

// dirCount tallies engine levels against a reference level.
type dirCount struct{ match, under, over int }

func (d *dirCount) add(got, want string) {
	switch g, w := levelRank(got), levelRank(want); {
	case g == w:
		d.match++
	case g < w:
		d.under++
	default:
		d.over++
	}
}

func (d dirCount) row() string {
	r := rate{d.match, d.match + d.under + d.over}
	return fmt.Sprintf("%d | %s | %d | %d", r.n, r, d.under, d.over)
}

func loadRuns() ([]*runRecord, error) {
	files, _ := filepath.Glob(filepath.Join(runsDir(), "*.json"))
	var runs []*runRecord
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var r runRecord
		if err := json.Unmarshal(data, &r); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		runs = append(runs, &r)
	}
	sort.Slice(runs, func(i, k int) bool { return runs[i].Started.Before(runs[k].Started) })
	return runs, nil
}

func cmdReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	out := fs.String("out", filepath.Join(resultsDir(), "report.md"), "report file")
	player := fs.String("player", "", "only count runs played by this model (default: all)")
	since := fs.String("since", "", "only count runs started at or after this local time, e.g. 2026-10-03T19:30")
	fs.Parse(args)
	runs, err := loadRuns()
	if err != nil {
		return err
	}
	if *since != "" {
		cutoff, err := time.ParseInLocation("2006-01-02T15:04", *since, time.Local)
		if err != nil {
			return fmt.Errorf("-since: %w", err)
		}
		var kept []*runRecord
		for _, r := range runs {
			if !r.Started.Before(cutoff) {
				kept = append(kept, r)
			}
		}
		runs = kept
	}
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	var judged []*runRecord
	status := map[string]int{}
	for _, r := range runs {
		status[r.Status]++
		if r.Status == "judged" && (*player == "" || playerOf(r) == *player) {
			judged = append(judged, r)
		}
	}
	w("# Persona assessment probe report\n\n")
	w("Generated %s from %d runs (%d judged", time.Now().Format("2006-01-02 15:04"), len(runs), len(judged))
	for _, s := range []string{"coaching", "results", "in_progress", "failed"} {
		if status[s] > 0 {
			w(", %d %s", status[s], s)
		}
	}
	w("). Only judged runs are counted below. Percentages carry a Wilson 95%% confidence interval; ")
	w("run more sessions to narrow them.\n\n")
	w("Expected levels: not_ready → Not Ready Yet; competent → Competent; exceptional and engineer → Exceptional. ")
	w("Correctness is pass/fail in the engine, so working code from the competent persona is expected to be labelled exceptional there.\n\n")
	if *player != "" {
		w("**Only runs played and judged by %s are counted.**\n\n", *player)
	}
	if *since != "" {
		w("**Only runs started at or after %s are counted.**\n\n", *since)
	}

	// Runs per persona and week.
	w("## Judged runs by persona and week\n\nPersona | ")
	for wk := 1; wk <= 9; wk++ {
		w("W%d | ", wk)
	}
	w("Total\n---|")
	for wk := 1; wk <= 9; wk++ {
		w("---|")
	}
	w("---\n")
	for _, p := range personaOrder {
		w("%s | ", p)
		total := 0
		for wk := 1; wk <= 9; wk++ {
			n := 0
			for _, r := range judged {
				if r.Persona == p && r.Week == wk {
					n++
				}
			}
			total += n
			w("%d | ", n)
		}
		w("%d\n", total)
	}

	// Overall rating.
	w("\n## Overall rating vs persona\n\nPersona | Runs | Matched persona | Got NRY | Got C | Got E | Matched judge's overall\n---|---|---|---|---|---|---\n")
	all, allJudge := rate{}, rate{}
	for _, p := range personaOrder {
		var m, mj rate
		got := map[string]int{}
		for _, r := range judged {
			if r.Persona != p {
				continue
			}
			lvl := bucketLevel(r.Overall)
			got[lvl]++
			m.add(lvl == r.Expected)
			mj.add(lvl == r.Judgement.Overall.Level)
			all.add(lvl == r.Expected)
			allJudge.add(lvl == r.Judgement.Overall.Level)
		}
		w("%s | %d | %s | %d | %d | %d | %s\n", p, m.n, m, got[levelNotReady], got[levelCompetent], got[levelExceptional], mj)
	}
	w("**All** | %d | %s | | | | %s\n", all.n, all, allJudge)

	// Overall rating by the model that played the persona: a weaker player can drift off
	// the persona's level, so compare players before trusting a persona's figures.
	w("\n## Overall rating by player model\n\nPersona | Player | Runs | Matched persona | Matched judge's overall | Answers on persona level\n---|---|---|---|---|---\n")
	for _, p := range personaOrder {
		byPlayer := map[string][]*runRecord{}
		var players []string
		for _, r := range judged {
			if r.Persona == p {
				if byPlayer[playerOf(r)] == nil {
					players = append(players, playerOf(r))
				}
				byPlayer[playerOf(r)] = append(byPlayer[playerOf(r)], r)
			}
		}
		sort.Strings(players)
		for _, pl := range players {
			var m, mj, fid rate
			for _, r := range byPlayer[pl] {
				lvl := bucketLevel(r.Overall)
				m.add(lvl == r.Expected)
				mj.add(lvl == r.Judgement.Overall.Level)
				for _, a := range r.Judgement.Answers {
					fid.add(a.OnPersona)
				}
			}
			w("%s | %s | %d | %s | %s | %s\n", p, pl, m.n, m, mj, fid)
		}
	}

	writeByPersonaAndPlayer(w, judged)

	// Part buckets.
	w("\n## Part ratings vs persona\n\nPersona | Part | Graded | Matched | Under-rated | Over-rated\n---|---|---|---|---|---\n")
	for _, p := range personaOrder {
		for _, mode := range []string{modeConceptual, modeCode, modeBug} {
			var d dirCount
			for _, r := range judged {
				if r.Persona == p {
					if lvl := bucketLevel(r.Buckets[mode]); lvl != "" {
						d.add(lvl, r.Expected)
					}
				}
			}
			if d.match+d.under+d.over > 0 {
				w("%s | %s | %s\n", p, modeTitle(mode), d.row())
			}
		}
	}

	// Per-answer labels.
	type key struct{ persona, mode, dim string }
	vsPersona, vsJudge := map[key]*dirCount{}, map[key]*dirCount{}
	fidelity := map[string]*rate{}
	var keys []key
	unjudgedLabels := 0
	for _, r := range judged {
		ja := map[int]answerJudgement{}
		for _, a := range r.Judgement.Answers {
			ja[a.Answer] = a
			if fidelity[r.Persona] == nil {
				fidelity[r.Persona] = &rate{}
			}
			fidelity[r.Persona].add(a.OnPersona)
		}
		for _, l := range r.Labels {
			k := key{r.Persona, l.Mode, l.Dimension}
			if vsPersona[k] == nil {
				vsPersona[k], vsJudge[k] = &dirCount{}, &dirCount{}
				keys = append(keys, k)
			}
			vsPersona[k].add(l.Level, expectedFor(r.Expected, l.Dimension))
			if a, ok := ja[l.AnswerIndex]; ok {
				vsJudge[k].add(l.Level, comparable(a.Level, l.Dimension))
			} else {
				unjudgedLabels++
			}
		}
	}
	sort.Slice(keys, func(i, k int) bool {
		a, c := keys[i], keys[k]
		if a.persona != c.persona {
			return personaRank(a.persona) < personaRank(c.persona)
		}
		if a.mode != c.mode {
			return modeRank(a.mode) < modeRank(c.mode)
		}
		return a.dim < c.dim
	})
	w("\n## Per-answer labels\n\nEach row counts the engine's labels for one dimension. \"vs persona\" compares with the level the persona was told to answer at; ")
	w("\"vs judge\" compares with the judge's rubric level for the answer as actually given. Where the two disagree, check persona fidelity below before blaming the engine.\n\n")
	w("Persona | Part | Dimension | Labels | Matched persona | Under | Over | Matched judge | Under | Over\n---|---|---|---|---|---|---|---|---|---\n")
	for _, k := range keys {
		vj := vsJudge[k]
		rj := rate{vj.match, vj.match + vj.under + vj.over}
		w("%s | %s | %s | %s | %s | %d | %d\n", k.persona, modeTitle(k.mode), k.dim, vsPersona[k].row(), rj, vj.under, vj.over)
	}
	if unjudgedLabels > 0 {
		w("\n%d labels had no judge level (answer not judged).\n", unjudgedLabels)
	}

	w("\n## Persona fidelity\n\nShare of answers the judge found really were at the persona's level. Low fidelity means the simulation, not the engine, needs work.\n\nPersona | Answers on level\n---|---\n")
	for _, p := range personaOrder {
		if f := fidelity[p]; f != nil {
			w("%s | %s\n", p, *f)
		}
	}

	// Coaching.
	w("\n## Coach mode\n\nPersona | Coach replies appropriate | Runs: accurate | Specific | In scope | Helpful\n---|---|---|---|---|---\n")
	issues := map[string]int{}
	for _, p := range personaOrder {
		var app, acc, spec, scope, help rate
		for _, r := range judged {
			if r.Persona != p {
				continue
			}
			for _, c := range r.Judgement.Coach {
				app.add(c.Appropriate)
				for _, is := range c.Issues {
					issues[strings.ToLower(strings.TrimSpace(is))]++
				}
			}
			co := r.Judgement.CoachOverall
			acc.add(co.Accurate)
			spec.add(co.Specific)
			scope.add(co.InScope)
			help.add(co.Helpful)
		}
		if app.n+acc.n > 0 {
			w("%s | %s | %s | %s | %s | %s\n", p, app, acc, spec, scope, help)
		}
	}
	if len(issues) > 0 {
		w("\nCoach issues reported (count):\n\n")
		for _, is := range sortedByCount(issues) {
			w("- %s (%d)\n", is, issues[is])
		}
	}

	// Mismatch details.
	w("\n## Where the engine disagreed with the judge\n\nRun | Answer | Part | Dimension | Engine | Judge | Persona | Judge's note\n---|---|---|---|---|---|---|---\n")
	for _, r := range judged {
		ja := map[int]answerJudgement{}
		for _, a := range r.Judgement.Answers {
			ja[a.Answer] = a
		}
		for _, l := range r.Labels {
			a, ok := ja[l.AnswerIndex]
			if !ok || comparable(a.Level, l.Dimension) == l.Level {
				continue
			}
			w("%s | #%d | %s | %s | %s | %s | %s | %s\n", r.ID, l.AnswerIndex, modeTitle(l.Mode), l.Dimension, l.Level, a.Level, r.Persona, oneLine(a.Note, 160))
		}
	}

	w("\n## Overall rating by week\n\nWeek | Runs | Matched persona\n---|---|---\n")
	for wk := 1; wk <= 9; wk++ {
		var m rate
		for _, r := range judged {
			if r.Week == wk {
				m.add(bucketLevel(r.Overall) == r.Expected)
			}
		}
		if m.n > 0 {
			w("%d | %d | %s\n", wk, m.n, m)
		}
	}

	if err := os.WriteFile(*out, []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Printf("%d runs (%d judged). Overall matched persona: %s. Report: %s\n", len(runs), len(judged), all, *out)
	return nil
}

// writeByPersonaAndPlayer writes one row per persona and player model: overall and part
// ratings, per-answer label accuracy, persona fidelity and coach quality, so a player
// model's slips can be told apart from the engine's.
func writeByPersonaAndPlayer(w func(string, ...any), judged []*runRecord) {
	w("\n## By persona and player model\n\n")
	w("Part ratings and labels are compared with the persona's level; \"labels vs judge\" compares with the judge's level for the answer as given.\n\n")
	w("Persona | Player | Runs | Overall matched | Got NRY/C/E | Conceptual | Code | Bug | Labels vs persona | Labels vs judge | Over / under (labels) | On persona level | Coach replies OK | Coach runs accurate\n")
	w("---|---|---|---|---|---|---|---|---|---|---|---|---|---\n")
	for _, p := range personaOrder {
		byPlayer := map[string][]*runRecord{}
		var players []string
		for _, r := range judged {
			if r.Persona == p {
				if byPlayer[playerOf(r)] == nil {
					players = append(players, playerOf(r))
				}
				byPlayer[playerOf(r)] = append(byPlayer[playerOf(r)], r)
			}
		}
		sort.Strings(players)
		for _, pl := range players {
			runs := byPlayer[pl]
			var overall, fid, coachOK, coachAcc rate
			got := map[string]int{}
			parts := map[string]*rate{modeConceptual: {}, modeCode: {}, modeBug: {}}
			var vsPersona, vsJudge dirCount
			for _, r := range runs {
				lvl := bucketLevel(r.Overall)
				got[lvl]++
				overall.add(lvl == r.Expected)
				for mode, rt := range parts {
					if l := bucketLevel(r.Buckets[mode]); l != "" {
						rt.add(l == r.Expected)
					}
				}
				ja := map[int]answerJudgement{}
				for _, a := range r.Judgement.Answers {
					ja[a.Answer] = a
					fid.add(a.OnPersona)
				}
				for _, l := range r.Labels {
					vsPersona.add(l.Level, expectedFor(r.Expected, l.Dimension))
					if a, ok := ja[l.AnswerIndex]; ok {
						vsJudge.add(l.Level, comparable(a.Level, l.Dimension))
					}
				}
				for _, c := range r.Judgement.Coach {
					coachOK.add(c.Appropriate)
				}
				coachAcc.add(r.Judgement.CoachOverall.Accurate)
			}
			lp := rate{vsPersona.match, vsPersona.match + vsPersona.under + vsPersona.over}
			lj := rate{vsJudge.match, vsJudge.match + vsJudge.under + vsJudge.over}
			w("%s | %s | %d | %s | %d/%d/%d | %s | %s | %s | %s | %s | %d over, %d under | %s | %s | %s\n",
				p, pl, len(runs), overall, got[levelNotReady], got[levelCompetent], got[levelExceptional],
				*parts[modeConceptual], *parts[modeCode], *parts[modeBug], lp, lj, vsPersona.over, vsPersona.under, fid, coachOK, coachAcc)
		}
	}
}

// playerOf is the model that played a run; runs from before the player was recorded
// show as "unrecorded".
func playerOf(r *runRecord) string {
	if r.Player == "" {
		return "unrecorded"
	}
	return r.Player
}

func personaRank(p string) int {
	for i, k := range personaOrder {
		if k == p {
			return i
		}
	}
	return len(personaOrder)
}

func modeRank(m string) int {
	return map[string]int{modeConceptual: 0, modeCode: 1, modeBug: 2}[m]
}

func sortedByCount(m map[string]int) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, k int) bool {
		if m[out[i]] != m[out[k]] {
			return m[out[i]] > m[out[k]]
		}
		return out[i] < out[k]
	})
	return out
}

func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.ReplaceAll(s, "|", "/")
	return truncate(s, n)
}
