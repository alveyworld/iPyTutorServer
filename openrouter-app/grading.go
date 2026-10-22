package main

import "strings"

// Grading for the D5 engine, as specified in grading-rules.md. The model only labels
// answers; every rule that turns labels into buckets lives here.

const (
	levelNotReady    = "not_ready"
	levelCompetent   = "competent"
	levelExceptional = "exceptional"
)

const (
	dimConceptual    = "conceptual"
	dimDecomposition = "decomposition"
	dimCorrectness   = "correctness"
	dimUnderstanding = "understanding"
	dimAIUse         = "ai_use"
	dimStrategy      = "strategy"
)

// modeDimensions lists the rubric dimensions each assessment mode is graded on.
var modeDimensions = map[string][]string{
	modeConceptual: {dimConceptual},
	modeCode:       {dimDecomposition, dimCorrectness, dimUnderstanding, dimAIUse},
	modeBug:        {dimStrategy},
}

type gradeLabel struct {
	Dimension   string
	Level       string
	AnswerIndex int
	// Source is who set the label. A vague label always stands; between two other labels
	// for the same answer and dimension the higher level wins (grading-rules.md, Step 1).
	Source string
}

const (
	labelSourceLevels    = "levels"    // levels-only call at mode close
	labelSourceEvaluator = "evaluator" // Evaluator brief
	labelSourceVague     = "vague"     // set by Go for a vague answer
	labelSourceHolistic  = "holistic"  // one call over the whole Bug part, which replaces its per-answer labels
)

func levelRank(level string) int {
	switch level {
	case levelNotReady:
		return 0
	case levelCompetent:
		return 1
	case levelExceptional:
		return 2
	default:
		return -1
	}
}

// normalizeLevel maps a model-written level to its canonical form, or "" when it is not
// one. Rubric headings say "Not Yet Ready", so that spelling is accepted too.
func normalizeLevel(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.Trim(s, ".*`\"'")
	s = strings.NewReplacer(" ", "_", "-", "_").Replace(s)
	switch s {
	case "not_ready", "not_yet_ready", "not_ready_yet", "notready":
		return levelNotReady
	case "competent":
		return levelCompetent
	case "exceptional":
		return levelExceptional
	default:
		return ""
	}
}

func dimensionInMode(mode, dimension string) bool {
	for _, d := range modeDimensions[mode] {
		if d == dimension {
			return true
		}
	}
	return false
}

// combinedLevels returns each dimension's level across the mode's answers: the level
// given most often, with a tie going to the higher level (grading-rules.md, Step 2).
func combinedLevels(labels []gradeLabel) map[string]string {
	counts := map[string]map[string]int{}
	for _, l := range labels {
		if levelRank(l.Level) < 0 {
			continue
		}
		if counts[l.Dimension] == nil {
			counts[l.Dimension] = map[string]int{}
		}
		counts[l.Dimension][l.Level]++
	}
	out := map[string]string{}
	for dim, byLevel := range counts {
		best := ""
		for _, level := range []string{levelNotReady, levelCompetent, levelExceptional} {
			// Levels are visited lowest first, so an equal count moves up: ties go higher.
			if byLevel[level] > 0 && byLevel[level] >= byLevel[best] {
				best = level
			}
		}
		out[dim] = best
	}
	return out
}

func bucketForLevel(level string) string {
	switch level {
	case levelExceptional:
		return bucketExceptional
	case levelCompetent:
		return bucketCompetent
	default:
		return bucketNotReady
	}
}

// gradeMode turns one mode's labels into its bucket (grading-rules.md, Step 2).
// codePasted only matters for Code mode: closing without pasted code is Not Ready Yet.
func gradeMode(mode string, labels []gradeLabel, codePasted bool) string {
	var kept []gradeLabel
	for _, l := range labels {
		if dimensionInMode(mode, l.Dimension) {
			kept = append(kept, l)
		}
	}
	if mode == modeCode && !codePasted {
		return bucketNotReady
	}
	levels := combinedLevels(kept)
	if len(levels) == 0 {
		return bucketNotReady
	}

	switch mode {
	case modeConceptual:
		return bucketForLevel(levels[dimConceptual])
	case modeBug:
		return bucketForLevel(levels[dimStrategy])
	case modeCode:
		// Code assesses understanding of the code, so a candidate who cannot explain it
		// (understanding Not Ready) is Not Ready Yet. Otherwise Not Ready Yet needs 2+ Not
		// Ready dimensions; one other weak dimension alone gives Competent.
		if levels[dimUnderstanding] == levelNotReady {
			return bucketNotReady
		}
		notReady, allExceptional := 0, true
		for _, level := range levels {
			if level == levelNotReady {
				notReady++
			}
			if level != levelExceptional {
				allExceptional = false
			}
		}
		if notReady >= 2 {
			return bucketNotReady
		}
		if allExceptional {
			return bucketExceptional
		}
		return bucketCompetent
	default:
		return bucketNotReady
	}
}

// applyD5Grades sets the mode buckets and the overall rating on state (Steps 2 and 3).
// The overall rating reuses computeFinalRating, which already follows the final rubric.
func applyD5Grades(state *AgentSessionState, labels map[string][]gradeLabel, codePasted bool) {
	state.ConceptualAssessmentBucket = gradeMode(modeConceptual, labels[modeConceptual], false)
	if state.isProblemDecompositionWeek() {
		state.CodeAssessmentBucket = bucketNA
		state.BugAssessmentBucket = bucketNA
	} else {
		state.CodeAssessmentBucket = gradeMode(modeCode, labels[modeCode], codePasted)
		state.BugAssessmentBucket = gradeMode(modeBug, labels[modeBug], false)
	}
	state.FinalRating = computeFinalRating(state)
	state.AssessmentComplete = state.FinalRating != ""
}
