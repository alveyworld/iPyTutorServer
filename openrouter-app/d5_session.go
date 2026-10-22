package main

import (
	"hash/fnv"
	"math/rand/v2"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// chatEngine selects the /api/chat implementation: "d5" for the Interviewer + Evaluator
// engine (D5-interviewer-evaluator-design.md); anything else keeps the original engine.
var chatEngine = strings.ToLower(strings.TrimSpace(os.Getenv("IPY_ENGINE")))

func d5Enabled() bool {
	return chatEngine == "d5"
}

type d5Persona struct {
	Name string
	Role string
}

// d5PersonaRoles and d5NamePools are the canned personas (design §4.1): one interviewer
// per phase, its name drawn per session from the pool.
var d5PersonaRoles = map[string]string{
	modeConceptual: "hiring manager",
	modeCode:       "software developer",
	modeBug:        "QA engineer",
	modeCoaching:   "recruiter on the HR team",
}

var d5NamePools = map[string][]string{
	modeConceptual: {"Alex", "Jordan", "Priya", "Marcus", "Elena", "Sam"},
	modeCode:       {"Taylor", "Diego", "Mei", "Noah", "Aisha", "Chris"},
	modeBug:        {"Riley", "Omar", "Hannah", "Kenji", "Lucia", "Ben"},
	modeCoaching:   {"Samantha", "David", "Andre", "Nina", "Leo", "Farah"},
}

// d5PersonaNamePattern matches any pooled persona name, for the addressesPersona post-check.
var d5PersonaNamePattern = func() string {
	var names []string
	for _, pool := range d5NamePools {
		for _, n := range pool {
			names = append(names, strings.ToLower(n))
		}
	}
	return strings.Join(names, "|")
}()

func pickD5Personas(sessionID string) map[string]d5Persona {
	h := fnv.New64a()
	_, _ = h.Write([]byte(sessionID))
	rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), h.Sum64()))
	out := map[string]d5Persona{}
	for _, mode := range []string{modeConceptual, modeCode, modeBug, modeCoaching} {
		pool := d5NamePools[mode]
		out[mode] = d5Persona{Name: pool[rng.IntN(len(pool))], Role: d5PersonaRoles[mode]}
	}
	return out
}

type d5Message struct {
	Role    string // "assistant" (interviewer) or "user" (candidate)
	Content string
	Mode    string
}

// d5Session is the D5 engine's per-session state, hung off AgentSessionState. turnMu
// serializes chat turns for the session; mu guards the fields background Evaluator jobs
// write (brief, labels, evidence).
type d5Session struct {
	turnMu sync.Mutex
	mu     sync.Mutex

	Personas      map[string]d5Persona
	CompanyName   string
	CompanyDomain string

	Transcript []d5Message
	// Material is the scenario, task or snippet each mode opened with.
	Material map[string]string
	// NeedsOpening is set when a mode's opening could not be generated; the next student
	// message retries it.
	NeedsOpening bool

	AnswerIndex  int
	RedirectUsed map[string]bool
	// LastAskTarget is the rubric dimension the interviewer's latest question targets.
	LastAskTarget string
	LastMove      string
	// PendingIssues are post-check hits waiting for the next Evaluator run.
	PendingIssues []string

	Brief         *d5Brief
	briefInFlight chan struct{}
	ModeBriefs    map[string]*d5Brief
	Labels        map[string][]gradeLabel
	Evidence      map[string][]string
	VagueAnswers  map[int]bool
	labelJobs     sync.WaitGroup
	CodePasted    bool
	// BugDefect is the Bug snippet's deliberate defect, from the opening's hidden DEFECT
	// line. Only the Evaluator sees it; the interviewer never does.
	BugDefect string
	// Drafts are pre-drafted openings by mode (Phase 2, d5_drafts.go); draftDone closes
	// when a mode's draft job finishes. Both are guarded by mu.
	Drafts    map[string]*d5PreparedOpening
	draftDone map[string]chan struct{}
	// CodeRequests counts requests for code in Code mode before a paste.
	CodeRequests int
	// Answers holds each candidate answer with the question it replied to, so answers the
	// Evaluator failed to label can be labelled when the mode closes.
	Answers  map[int]d5Answer
	Labelled map[int]bool
	// NoAIAnswers are answers saying the candidate did not use AI; Go labels them
	// ai_use = competent (the cap), because students are expected to use AI.
	NoAIAnswers map[int]bool
}

type d5Answer struct {
	Mode     string
	Question string
	Answer   string
	Target   string
}

func newD5Session(sessionID string) *d5Session {
	return &d5Session{
		Personas:     pickD5Personas(sessionID),
		Material:     map[string]string{},
		RedirectUsed: map[string]bool{},
		ModeBriefs:   map[string]*d5Brief{},
		Labels:       map[string][]gradeLabel{},
		Evidence:     map[string][]string{},
		VagueAnswers: map[int]bool{},
		Answers:      map[int]d5Answer{},
		Labelled:     map[int]bool{},
		NoAIAnswers:  map[int]bool{},
	}
}

func (s *d5Session) addLabels(mode string, labels []gradeLabel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range labels {
		if !dimensionInMode(mode, l.Dimension) || levelRank(l.Level) < 0 {
			continue
		}
		if l.Dimension == dimAIUse && s.NoAIAnswers[l.AnswerIndex] && l.Source != labelSourceVague {
			continue // not using AI is already capped at Competent; no labeller overrides it
		}
		if l.Dimension == dimCorrectness && l.Level == levelCompetent {
			// Correctness is pass/fail: code that runs and gives correct output passes.
			l.Level = levelExceptional
		}
		s.Labelled[l.AnswerIndex] = true
		replaced := false
		for i, existing := range s.Labels[mode] {
			if existing.AnswerIndex == l.AnswerIndex && existing.Dimension == l.Dimension {
				// One label per answer and dimension. A vague answer's label stands;
				// otherwise the higher level wins, giving the benefit of the doubt.
				if existing.Source != labelSourceVague && (l.Source == labelSourceVague || levelRank(l.Level) > levelRank(existing.Level)) {
					s.Labels[mode][i] = l
				}
				replaced = true
				break
			}
		}
		if !replaced {
			s.Labels[mode] = append(s.Labels[mode], l)
		}
	}
}

func (s *d5Session) labelsSnapshot() map[string][]gradeLabel {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string][]gradeLabel{}
	for mode, labels := range s.Labels {
		out[mode] = append([]gradeLabel(nil), labels...)
	}
	return out
}

// withoutOffTargetLabels drops Code labels the Evaluator gave answers that were not replies
// to that dimension's question. The prompt tells the Evaluator not to label a dimension an
// answer did not discuss, but it still did: AI use on the code itself, decomposition on an
// explanation of a line, correctness or understanding on the planning answer before any code
// existed. Those labels (mostly Competent or Not Ready, since the answer said nothing about
// the dimension) outvoted the one label from the real question and held strong candidates'
// Code part below Exceptional.
//
// A label is dropped when its dimension has a label from its own question and either the
// dimension is AI use or decomposition, or the label is on the planning answer. When no
// answer to the dimension's own question was labelled, the labels are kept, since they may
// be the only evidence. Understanding and correctness labels from later answers are kept:
// dropping those as well made ratings less accurate in replays of recorded runs.
// It returns the labels to grade and the ones dropped.
func (s *d5Session) withoutOffTargetLabels(labels map[string][]gradeLabel) (map[string][]gradeLabel, []gradeLabel) {
	code := labels[modeCode]
	s.mu.Lock()
	defer s.mu.Unlock()
	asked := map[string]bool{}
	for _, l := range code {
		if s.Answers[l.AnswerIndex].Target == l.Dimension {
			asked[l.Dimension] = true
		}
	}
	var kept, dropped []gradeLabel
	for _, l := range code {
		target := s.Answers[l.AnswerIndex].Target
		offTarget := target != l.Dimension && asked[l.Dimension]
		weak := l.Dimension == dimAIUse || l.Dimension == dimDecomposition || target == dimDecomposition
		if offTarget && weak {
			dropped = append(dropped, l)
			continue
		}
		kept = append(kept, l)
	}
	if len(dropped) == 0 {
		return labels, nil
	}
	out := make(map[string][]gradeLabel, len(labels))
	for mode, ls := range labels {
		out[mode] = ls
	}
	out[modeCode] = kept
	return out, dropped
}

// lastAnswerIndex is the highest answer index recorded for the mode (0 when none).
func (s *d5Session) lastAnswerIndex(mode string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	last := 0
	for idx, a := range s.Answers {
		if a.Mode == mode && idx > last {
			last = idx
		}
	}
	return last
}

// unlabelledAnswers lists the mode's non-vague answers whose targeted dimension has no
// label yet.
func (s *d5Session) unlabelledAnswers(mode string) map[int]d5Answer {
	s.mu.Lock()
	defer s.mu.Unlock()
	targetLabelled := map[int]bool{}
	for _, l := range s.Labels[mode] {
		if a, ok := s.Answers[l.AnswerIndex]; ok && a.Target == l.Dimension {
			targetLabelled[l.AnswerIndex] = true
		}
	}
	out := map[int]d5Answer{}
	for idx, a := range s.Answers {
		// An answer still needs a label when its targeted dimension has none, even if the
		// Evaluator labelled some other dimension (it once labelled a code paste as
		// understanding, leaving correctness ungraded).
		if a.Mode == mode && a.Target != "" && !targetLabelled[idx] && !s.VagueAnswers[idx] {
			out[idx] = a
		}
	}
	return out
}

func (s *d5Session) latestBrief() *d5Brief {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Brief
}

// modeMessages returns the transcript messages from one mode, most recent last.
func (s *d5Session) modeMessages(mode string) []d5Message {
	var out []d5Message
	for _, m := range s.Transcript {
		if m.Mode == mode {
			out = append(out, m)
		}
	}
	return out
}

// lastInterviewerMessage returns the interviewer's latest message in the transcript.
func (s *d5Session) lastInterviewerMessage() string {
	for i := len(s.Transcript) - 1; i >= 0; i-- {
		if s.Transcript[i].Role == "assistant" {
			return s.Transcript[i].Content
		}
	}
	return ""
}

func d5SessionFor(state *AgentSessionState, sessionID string) *d5Session {
	if state.D5 == nil {
		state.D5 = newD5Session(sessionID)
	}
	return state.D5
}

var articleVowelPattern = regexp.MustCompile(`(?i)^[aeiou]`)

func withArticle(phrase string) string {
	phrase = strings.TrimSpace(phrase)
	lower := strings.ToLower(phrase)
	if phrase == "" {
		return phrase
	}
	if strings.HasPrefix(lower, "a ") || strings.HasPrefix(lower, "an ") || strings.HasPrefix(lower, "the ") {
		// The phrase goes mid-sentence, so "A food-tech company" becomes "a food-tech company".
		return strings.ToLower(phrase[:1]) + phrase[1:]
	}
	if articleVowelPattern.MatchString(phrase) {
		return "an " + phrase
	}
	return "a " + phrase
}

// personaIntro is the server-written introduction for a phase's interviewer (§4.1).
func (s *d5Session) personaIntro(mode string) string {
	p := s.Personas[mode]
	company := s.CompanyName
	if company == "" {
		return "Hi, I'm " + p.Name + ", " + withArticle(p.Role) + " on the team."
	}
	intro := "Hi, I'm " + p.Name + ", " + withArticle(p.Role) + " at " + company
	if s.CompanyDomain != "" {
		intro += ", " + withArticle(strings.TrimRight(s.CompanyDomain, "."))
	}
	return intro + "."
}
