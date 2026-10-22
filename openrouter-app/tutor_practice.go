package main

import (
	"context"
	"errors"
	"log"
	"regexp"
	"strings"
	"time"
)

// generatedProblem is a practice problem: what the student sees and the hidden answer key.
type generatedProblem struct {
	Title     string
	Problem   string
	Reference string
}

var (
	problemTitleLine     = regexp.MustCompile(`(?im)^[ \t>*#_]*TITLE[*_]*\s*:[*_]*[ \t]*(.+)$`)
	problemMarkerLine    = regexp.MustCompile(`(?im)^[ \t>*#_]*PROBLEM[*_]*\s*:[*_]*[ \t]*`)
	referenceMarkerLine  = regexp.MustCompile(`(?im)^[ \t>*#_]*(?:REFERENCE|ANSWER[ _]KEY)[*_]*\s*:[*_]*[ \t]*`)
	errProblemUnreadable = errors.New("problem reply not in the TITLE/PROBLEM/REFERENCE format")
)

// parseGeneratedProblem splits a TITLE / PROBLEM / REFERENCE reply.
func parseGeneratedProblem(content string) (generatedProblem, error) {
	var p generatedProblem
	pm := problemMarkerLine.FindStringIndex(content)
	rm := referenceMarkerLine.FindStringIndex(content)
	if pm == nil || rm == nil || rm[0] < pm[1] {
		return p, errProblemUnreadable
	}
	if m := problemTitleLine.FindStringSubmatch(content[:pm[0]]); m != nil {
		p.Title = strings.Trim(strings.TrimSpace(m[1]), "*_\"")
	}
	p.Problem = strings.TrimSpace(content[pm[1]:rm[0]])
	p.Reference = strings.TrimSpace(content[rm[1]:])
	if p.Problem == "" || p.Reference == "" {
		return p, errProblemUnreadable
	}
	if p.Title == "" {
		p.Title = firstWords(p.Problem, 8)
	}
	return p, nil
}

func firstWords(s string, n int) string {
	words := strings.Fields(strings.NewReplacer("*", "", "#", "", "`", "").Replace(s))
	if len(words) > n {
		words = words[:n]
	}
	return strings.Join(words, " ")
}

var pythonBlock = regexp.MustCompile("(?s)```python\\s*\\n.*?```")

// problemIssues lists what is wrong with a generated problem, as feedback for a rewrite.
func problemIssues(w *weekContent, kind string, p generatedProblem) []string {
	var issues []string
	if found := outOfScopeConcepts(w.Number, p.Problem); len(found) > 0 {
		issues = append(issues, "It used things the student has not learned yet: "+strings.Join(found, ", ")+". Write a different problem that needs none of them.")
	}
	lower := strings.ToLower(normalizeQuotes(p.Problem))
	switch {
	case w.Number == 1:
		if strings.Contains(p.Problem, "```") {
			issues = append(issues, "It contained code. Week 1 problems are everyday tasks with no code.")
		}
	case kind == kindCode:
		if strings.Contains(p.Problem, "`") {
			issues = append(issues, "It contained code. Describe the data and the result in plain words only.")
		}
		if !strings.Contains(lower, "data available") || !strings.Contains(lower, "what's wanted") {
			issues = append(issues, "It was missing the \"Data available:\" or \"What's wanted:\" line.")
		}
	case kind == kindBug:
		if !pythonBlock.MatchString(p.Problem) {
			issues = append(issues, "It had no program in a ```python block.")
		}
	}
	return issues
}

func normalizeQuotes(s string) string {
	return strings.NewReplacer("’", "'", "‘", "'", "“", "\"", "”", "\"").Replace(s)
}

// generateProblem writes a practice problem, checks it, and rewrites it once if it has
// problems, keeping whichever draft has fewer.
func (t *tutor) generateProblem(ctx context.Context, sessionID string, spec callSpec, w *weekContent, kind, major, difficulty string, avoid []string) (generatedProblem, error) {
	base := tutorBasePrompt(w, major)
	var best generatedProblem
	var bestIssues []string
	haveBest := false
	var fixes []string
	for attempt := 1; attempt <= 2; attempt++ {
		res, err := t.llm.call(ctx, spec, problemMessages(base, w, kind, major, difficulty, avoid, fixes))
		logCall(sessionID, spec, res, err)
		if err != nil {
			if haveBest {
				break
			}
			return generatedProblem{}, err
		}
		p, err := parseGeneratedProblem(res.Content)
		if err != nil {
			log.Printf("[tutor] problem_unreadable session=%s kind=%s raw=%q", shortID(sessionID), kind, truncate(res.Content, 200))
			fixes = []string{"It was not in the required TITLE / PROBLEM / REFERENCE format."}
			continue
		}
		issues := problemIssues(w, kind, p)
		log.Printf("[tutor] problem_check session=%s week=%d kind=%s attempt=%d issues=%q", shortID(sessionID), w.Number, kind, attempt, issues)
		if !haveBest || len(issues) <= len(bestIssues) {
			best, bestIssues, haveBest = p, issues, true
		}
		if len(issues) == 0 {
			break
		}
		fixes = issues
	}
	if !haveBest {
		return generatedProblem{}, errProblemUnreadable
	}
	return best, nil
}

// preparedProblem is a practice problem written in the background, with reasoning on,
// so the next one of its kind is ready when the student asks.
type preparedProblem struct {
	week    int
	major   string
	done    chan struct{}
	problem generatedProblem
	err     error
}

// prepareProblems starts background drafts for the week's practice formats that have none.
func (t *tutor) prepareProblems(sessionID string, sess *tutorSession, w *weekContent, major string, wp *weekProgress, kinds ...string) {
	if len(kinds) == 0 {
		kinds = w.practiceKinds()
	}
	sess.prepMu.Lock()
	defer sess.prepMu.Unlock()
	for _, kind := range kinds {
		if !w.hasPractice(kind) {
			continue
		}
		if p := sess.prepared[kind]; p != nil && p.week == w.Number && p.major == major {
			continue
		}
		var avoid []string
		if wp != nil {
			avoid = wp.recentTitles(kind, 6)
		}
		p := &preparedProblem{week: w.Number, major: major, done: make(chan struct{})}
		sess.prepared[kind] = p
		go func(kind string) {
			defer close(p.done)
			p.problem, p.err = t.generateProblem(context.Background(), sessionID, callPreparedProblem, w, kind, major, difficultyStandard, avoid)
		}(kind)
	}
}

// preparedWait is how long a request waits for a background draft still being written
// before writing a problem itself.
const preparedWait = 20 * time.Second

// takePrepared returns the prepared problem for kind, waiting briefly if it is still being
// written. It returns false when there is none to use.
func (sess *tutorSession) takePrepared(kind string, week int, major string) (generatedProblem, bool) {
	sess.prepMu.Lock()
	p := sess.prepared[kind]
	sess.prepMu.Unlock()
	if p == nil || p.week != week || p.major != major {
		return generatedProblem{}, false
	}
	select {
	case <-p.done:
	case <-time.After(preparedWait):
		return generatedProblem{}, false
	}
	sess.prepMu.Lock()
	if sess.prepared[kind] == p {
		delete(sess.prepared, kind)
	}
	sess.prepMu.Unlock()
	if p.err != nil {
		return generatedProblem{}, false
	}
	return p.problem, true
}

// clearPrepared drops background drafts, for a new week or student.
func (sess *tutorSession) clearPrepared() {
	sess.prepMu.Lock()
	sess.prepared = map[string]*preparedProblem{}
	sess.prepMu.Unlock()
}

var levelLine = regexp.MustCompile(`(?im)^[ \t>*_` + "`" + `]*LEVEL[*_]*\s*:\s*[*_` + "`" + `]*\s*(not[ _-]?ready(?:[ _-]yet)?|competent|exceptional|none)\b[^\n]*$`)

// splitLevel removes the hidden LEVEL line from a feedback reply and returns the level
// ("" for none or a missing line).
func splitLevel(content string) (string, string) {
	matches := levelLine.FindAllStringSubmatch(content, -1)
	visible := strings.TrimSpace(levelLine.ReplaceAllString(content, ""))
	if len(matches) == 0 {
		return visible, ""
	}
	switch word := strings.ToLower(matches[len(matches)-1][1]); {
	case strings.HasPrefix(word, "not"):
		return visible, levelNotReady
	case word == "competent":
		return visible, levelCompetent
	case word == "exceptional":
		return visible, levelExceptional
	}
	return visible, ""
}

var pythonStatementLine = regexp.MustCompile(`^\s*(?:def \w+\(|import \w|from \w+ import|(?:for|while|if|elif) .+:\s*$|else:\s*$|with .+:\s*$|return\b|print\(|[A-Za-z_]\w*(?:\[[^\]]*\])?\s*[-+*/]?=[^=])`)

// looksLikeCode reports pasted code, as opposed to a plan in prose ("if the total is over
// 100, apply the discount").
func looksLikeCode(msg string) bool {
	if strings.Contains(msg, "```") {
		return true
	}
	lines := 0
	for _, line := range strings.Split(msg, "\n") {
		if pythonStatementLine.MatchString(line) {
			lines++
		}
	}
	if lines >= 2 {
		return true
	}
	return lines == 1 && len(strings.Split(strings.TrimSpace(msg), "\n")) == 1 && strings.ContainsAny(msg, "()")
}
