//go:build smoke

package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestZZDefectCheckAccuracy(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	data, _ := os.ReadFile(os.Getenv("ZZ_REVIEW"))
	noBug := map[string]bool{"#6": true, "#18": true, "#36": true, "#42": true}
	blocks := regexp.MustCompile(`(?m)^########## `).Split(string(data), -1)
	tp, fn, tn, fp := 0, 0, 0, 0
	for _, blk := range blocks {
		if !strings.Contains(blk, "Bug hunting") {
			continue
		}
		id := strings.Fields(blk)[0]
		final := blk[strings.LastIndex(blk, "--- FINAL"):]
		final = final[strings.Index(final, "\n")+1:]
		parts := strings.SplitN(final, "[hidden] DEFECT:", 2)
		snippet, defect := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		var verdicts []string
		for i := 0; i < 3; i++ {
			res, _ := runBackgroundCall(apiKey, "zz", defectCheckRequest("", d5VerifyDefectMessages(snippet, defect)), d5LevelsTimeout*2, 1)
			v := strings.ToLower(strings.Trim(strings.TrimSpace(res.Content), "\"'*` "))
			flagged := strings.HasPrefix(v, "no")
			verdicts = append(verdicts, map[bool]string{true: "NO", false: "yes"}[flagged])
			switch {
			case noBug[id] && flagged:
				tp++
			case noBug[id] && !flagged:
				fn++
			case !noBug[id] && flagged:
				fp++
				fmt.Printf("   false alarm on %s: %q\n", id, truncateSummary(res.Content, 140))
			default:
				tn++
			}
		}
		fmt.Printf("%-4s my judgement: %-9s checker: %s\n", id, map[bool]string{true: "NO BUG", false: "real bug"}[noBug[id]], strings.Join(verdicts, " "))
	}
	fmt.Printf("\nNo-bug snippets caught: %d/%d · good snippets wrongly flagged: %d/%d\n", tp, tp+fn, fp, fp+tn)
}
