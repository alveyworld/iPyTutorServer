package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ratingOrder = map[string]int{"Not Ready Yet": 0, "Competent": 1, "Exceptional": 2}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func sumCounts(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

func shortRating(s string) string {
	switch s {
	case "Not Ready Yet":
		return "NRY"
	case "Competent":
		return "C"
	case "Exceptional":
		return "E"
	case "N/A":
		return "NA"
	case "":
		return "-"
	}
	return s
}

func bucketString(r runResult) string {
	return fmt.Sprintf("%s/%s/%s", shortRating(r.Buckets["ConceptualUnderstanding"]),
		shortRating(r.Buckets["CodeProblem"]), shortRating(r.Buckets["BugHunting"]))
}

func direction(expected, got string) string {
	e, eok := ratingOrder[expected]
	g, gok := ratingOrder[got]
	switch {
	case !eok || !gok:
		return "unrated"
	case g > e:
		return "over-rated"
	case g < e:
		return "under-rated"
	}
	return "match"
}

func writeRun(outDir string, r runResult) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "runs", r.ID+".json"), data, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "runs", r.ID+".md"), []byte(runMarkdown(r)), 0o644)
}

func runMarkdown(r runResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — %s, week %d\n\n", r.ID, r.PersonaName, r.Week)
	fmt.Fprintf(&b, "- Expected: **%s**  Final: **%s**  Match: **%v**  Completed: %v\n", r.Expected, orDash(r.FinalRating), r.Match, r.Completed)
	fmt.Fprintf(&b, "- Buckets: conceptual=%s code=%s bug=%s\n", orDash(r.Buckets["ConceptualUnderstanding"]), orDash(r.Buckets["CodeProblem"]), orDash(r.Buckets["BugHunting"]))
	fmt.Fprintf(&b, "- Major: %s  Session: %s  Duration: %.0fs  Upstream calls: %d (failed %d)\n", r.Major, r.SessionID, r.DurationSec, r.UpstreamTotal, r.UpstreamFailed)
	if r.Error != "" {
		fmt.Fprintf(&b, "- Error: `%s`\n", r.Error)
	}
	if len(r.HandoffKinds) > 0 {
		fmt.Fprintf(&b, "- Corrective handoffs: %v\n", r.HandoffKinds)
	}
	for _, mm := range r.ModeMismatches {
		fmt.Fprintf(&b, "\n## Mismatch in %s: expected %s, got %s (%s)\n\n", mm.Mode, mm.Expected, orDash(mm.Got), direction(mm.Expected, mm.Got))
		fmt.Fprintf(&b, "- Bucket the model wrote: %s; graded on turn %d, upstream call #%d; %d student turns in mode; %d handoffs in mode\n",
			orDash(mm.ModelBucket), mm.GradingTurn, mm.GradingCallSeq, mm.TurnsInMode, len(mm.HandoffsInMode))
		if mm.ModelBucket != "" && mm.ModelBucket != mm.Got {
			fmt.Fprintf(&b, "- **Server state differs from the model's bucket** — check server-side normalization/override.\n")
		}
	}

	callBySeq := map[int]upstreamCall{}
	for _, c := range r.UpstreamCalls {
		callBySeq[c.Seq] = c
	}
	b.WriteString("\n## Transcript\n")
	for _, t := range r.Turns {
		fmt.Fprintf(&b, "\n### Turn %d — %s %s\n\n", t.Index, t.PhaseBefore, t.ModeBefore)
		fmt.Fprintf(&b, "**Student:**\n\n%s\n\n", quote(t.Student))
		fmt.Fprintf(&b, "**Interviewer (visible):**\n\n%s\n\n", quote(t.AssistantVisible))
		if t.Sync != nil {
			sync, _ := json.Marshal(t.Sync)
			fmt.Fprintf(&b, "Sync: `%s`\n\n", sync)
		}
		if t.State != nil {
			fmt.Fprintf(&b, "State after: mode=%s conceptual=%s/%s code=%s/%s bug=%s/%s final=%s vague=%v similarAsks=%v\n\n",
				str(t.State["activeMode"]),
				str(t.State["conceptualAssessmentPhase"]), str(t.State["conceptualAssessmentBucket"]),
				str(t.State["codeAssessmentPhase"]), str(t.State["codeAssessmentBucket"]),
				str(t.State["bugAssessmentPhase"]), str(t.State["bugAssessmentBucket"]),
				str(t.State["finalRating"]), t.State["modeVagueAnswers"], t.State["modeSimilarQuestionAsks"])
		}
		if t.Error != "" {
			fmt.Fprintf(&b, "Error: `%s`\n\n", t.Error)
		}
		for _, seq := range t.UpstreamSeqs {
			c := callBySeq[seq]
			fmt.Fprintf(&b, "<details><summary>Upstream #%d — status %d, %dms, system prompt <code>prompts/%s.md</code> (%d chars), %d msgs%s</summary>\n\n",
				c.Seq, c.Status, c.LatencyMs, c.SystemPromptSHA, c.SystemPromptChars, len(c.Messages), handoffLabel(c))
			for _, h := range c.Handoffs {
				fmt.Fprintf(&b, "Handoff injected:\n\n%s\n\n", quote(h))
			}
			if c.Error != "" {
				fmt.Fprintf(&b, "Error: `%s`\n\n", c.Error)
			}
			fmt.Fprintf(&b, "Raw response:\n\n%s\n\n</details>\n\n", quote(c.Response))
		}
	}
	return b.String()
}

func handoffLabel(c upstreamCall) string {
	if len(c.Handoffs) == 0 {
		return ""
	}
	kinds := make([]string, len(c.Handoffs))
	for i, h := range c.Handoffs {
		kinds[i] = handoffKind(h)
	}
	return ", handoff: " + strings.Join(kinds, "; ")
}

func quote(s string) string {
	if strings.TrimSpace(s) == "" {
		return "> _(empty)_"
	}
	return "> " + strings.ReplaceAll(s, "\n", "\n> ")
}

type tally struct {
	Runs, Matched, Incomplete int
	Got                       map[string]int
}

func (t *tally) add(r runResult) {
	if t.Got == nil {
		t.Got = map[string]int{}
	}
	t.Runs++
	if !r.Completed {
		t.Incomplete++
		return
	}
	if r.Match {
		t.Matched++
	}
	t.Got[shortRating(r.FinalRating)]++
}

func (t tally) row() string {
	pct := 0.0
	if t.Runs > 0 {
		pct = 100 * float64(t.Matched) / float64(t.Runs)
	}
	return fmt.Sprintf("%d | %d (%.0f%%) | %d | %d | %d | %d", t.Runs, t.Matched, pct, t.Got["NRY"], t.Got["C"], t.Got["E"], t.Incomplete)
}

func writeSummary(cfg config, results []runResult, total int) error {
	results = append([]runResult(nil), results...)
	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })

	var b strings.Builder
	fmt.Fprintf(&b, "# Persona simulation report — %s\n\n", time.Now().Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "**Progress: %d of %d assessments finished.** In-progress transcripts are in `runs/` and update after every turn.\n\n", len(results), total)
	fmt.Fprintf(&b, "Interviewer model (client request): `%s` · Student model: `%s` · Weeks: %v · Runs per persona per week: %d · Max student turns: %d\n\n",
		cfg.InterviewerModel, cfg.StudentModel, cfg.Weeks, cfg.Runs, cfg.MaxTurns)
	b.WriteString("Expected rating: Not Ready Yet student → Not Ready Yet; Competent → Competent; Exceptional → Exceptional; software engineer → Exceptional.\n\n")

	b.WriteString("## Final rating by persona\n\nPersona | Runs | Matched | Got NRY | Got C | Got E | Incomplete\n---|---|---|---|---|---|---\n")
	all := tally{}
	for _, p := range cfg.Personas {
		t := tally{}
		for _, r := range results {
			if r.Persona == p.Key {
				t.add(r)
				all.add(r)
			}
		}
		fmt.Fprintf(&b, "%s | %s\n", p.Name, t.row())
	}
	fmt.Fprintf(&b, "**All** | %s\n\n", all.row())

	b.WriteString("## Final rating by week and persona\n\nWeek | Persona | Runs | Matched | Got NRY | Got C | Got E | Incomplete\n---|---|---|---|---|---|---|---\n")
	for _, w := range cfg.Weeks {
		for _, p := range cfg.Personas {
			t := tally{}
			for _, r := range results {
				if r.Week == w && r.Persona == p.Key {
					t.add(r)
				}
			}
			fmt.Fprintf(&b, "%d | %s | %s\n", w, p.Name, t.row())
		}
	}

	b.WriteString("\n## Per-mode buckets vs expected (completed runs)\n\nPersona | Mode | Graded | Matched | Under-rated | Over-rated\n---|---|---|---|---|---\n")
	for _, p := range cfg.Personas {
		for _, f := range modeFields {
			graded, matched, under, over := 0, 0, 0, 0
			for _, r := range results {
				if r.Persona != p.Key || !r.Completed || (r.Week == 1 && f.Mode != "ConceptualUnderstanding") {
					continue
				}
				graded++
				switch direction(r.Expected, r.Buckets[f.Mode]) {
				case "match":
					matched++
				case "under-rated":
					under++
				case "over-rated":
					over++
				}
			}
			if graded > 0 {
				fmt.Fprintf(&b, "%s | %s | %d | %d | %d | %d\n", p.Name, f.Mode, graded, matched, under, over)
			}
		}
	}

	b.WriteString("\n## Process health\n\n")
	kinds := map[string]int{}
	kindsMismatch := map[string]int{}
	turns, calls, failed, mismatched := 0, 0, 0, 0
	for _, r := range results {
		turns += len(r.Turns)
		calls += r.UpstreamTotal
		failed += r.UpstreamFailed
		for k, v := range r.HandoffKinds {
			kinds[k] += v
			if r.Completed && !r.Match {
				kindsMismatch[k] += v
			}
		}
		if r.Completed && !r.Match {
			mismatched++
		}
	}
	if n := len(results); n > 0 {
		fmt.Fprintf(&b, "- Avg turns per run: %.1f · avg upstream calls per run: %.1f · failed upstream calls: %d\n", float64(turns)/float64(n), float64(calls)/float64(n), failed)
	}
	b.WriteString("- Corrective handoffs injected by the server (all runs / mismatched runs):\n\n")
	b.WriteString("Kind | All runs | Mismatched runs\n---|---|---\n")
	for _, k := range sortedKeys(kinds) {
		fmt.Fprintf(&b, "%s | %d | %d\n", k, kinds[k], kindsMismatch[k])
	}

	b.WriteString("\n## Mismatches\n\n")
	for _, r := range results {
		if r.Completed && r.Match {
			continue
		}
		status := "MISMATCH"
		if !r.Completed {
			status = "INCOMPLETE"
		}
		fmt.Fprintf(&b, "### [%s](runs/%s.md) — %s, expected %s, got %s (%s)\n\n", r.ID, r.ID, status, r.Expected, orDash(r.FinalRating), direction(r.Expected, r.FinalRating))
		fmt.Fprintf(&b, "- %s · week %d · major %s · buckets %s · %d turns · %d upstream calls · handoffs %v\n", r.PersonaName, r.Week, r.Major, bucketString(r), len(r.Turns), r.UpstreamTotal, r.HandoffKinds)
		if r.Error != "" {
			fmt.Fprintf(&b, "- Error: `%s`\n", r.Error)
		}
		for _, mm := range r.ModeMismatches {
			fmt.Fprintf(&b, "- **%s**: expected %s, got %s (%s); model wrote %s on turn %d (upstream #%d); %d student turns, %d handoffs in mode\n",
				mm.Mode, mm.Expected, orDash(mm.Got), direction(mm.Expected, mm.Got), orDash(mm.ModelBucket), mm.GradingTurn, mm.GradingCallSeq, mm.TurnsInMode, len(mm.HandoffsInMode))
			for _, h := range dedupe(mm.HandoffsInMode) {
				fmt.Fprintf(&b, "  - handoff: %s\n", handoffKind(h))
			}
			if mm.ModelBucket != "" && mm.ModelBucket != mm.Got {
				b.WriteString("  - **server state differs from the model's bucket**\n")
			}
			if len(mm.StudentAnswers) > 0 {
				fmt.Fprintf(&b, "  - last student answer: %s\n", oneLine(mm.StudentAnswers[len(mm.StudentAnswers)-1], 300))
			}
			if vis := visibleContent(mm.GraderReply); vis != "" {
				fmt.Fprintf(&b, "  - grader's closing reply: %s\n", oneLine(vis, 300))
			}
		}
		b.WriteString("\n")
	}

	if err := os.WriteFile(filepath.Join(cfg.OutDir, "summary.md"), []byte(b.String()), 0o644); err != nil {
		return err
	}

	type slim struct {
		ID, Persona, Expected, FinalRating, Error string
		Week                                      int
		Completed, Match                          bool
		Buckets                                   map[string]string
		Turns, UpstreamCalls                      int
		HandoffKinds                              map[string]int
		ModeMismatches                            []modeMismatch
	}
	var slims []slim
	for _, r := range results {
		slims = append(slims, slim{r.ID, r.Persona, r.Expected, r.FinalRating, r.Error, r.Week, r.Completed, r.Match, r.Buckets, len(r.Turns), r.UpstreamTotal, r.HandoffKinds, r.ModeMismatches})
	}
	data, _ := json.MarshalIndent(slims, "", "  ")
	if err := os.WriteFile(filepath.Join(cfg.OutDir, "summary.json"), data, 0o644); err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(cfg.OutDir, "results.csv"))
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"id", "persona", "week", "major", "expected", "final", "match", "completed", "conceptual", "code", "bug", "turns", "upstream_calls", "handoffs", "duration_sec", "error"})
	for _, r := range results {
		_ = w.Write([]string{r.ID, r.Persona, fmt.Sprint(r.Week), r.Major, r.Expected, r.FinalRating, fmt.Sprint(r.Match), fmt.Sprint(r.Completed),
			r.Buckets["ConceptualUnderstanding"], r.Buckets["CodeProblem"], r.Buckets["BugHunting"],
			fmt.Sprint(len(r.Turns)), fmt.Sprint(r.UpstreamTotal), fmt.Sprint(sumCounts(r.HandoffKinds)), fmt.Sprintf("%.0f", r.DurationSec), r.Error})
	}
	w.Flush()
	return w.Error()
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return m[keys[i]] > m[keys[j]] })
	return keys
}

func dedupe(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range items {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func oneLine(s string, n int) string {
	return truncate(strings.Join(strings.Fields(s), " "), n)
}
