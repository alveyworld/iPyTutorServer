//go:build smoke

package main

// A live run of the D5 engine against the real OpenRouter, for checking reply quality and
// latency before staging. It is excluded from normal test runs:
//
//	OPENROUTER_API_KEY=... go test -tags smoke -run TestD5Smoke -v -timeout 15m
//
// D5_SMOKE_WEEK picks the week (default 5); D5_SMOKE_MAJOR the major.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

var smokeAnswers = map[string][]string{
	modeConceptual: {
		"I'd look at the value we're deciding on and compare it against each threshold, starting with the strictest one, so each case lands in exactly one group.",
		"The order matters because a high value also passes the lower checks, so if I checked the low threshold first everything would end up there.",
		"For a value exactly on the line I'd decide whether the boundary belongs to the higher or lower group and use >= or > to match that.",
		"If two conditions both have to hold, like a high score and a recent visit, I'd join them with and; if either is enough I'd use or.",
		"I'd add an else at the end to catch anything that didn't match the earlier checks, so nothing falls through without a result.",
	},
	"decomposition": {
		"First I'd get the inputs, then decide which rule applies with an if/elif chain from the highest threshold down, then calculate the result, and finally print it in a clear sentence.",
	},
	"code": {
		"```python\nscore = float(input('Score: '))\nif score >= 90:\n    level = 'high'\nelif score >= 70:\n    level = 'medium'\nelse:\n    level = 'low'\nprint(f'Level: {level}')\n```",
	},
	"explain": {
		"I used float because scores can have decimals, and I checked the highest threshold first so a 95 doesn't stop at the 70 check.",
		"I asked an AI assistant to remind me of f-string syntax, then I ran the script with 69, 70, 89 and 90 to make sure each boundary landed in the right level.",
		"The else branch catches everything under 70, so I didn't need a third comparison.",
	},
	modeBug: {
		"First I'd run it with a value I can work out by hand, like 60, and compare what it prints with what it should print.",
		"I'd add a print of the variable right before and after the if line to see which line changes it unexpectedly.",
		"If the prints look fine, I'd test values on each side of the threshold, like 49, 50 and 51, to see whether the comparison is the problem.",
		"I'd check whether each branch is reached by printing a short message inside each one, then narrow it down to the branch that misbehaves.",
	},
}

func TestD5Smoke(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}
	week := os.Getenv("D5_SMOKE_WEEK")
	if week == "" {
		week = "5"
	}
	major := os.Getenv("D5_SMOKE_MAJOR")
	if major == "" {
		major = "Nutrition Science"
	}
	saved := chatEngine
	chatEngine = "d5"
	defer func() { chatEngine = saved }()

	states, turns := newAgentStateStore(), newTurnStore()
	sessionID := "smoke-" + strconv.FormatInt(time.Now().Unix(), 10)
	var durations []time.Duration
	turn := 0
	say := func(text string) string {
		turn++
		body, _ := json.Marshal(chatCompletionRequest{Messages: []chatMessage{{Role: "user", Content: text}}})
		req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(string(body)))
		req = req.WithContext(context.WithValue(req.Context(), sessionIDContextKey, sessionID))
		req.Header.Set(turnIDHeader, fmt.Sprintf("smoke-%d", turn))
		rec := httptest.NewRecorder()
		started := time.Now()
		handleChat(apiKey, states, turns)(rec, req)
		elapsed := time.Since(started)
		reply := extractAssistantContent(rec.Body.Bytes())
		st, _ := states.get(sessionID)
		fmt.Printf("\n───── turn %d · %s · %s · %.1fs\nSTUDENT: %s\nINTERVIEWER:\n%s\n", turn, st.ConversationPhase, st.ActiveMode, elapsed.Seconds(), text, reply)
		if st.ConversationPhase == phaseAssessmentInProgress || st.ConversationPhase == phaseAssessmentResults {
			durations = append(durations, elapsed)
		}
		return reply
	}

	say(major)
	say(week)
	used := map[string]int{}
	next := func(key string) string {
		list := smokeAnswers[key]
		a := list[used[key]%len(list)]
		used[key]++
		return a
	}
	for i := 0; i < 30; i++ {
		st, _ := states.get(sessionID)
		if st.ConversationPhase != phaseAssessmentInProgress {
			break
		}
		// Give the background Evaluator the student's typing time, as a real student would.
		time.Sleep(8 * time.Second)
		switch st.ActiveMode {
		case modeCode:
			switch {
			case st.D5.CodePasted:
				say(next("explain"))
			case st.ModeInterviewStep == interviewStepAwaitingCode:
				say(next("code"))
			default:
				say(next("decomposition"))
			}
		default:
			say(next(st.ActiveMode))
		}
	}
	say("switch to coach mode")

	st, _ := states.get(sessionID)
	fmt.Printf("\n===== Buckets: conceptual=%s code=%s bug=%s overall=%s\n", st.ConceptualAssessmentBucket, st.CodeAssessmentBucket, st.BugAssessmentBucket, st.FinalRating)
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	if len(durations) > 0 {
		fmt.Printf("===== Reply times over %d assessment turns: p50 %.1fs, max %.1fs\n", len(durations), durations[len(durations)/2].Seconds(), durations[len(durations)-1].Seconds())
	}
	if st.ConversationPhase != phaseAssessmentResults {
		t.Errorf("interview did not reach results: phase %s", st.ConversationPhase)
	}
}
