package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type job struct {
	ID      string
	Persona persona
	Week    int
	Concept string
	Major   string
}

type turnRecord struct {
	Index            int            `json:"index"`
	PhaseBefore      string         `json:"phaseBefore"`
	ModeBefore       string         `json:"modeBefore"`
	Student          string         `json:"student"`
	AssistantRaw     string         `json:"assistantRaw"`
	AssistantVisible string         `json:"assistantVisible"`
	Sync             map[string]any `json:"sync,omitempty"`
	Attempts         int            `json:"attempts"`
	LatencyMs        int64          `json:"latencyMs"`
	Error            string         `json:"error,omitempty"`
	UpstreamSeqs     []int          `json:"upstreamSeqs,omitempty"`
	Handoffs         []string       `json:"handoffs,omitempty"`
	State            map[string]any `json:"state,omitempty"`
}

type modeMismatch struct {
	Mode           string   `json:"mode"`
	Expected       string   `json:"expected"`
	Got            string   `json:"got"`
	ModelBucket    string   `json:"modelBucket"`
	GradingTurn    int      `json:"gradingTurn"`
	GradingCallSeq int      `json:"gradingCallSeq"`
	TurnsInMode    int      `json:"turnsInMode"`
	HandoffsInMode []string `json:"handoffsInMode,omitempty"`
	StudentAnswers []string `json:"studentAnswers"`
	GraderReply    string   `json:"graderReply"`
}

type runResult struct {
	ID             string            `json:"id"`
	Persona        string            `json:"persona"`
	PersonaName    string            `json:"personaName"`
	Expected       string            `json:"expected"`
	Week           int               `json:"week"`
	Concept        string            `json:"concept"`
	Major          string            `json:"major"`
	SessionID      string            `json:"sessionId"`
	Started        time.Time         `json:"started"`
	DurationSec    float64           `json:"durationSec"`
	Completed      bool              `json:"completed"`
	Error          string            `json:"error,omitempty"`
	FinalRating    string            `json:"finalRating"`
	Buckets        map[string]string `json:"buckets"`
	Match          bool              `json:"match"`
	ModeMismatches []modeMismatch    `json:"modeMismatches,omitempty"`
	HandoffKinds   map[string]int    `json:"handoffKinds,omitempty"`
	UpstreamTotal  int               `json:"upstreamTotal"`
	UpstreamFailed int               `json:"upstreamFailed"`
	StudentPrompt  string            `json:"studentPrompt"`
	Turns          []turnRecord      `json:"turns"`
	UpstreamCalls  []upstreamCall    `json:"upstreamCalls"`
	FinalState     map[string]any    `json:"finalState"`
}

// mode -> (state phase field, state bucket field)
var modeFields = []struct{ Mode, Phase, Bucket string }{
	{"ConceptualUnderstanding", "conceptualAssessmentPhase", "conceptualAssessmentBucket"},
	{"CodeProblem", "codeAssessmentPhase", "codeAssessmentBucket"},
	{"BugHunting", "bugAssessmentPhase", "bugAssessmentBucket"},
}

type simulator struct {
	baseURL   string
	cfg       config
	proxy     *recordingProxy
	student   *studentLLM
	serverDir string
	outDir    string
	newClient func() *apiClient
}

func newTurnID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *simulator) run(ctx context.Context, j job) (res runResult) {
	res = runResult{
		ID: j.ID, Persona: j.Persona.Key, PersonaName: j.Persona.Name, Expected: j.Persona.Expected,
		Week: j.Week, Concept: j.Concept, Major: j.Major, Started: time.Now(), Buckets: map[string]string{},
	}
	defer func() { res.DurationSec = time.Since(res.Started).Seconds() }()

	rubric, err := os.ReadFile(filepath.Join(s.serverDir, "env", "rubrics", fmt.Sprintf("week%d_rubric.md", j.Week)))
	if err != nil {
		res.Error = "read rubric: " + err.Error()
		return res
	}
	res.StudentPrompt = studentSystemPrompt(j.Persona, j.Concept, j.Major, string(rubric))

	c := s.newClient()
	if err := c.login(); err != nil {
		res.Error = "login: " + err.Error()
		return res
	}
	res.SessionID = c.SessID
	welcome, err := c.bootstrap()
	if err != nil {
		res.Error = err.Error()
		return res
	}
	history := []chatMessage{{Role: "user", Content: "start"}, {Role: "assistant", Content: welcome}}

	send := func(text string) (turnRecord, error) {
		before, _ := c.state()
		t := turnRecord{
			Index: len(res.Turns), Student: text,
			PhaseBefore: str(before["conversationPhase"]), ModeBefore: str(before["activeMode"]),
		}
		history = append(history, chatMessage{Role: "user", Content: text})
		n0 := s.proxy.count(c.SessID)
		started := time.Now()
		raw, attempts, err := c.chat(newTurnID(), history)
		t.LatencyMs = time.Since(started).Milliseconds()
		t.Attempts = attempts
		t.AssistantRaw = raw
		t.AssistantVisible = visibleContent(raw)
		t.Sync = parseSync(raw)
		calls := s.proxy.calls(c.SessID)
		seen := map[string]bool{}
		for _, call := range calls[n0:] {
			t.UpstreamSeqs = append(t.UpstreamSeqs, call.Seq)
			for _, h := range call.Handoffs {
				if !seen[h] {
					seen[h] = true
					t.Handoffs = append(t.Handoffs, h)
				}
			}
		}
		if st, serr := c.state(); serr == nil {
			t.State = stateSummary(st)
		}
		if err != nil {
			t.Error = err.Error()
			history = history[:len(history)-1]
		} else {
			history = append(history, chatMessage{Role: "assistant", Content: t.AssistantVisible})
		}
		res.Turns = append(res.Turns, t)
		if err := writeRun(s.outDir, res); err != nil {
			log.Printf("write %s: %v", res.ID, err)
		}
		return t, err
	}

	if _, err := send(j.Major); err != nil {
		res.Error = "send major: " + err.Error()
		return s.finish(c, res)
	}
	if _, err := send(j.Concept); err != nil {
		res.Error = "select concept: " + err.Error()
		return s.finish(c, res)
	}
	studentStart := len(history) - 1

	for turn := 0; turn < s.cfg.MaxTurns; turn++ {
		st, err := c.state()
		if err != nil {
			res.Error = "state: " + err.Error()
			break
		}
		if st["assessmentComplete"] == true || str(st["conversationPhase"]) == "AssessmentResults" {
			res.Completed = true
			break
		}
		answer, err := s.student.reply(ctx, res.StudentPrompt, history[studentStart:])
		if err != nil {
			res.Error = "student model: " + err.Error()
			break
		}
		if _, err := send(answer); err != nil {
			res.Error = "chat: " + err.Error()
			break
		}
	}
	if !res.Completed && res.Error == "" {
		if st, err := c.state(); err == nil && (st["assessmentComplete"] == true || str(st["conversationPhase"]) == "AssessmentResults") {
			res.Completed = true
		} else {
			res.Error = fmt.Sprintf("assessment did not finish within %d student turns", s.cfg.MaxTurns)
		}
	}
	return s.finish(c, res)
}

func (s *simulator) finish(c *apiClient, res runResult) runResult {
	res.UpstreamCalls = s.proxy.calls(c.SessID)
	if st, err := c.state(); err == nil {
		res.FinalState = st
		res.FinalRating = str(st["finalRating"])
		for _, f := range modeFields {
			res.Buckets[f.Mode] = str(st[f.Bucket])
		}
	}
	analyze(&res)
	return res
}

func analyze(res *runResult) {
	res.HandoffKinds = map[string]int{}
	for _, t := range res.Turns {
		for _, h := range t.Handoffs {
			res.HandoffKinds[handoffKind(h)]++
		}
	}
	res.UpstreamTotal = len(res.UpstreamCalls)
	for _, call := range res.UpstreamCalls {
		if call.Status != 200 || call.Error != "" {
			res.UpstreamFailed++
		}
	}
	res.Match = res.Completed && res.FinalRating == res.Expected
	if !res.Completed {
		return
	}

	callBySeq := map[int]upstreamCall{}
	for _, call := range res.UpstreamCalls {
		callBySeq[call.Seq] = call
	}
	for _, f := range modeFields {
		if res.Week == 1 && f.Mode != "ConceptualUnderstanding" {
			continue
		}
		got := res.Buckets[f.Mode]
		if got == res.Expected {
			continue
		}
		mm := modeMismatch{Mode: f.Mode, Expected: res.Expected, Got: got, GradingTurn: -1}
		for _, t := range res.Turns {
			if t.ModeBefore == f.Mode {
				mm.TurnsInMode++
				mm.StudentAnswers = append(mm.StudentAnswers, t.Student)
				mm.HandoffsInMode = append(mm.HandoffsInMode, t.Handoffs...)
			}
			if mm.GradingTurn < 0 && str(t.State[f.Phase]) == "complete" {
				mm.GradingTurn = t.Index
				for i := len(t.UpstreamSeqs) - 1; i >= 0; i-- {
					call := callBySeq[t.UpstreamSeqs[i]]
					if b := str(parseSync(call.Response)[f.Bucket]); b != "" {
						mm.GradingCallSeq = call.Seq
						mm.ModelBucket = b
						mm.GraderReply = call.Response
						break
					}
				}
				if mm.ModelBucket == "" {
					mm.ModelBucket = str(t.Sync[f.Bucket])
					mm.GraderReply = t.AssistantRaw
				}
			}
		}
		res.ModeMismatches = append(res.ModeMismatches, mm)
	}
}

func handoffKind(h string) string {
	switch {
	case strings.Contains(h, "asked no interview question and omitted"):
		return "missing_sync_closing"
	case strings.Contains(h, "omitted the required"):
		return "missing_sync"
	case strings.Contains(h, "not learned yet"):
		return "out_of_scope_concepts"
	case strings.Contains(h, "the interview is finished"):
		return "closing_due"
	case strings.Contains(h, "did not open this part"):
		return "bad_opening"
	case strings.Contains(h, "violated assessment protocol because it"):
		reason := h[strings.Index(h, "because it")+len("because it "):]
		if i := strings.Index(reason, ". "); i > 0 {
			reason = reason[:i]
		}
		return "protocol: " + truncate(reason, 90)
	default:
		return "other"
	}
}

func parseSync(content string) map[string]any {
	matches := ipyJSONPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(matches[len(matches)-1][1]), &out)
	return out
}

func stateSummary(st map[string]any) map[string]any {
	keys := []string{
		"conversationPhase", "activeMode", "modeInterviewStep", "modesCompleted",
		"conceptualAssessmentPhase", "conceptualAssessmentBucket",
		"codeAssessmentPhase", "codeAssessmentBucket",
		"bugAssessmentPhase", "bugAssessmentBucket",
		"finalRating", "assessmentComplete", "coachingRequested",
		"modeVagueAnswers", "modeSimilarQuestionAsks", "modeQuestionsAsked",
	}
	out := map[string]any{}
	for _, k := range keys {
		if v, ok := st[k]; ok && v != nil && v != "" {
			out[k] = v
		}
	}
	return out
}

func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
