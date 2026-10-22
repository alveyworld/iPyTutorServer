//go:build smoke

package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Temporary probe: re-labels the Conceptual answers of chosen full runs and asks why.
func TestZZProbeConceptualLabels(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	data, err := os.ReadFile(os.Getenv("PROBE_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	runs := regexp.MustCompile(`(?m)^########## `).Split(string(data), -1)
	want := strings.Split(os.Getenv("PROBE_RUNS"), ",")
	weekRe := regexp.MustCompile(`week (\d+)`)
	for _, run := range runs {
		id := strings.Fields(run + " x")[0]
		ok := false
		for _, w := range want {
			ok = ok || w == id
		}
		if !ok {
			continue
		}
		var week int
		fmt.Sscanf(weekRe.FindStringSubmatch(run)[1], "%d", &week)
		conceptual := strings.SplitN(run, "Thanks — that completes", 2)[0]
		blocks := regexp.MustCompile(`(?m)^\[(interviewer[^\]]*|student)\]\n`).Split(conceptual, -1)[1:]
		state, _ := d5TestState(week, modeConceptual)
		state.SelectedKeyConcept = weeklyKeyConceptSelections[week-1].SelectedKeyConcept
		for i := 0; i+1 < len(blocks); i += 2 {
			paras := strings.Split(strings.TrimSpace(blocks[i]), "\n\n")
			question := paras[len(paras)-1]
			answer := strings.TrimSpace(blocks[i+1])
			msgs := d5LevelsMessages(state, modeConceptual, dimConceptual, question, answer, "")
			var words []string
			for k := 0; k < 3; k++ {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				res, _ := d5Complete(ctx, apiKey, "probe", levelsRequest("", msgs))
				cancel()
				words = append(words, strings.TrimSpace(res.Content))
			}
			why := append([]chatMessage(nil), msgs...)
			why[0].Content = strings.Replace(why[0].Content, "Reply with exactly one word and nothing else: not_ready, competent or exceptional (or none if the question was not about this week's topic).", "Reply with the level (not_ready, competent, exceptional or none), then one or two sentences saying exactly why, naming what is missing for the next level up.", 1)
			req := levelsRequest("", why)
			req.MaxTokens = 200
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			res, _ := d5Complete(ctx, apiKey, "probe", req)
			cancel()
			fmt.Printf("== %s week %d Q%d labels=%v\nQ: %s\nWHY: %s\n\n", id, week, i/2+1, words, question, strings.TrimSpace(res.Content))
		}
	}
}
