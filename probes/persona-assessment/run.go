package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	phaseInProgress = "AssessmentInProgress"
	phaseResults    = "AssessmentResults"
	modeConceptual  = "ConceptualUnderstanding"
	modeCode        = "CodeProblem"
	modeBug         = "BugHunting"
	modeCoaching    = "Coaching"

	levelNotReady    = "not_ready"
	levelCompetent   = "competent"
	levelExceptional = "exceptional"

	maxStudentTurns = 45
	enoughCoaching  = 3
)

var personaExpected = map[string]string{
	"not_ready":   levelNotReady,
	"competent":   levelCompetent,
	"exceptional": levelExceptional,
	"engineer":    levelExceptional,
}

var personaOrder = []string{"not_ready", "competent", "exceptional", "engineer"}

var majors = []string{"Nursing", "Accounting", "Biology", "Graphic Design", "Agriculture", "Music",
	"Exercise Science", "History", "Business", "Psychology", "Mechanical Engineering", "Economics"}

type turn struct {
	Index       int    `json:"index"`
	Phase       string `json:"phase"` // before the message
	Mode        string `json:"mode"`  // before the message
	Student     string `json:"student"`
	Interviewer string `json:"interviewer"`
	LatencyMs   int64  `json:"latencyMs"`
	// AnswerIndex is the engine's answer number for this message (0 when the engine did
	// not record it as an answer, e.g. setup, clarification or coaching).
	AnswerIndex int    `json:"answerIndex,omitempty"`
	AnswerMode  string `json:"answerMode,omitempty"`
	Error       string `json:"error,omitempty"`
}

type label struct {
	AnswerIndex int    `json:"answerIndex"`
	Mode        string `json:"mode"`
	Dimension   string `json:"dimension"`
	Level       string `json:"level"`
	Source      string `json:"source"`
}

type answerJudgement struct {
	Answer    int    `json:"answer"`
	Level     string `json:"level"`     // the judge's rubric level for the answer as given
	OnPersona bool   `json:"onPersona"` // the answer really was at the persona's level
	Note      string `json:"note,omitempty"`
}

type coachJudgement struct {
	Turn        int      `json:"turn"`
	Appropriate bool     `json:"appropriate"`
	Issues      []string `json:"issues,omitempty"`
}

type judgement struct {
	Answers      []answerJudgement `json:"answers"`
	Coach        []coachJudgement  `json:"coach"`
	CoachOverall struct {
		Accurate bool   `json:"accurate"` // consistent with the results; doesn't change them
		Specific bool   `json:"specific"` // refers to this candidate's actual answers
		InScope  bool   `json:"inScope"`  // interview skills, no code tutoring or internals
		Helpful  bool   `json:"helpful"`  // concrete, actionable, right tone for this persona
		Note     string `json:"note,omitempty"`
	} `json:"coachOverall"`
	Overall struct {
		Level string `json:"level"` // the judge's overall rating for the performance as given
		Note  string `json:"note,omitempty"`
	} `json:"overall"`
	Judged time.Time `json:"judged"`
}

type runRecord struct {
	ID       string `json:"id"`
	Persona  string `json:"persona"`
	Expected string `json:"expected"`
	Week     int    `json:"week"`
	Major    string `json:"major"`
	// Player is the model that played the student and judged the run.
	Player    string            `json:"player,omitempty"`
	SessionID string            `json:"sessionId"`
	Token     string            `json:"token"`
	Server    string            `json:"server"`
	LogPath   string            `json:"logPath"`
	Started   time.Time         `json:"started"`
	Updated   time.Time         `json:"updated"`
	Status    string            `json:"status"` // in_progress, results, coaching, judged, failed
	Error     string            `json:"error,omitempty"`
	Phase     string            `json:"phase"`
	Mode      string            `json:"mode"`
	Buckets   map[string]string `json:"buckets,omitempty"` // by mode
	Overall   string            `json:"overall,omitempty"`
	History   []chatMessage     `json:"history"`
	Turns     []turn            `json:"turns"`
	Labels    []label           `json:"labels,omitempty"`
	Judgement *judgement        `json:"judgement,omitempty"`
}

func runPath(id string) string { return filepath.Join(runsDir(), id+".json") }

func loadRun(id string) (*runRecord, error) {
	if id == "" {
		return nil, errors.New("-run is required")
	}
	data, err := os.ReadFile(runPath(id))
	if err != nil {
		return nil, fmt.Errorf("no run %q", id)
	}
	var r runRecord
	return &r, json.Unmarshal(data, &r)
}

func (r *runRecord) save() error {
	r.Updated = time.Now()
	data, _ := json.MarshalIndent(r, "", "  ")
	tmp := runPath(r.ID) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, runPath(r.ID))
}

func (r *runRecord) studentTurns() int {
	n := 0
	for _, t := range r.Turns {
		if t.Phase == phaseInProgress {
			n++
		}
	}
	return n
}

func (r *runRecord) coachTurns() []turn {
	var out []turn
	for _, t := range r.Turns {
		if r.isCoachReply(t) {
			out = append(out, t)
		}
	}
	return out
}

// isCoachReply reports whether a turn after the results got a coaching reply: the session
// was in Coaching after it.
func (r *runRecord) isCoachReply(t turn) bool {
	if t.Phase != phaseResults || t.Error != "" {
		return false
	}
	if t.Index+1 < len(r.Turns) {
		return r.Turns[t.Index+1].Mode == modeCoaching
	}
	return r.Mode == modeCoaching
}

func readStdin() (string, error) {
	data, err := io.ReadAll(bufio.NewReader(os.Stdin))
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", errors.New("empty message on stdin")
	}
	return text, nil
}

func cmdStart(args []string) error {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	persona := fs.String("persona", "", "not_ready, competent, exceptional or engineer")
	week := fs.Int("week", 0, "week 1-9")
	major := fs.String("major", "", "student's major (default: random)")
	player := fs.String("player", "", "model playing and judging this run, e.g. haiku or sonnet")
	fs.Parse(args)

	expected, ok := personaExpected[*persona]
	if !ok {
		return fmt.Errorf("unknown -persona %q (want %s)", *persona, strings.Join(personaOrder, ", "))
	}
	if *week < 1 || *week > 9 {
		return errors.New("-week must be 1-9")
	}
	if *major == "" {
		*major = majors[rand.IntN(len(majors))]
	}
	info, err := loadServerInfo()
	if err != nil {
		return err
	}
	pub, err := loadPublicKey(filepath.Join(info.ServerDir, "env", "ipyintervu-pub.pem"))
	if err != nil {
		return err
	}
	c := newAPIClient(info.Base, "")
	sessionID, err := c.login(pub)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	welcome, err := c.bootstrap()
	if err != nil {
		return err
	}
	r := &runRecord{
		ID:      fmt.Sprintf("%s-w%d-%s-%04x", *persona, *week, time.Now().Format("0102-150405"), rand.IntN(0x10000)),
		Persona: *persona, Expected: expected, Week: *week, Major: *major, Player: *player,
		SessionID: sessionID, Token: c.token, Server: info.Base, LogPath: info.LogPath,
		Started: time.Now(), Status: "in_progress",
		History: []chatMessage{{Role: "user", Content: "start"}, {Role: "assistant", Content: welcome}},
	}
	if err := os.MkdirAll(runsDir(), 0o755); err != nil {
		return err
	}
	for _, setup := range []string{*major, strconv.Itoa(*week)} {
		if _, err := r.send(c, setup); err != nil {
			r.Status, r.Error = "failed", err.Error()
			_ = r.save()
			return fmt.Errorf("setup %q: %w", setup, err)
		}
	}
	if err := r.save(); err != nil {
		return err
	}
	dir := info.ServerDir
	fmt.Printf("RUN %s\npersona: %s (expected %s) · week %d · major %s\n", r.ID, *persona, expected, *week, *major)
	fmt.Printf("persona prompt: %s\nweek rubric:    %s\nkey concepts:   %s\n\n",
		filepath.Join(probeDir(), "personas", *persona+".md"),
		filepath.Join(dir, "env", "rubrics", fmt.Sprintf("week%d_rubric.md", *week)),
		filepath.Join(dir, "env", "IPYIntervu_support_files", fmt.Sprintf("week%d_key_concepts.md", *week)))
	r.printLast()
	return nil
}

// send posts one student message and records the turn, the engine's answer number for
// it, and the state afterwards.
func (r *runRecord) send(c *apiClient, text string) (*turn, error) {
	t := turn{Index: len(r.Turns), Phase: r.Phase, Mode: r.Mode, Student: text}
	if t.Phase == "" {
		t.Phase = "Setup"
	}
	before := answerLines(r.LogPath, r.SessionID)
	r.History = append(r.History, chatMessage{Role: "user", Content: text})
	started := time.Now()
	raw, err := c.chat(newTurnID(), r.History)
	t.LatencyMs = time.Since(started).Milliseconds()
	if err != nil {
		r.History = r.History[:len(r.History)-1]
		t.Error = err.Error()
		r.Turns = append(r.Turns, t)
		return &t, err
	}
	t.Interviewer = visibleContent(raw)
	r.History = append(r.History, chatMessage{Role: "assistant", Content: t.Interviewer})
	if after := answerLines(r.LogPath, r.SessionID); len(after) > len(before) {
		a := after[len(after)-1]
		t.AnswerIndex, t.AnswerMode = a.index, a.mode
	}
	r.Turns = append(r.Turns, t)

	st, err := c.state()
	if err != nil {
		return &t, fmt.Errorf("state: %w", err)
	}
	r.Phase, r.Mode = str(st["conversationPhase"]), str(st["activeMode"])
	if r.Phase == phaseResults {
		r.Buckets = map[string]string{
			modeConceptual: str(st["conceptualAssessmentBucket"]),
			modeCode:       str(st["codeAssessmentBucket"]),
			modeBug:        str(st["bugAssessmentBucket"]),
		}
		r.Overall = str(st["finalRating"])
		if len(r.Labels) == 0 {
			r.Labels = parseLabels(r.LogPath, r.SessionID)
		}
		if r.Status == "in_progress" {
			r.Status = "results"
		}
		if r.Mode == modeCoaching {
			r.Status = "coaching"
		}
	}
	return &t, nil
}

func cmdSay(args []string) error {
	fs := flag.NewFlagSet("say", flag.ExitOnError)
	id := fs.String("run", "", "run ID")
	fs.Parse(args)
	r, err := loadRun(*id)
	if err != nil {
		return err
	}
	switch r.Status {
	case "judged":
		return errors.New("this run is already judged")
	case "failed":
		return fmt.Errorf("this run failed: %s", r.Error)
	}
	text, err := readStdin()
	if err != nil {
		return err
	}
	c := newAPIClient(r.Server, r.Token)
	t, err := r.send(c, text)
	if err == nil && r.Phase == phaseInProgress && r.studentTurns() >= maxStudentTurns {
		r.Status, r.Error = "failed", fmt.Sprintf("assessment did not finish within %d answers", maxStudentTurns)
	}
	if serr := r.save(); serr != nil {
		return serr
	}
	if err != nil {
		return fmt.Errorf("%w (nothing was recorded for this message; you can send it again)", err)
	}
	if t.AnswerIndex > 0 {
		fmt.Printf("(recorded as answer #%d, %s)\n\n", t.AnswerIndex, modeTitle(t.AnswerMode))
	}
	r.printLast()
	return nil
}

// printLast prints the interviewer's latest reply and what to do next.
func (r *runRecord) printLast() {
	last := r.Turns[len(r.Turns)-1]
	who := "INTERVIEWER"
	if r.Mode == modeCoaching {
		who = "COACH"
	}
	fmt.Printf("%s (turn %d, %.1fs):\n%s\n\n", who, last.Index, float64(last.LatencyMs)/1000, last.Interviewer)
	switch {
	case r.Status == "failed":
		fmt.Printf("[RUN FAILED] %s\n", r.Error)
	case r.Phase == phaseInProgress:
		fmt.Printf("[part: %s] NEXT: reply in persona with `probe say -run %s`.\n", modeTitle(r.Mode), r.ID)
	case r.Mode != modeCoaching:
		fmt.Printf("[ASSESSMENT FINISHED: overall %s] NEXT: ask for coaching in persona; the message must contain the word \"coaching\" (e.g. \"Could I get some coaching on how I did?\").\n", r.Overall)
	default:
		n := len(r.coachTurns())
		if n < enoughCoaching {
			fmt.Printf("[coach reply %d of %d] NEXT: ask a natural follow-up in persona about the feedback.\n", n, enoughCoaching)
		} else {
			fmt.Printf("[coach reply %d] Enough coaching. NEXT: judge the run (PLAYBOOK.md, Judging) with `probe judge -run %s`.\n", n, r.ID)
		}
	}
}

func cmdShow(args []string) error {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	id := fs.String("run", "", "run ID")
	fs.Parse(args)
	r, err := loadRun(*id)
	if err != nil {
		return err
	}
	fmt.Printf("RUN %s · %s (expected %s) · week %d · %s · status %s\n\n", r.ID, r.Persona, r.Expected, r.Week, r.Major, r.Status)
	for _, t := range r.Turns {
		tag := t.Phase
		if t.AnswerIndex > 0 {
			tag = fmt.Sprintf("answer #%d, %s", t.AnswerIndex, modeTitle(t.AnswerMode))
		} else if r.isCoachReply(t) {
			tag = "to coach"
		}
		fmt.Printf("── turn %d · STUDENT (%s)\n%s\n── turn %d · REPLY\n%s\n\n", t.Index, tag, t.Student, t.Index, t.Interviewer)
	}
	if r.Overall != "" {
		fmt.Printf("RESULTS: conceptual=%s code=%s bug=%s overall=%s\n", r.Buckets[modeConceptual], r.Buckets[modeCode], r.Buckets[modeBug], r.Overall)
	}
	if r.Judgement != nil {
		r.printComparison()
	}
	return nil
}

func cmdJudge(args []string) error {
	fs := flag.NewFlagSet("judge", flag.ExitOnError)
	id := fs.String("run", "", "run ID")
	fs.Parse(args)
	r, err := loadRun(*id)
	if err != nil {
		return err
	}
	if r.Phase != phaseResults {
		return errors.New("the assessment has not finished yet")
	}
	text, err := readStdin()
	if err != nil {
		return err
	}
	var j judgement
	dec := json.NewDecoder(strings.NewReader(text))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&j); err != nil {
		return fmt.Errorf("judgement JSON: %w", err)
	}
	if err := r.validate(&j); err != nil {
		return err
	}
	if len(r.Labels) == 0 {
		r.Labels = parseLabels(r.LogPath, r.SessionID)
	}
	j.Judged = time.Now()
	r.Judgement = &j
	r.Status = "judged"
	if err := r.save(); err != nil {
		return err
	}
	fmt.Printf("Judgement saved for %s.\n\n", r.ID)
	r.printComparison()
	return nil
}

func (r *runRecord) validate(j *judgement) error {
	answers := map[int]bool{}
	for _, t := range r.Turns {
		if t.AnswerIndex > 0 {
			answers[t.AnswerIndex] = true
		}
	}
	seen := map[int]bool{}
	for _, a := range j.Answers {
		if !answers[a.Answer] {
			return fmt.Errorf("answer #%d is not an answer in this run (see `probe show`)", a.Answer)
		}
		if levelRank(a.Level) < 0 {
			return fmt.Errorf("answer #%d: level %q must be not_ready, competent or exceptional", a.Answer, a.Level)
		}
		seen[a.Answer] = true
	}
	var missing []string
	for idx := range answers {
		if !seen[idx] {
			missing = append(missing, "#"+strconv.Itoa(idx))
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("judge every answer; missing %s", strings.Join(missing, ", "))
	}
	coach := map[int]bool{}
	for _, t := range r.coachTurns() {
		coach[t.Index] = true
	}
	for _, c := range j.Coach {
		if !coach[c.Turn] {
			return fmt.Errorf("turn %d has no coach reply", c.Turn)
		}
		delete(coach, c.Turn)
	}
	if len(coach) > 0 {
		return fmt.Errorf("judge every coach reply; %d not judged", len(coach))
	}
	if levelRank(j.Overall.Level) < 0 {
		return errors.New("overall.level must be not_ready, competent or exceptional")
	}
	return nil
}

// printComparison shows, per answer, the engine's labels next to the persona's level and
// the judge's level. It is shown only after judging so the judge isn't anchored by it.
func (r *runRecord) printComparison() {
	j := r.Judgement
	judged := map[int]answerJudgement{}
	for _, a := range j.Answers {
		judged[a.Answer] = a
	}
	fmt.Printf("Correctness is pass/fail: working code counts as exceptional.\n\nAnswer  Part         Dimension      Engine        Judge         Persona\n")
	for _, l := range r.Labels {
		a := judged[l.AnswerIndex]
		fmt.Printf("#%-6d %-12s %-14s %-13s %-13s %s\n", l.AnswerIndex, shortMode(l.Mode), l.Dimension, l.Level, comparable(a.Level, l.Dimension), expectedFor(r.Expected, l.Dimension))
	}
	fmt.Printf("\nBuckets: conceptual=%s code=%s bug=%s · overall %s (persona expects %s, judge says %s)\n",
		r.Buckets[modeConceptual], r.Buckets[modeCode], r.Buckets[modeBug], r.Overall, bucketName(r.Expected), bucketName(j.Overall.Level))
}

type answerLine struct {
	index int
	mode  string
}

var (
	answerLinePattern = regexp.MustCompile(`\[d5\] answer session=(\S+) mode=(\S+) answer_index=(\d+)`)
	labelsLinePattern = regexp.MustCompile(`\[d5\] labels session=(\S+) mode=(\S+) labels="(.*)"`)
	labelPattern      = regexp.MustCompile(`#(\d+) (\w+)=(\w+) \((\w+)\)`)
)

func shortSession(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func answerLines(logPath, sessionID string) []answerLine {
	data, _ := os.ReadFile(logPath)
	short := shortSession(sessionID)
	var out []answerLine
	for _, m := range answerLinePattern.FindAllStringSubmatch(string(data), -1) {
		if m[1] == short {
			n, _ := strconv.Atoi(m[3])
			out = append(out, answerLine{index: n, mode: m[2]})
		}
	}
	return out
}

// parseLabels reads the engine's final per-answer labels, which it logs just before the
// results are shown.
func parseLabels(logPath, sessionID string) []label {
	data, _ := os.ReadFile(logPath)
	short := shortSession(sessionID)
	var out []label
	for _, m := range labelsLinePattern.FindAllStringSubmatch(string(data), -1) {
		if m[1] != short {
			continue
		}
		for _, l := range labelPattern.FindAllStringSubmatch(m[3], -1) {
			n, _ := strconv.Atoi(l[1])
			out = append(out, label{AnswerIndex: n, Mode: m[2], Dimension: l[2], Level: l[3], Source: l[4]})
		}
	}
	sort.SliceStable(out, func(i, k int) bool { return out[i].AnswerIndex < out[k].AnswerIndex })
	return out
}

func levelRank(level string) int {
	switch level {
	case levelNotReady:
		return 0
	case levelCompetent:
		return 1
	case levelExceptional:
		return 2
	}
	return -1
}

// expectedFor is the label a persona's answer should get on a dimension. Correctness is
// pass/fail in the engine (code that works is labelled exceptional), so a competent
// persona's working code expects exceptional there.
func expectedFor(personaLevel, dimension string) string {
	if dimension == "correctness" && personaLevel == levelCompetent {
		return levelExceptional
	}
	return personaLevel
}

// comparable maps a judge level onto the engine's scale for a dimension.
func comparable(level, dimension string) string {
	if dimension == "correctness" && level == levelCompetent {
		return levelExceptional
	}
	return level
}

func bucketName(level string) string {
	switch level {
	case levelNotReady:
		return "Not Ready Yet"
	case levelCompetent:
		return "Competent"
	case levelExceptional:
		return "Exceptional"
	}
	return level
}

func bucketLevel(bucket string) string {
	switch bucket {
	case "Not Ready Yet":
		return levelNotReady
	case "Competent":
		return levelCompetent
	case "Exceptional":
		return levelExceptional
	}
	return ""
}

func modeTitle(mode string) string {
	switch mode {
	case modeConceptual:
		return "Conceptual"
	case modeCode:
		return "Code"
	case modeBug:
		return "Bug"
	case modeCoaching:
		return "Coaching"
	case "":
		return "setup"
	}
	return mode
}

func shortMode(mode string) string { return modeTitle(mode) }

func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
