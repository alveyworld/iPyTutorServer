package main

import "testing"

func labelsOf(dim string, levels ...string) []gradeLabel {
	out := make([]gradeLabel, len(levels))
	for i, l := range levels {
		out[i] = gradeLabel{Dimension: dim, Level: l, AnswerIndex: i + 1}
	}
	return out
}

func codeLabels(pairs ...string) []gradeLabel {
	var out []gradeLabel
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, gradeLabel{Dimension: pairs[i], Level: pairs[i+1]})
	}
	return out
}

// The cases are the worked examples in grading-rules.md, numbered the same way.
func TestGradeModeSpecExamples(t *testing.T) {
	const nr, c, e = levelNotReady, levelCompetent, levelExceptional
	cases := []struct {
		n          int
		mode       string
		labels     []gradeLabel
		codePasted bool
		want       string
	}{
		{1, modeConceptual, labelsOf(dimConceptual, c, e, e), false, bucketExceptional},
		{2, modeConceptual, labelsOf(dimConceptual, nr, nr, c), false, bucketNotReady},
		{3, modeConceptual, labelsOf(dimConceptual, e, e, e), false, bucketExceptional},
		{4, modeConceptual, labelsOf(dimConceptual, nr, e, e), false, bucketExceptional},
		{19, modeConceptual, labelsOf(dimConceptual, c, c, nr, c), false, bucketCompetent},
		{20, modeConceptual, labelsOf(dimConceptual, c, e), false, bucketExceptional},
		{21, modeConceptual, labelsOf(dimConceptual, nr, c), false, bucketCompetent},
		{5, modeConceptual, nil, false, bucketNotReady},
		{6, modeBug, labelsOf(dimStrategy, c, e, c), false, bucketCompetent},
		{22, modeBug, labelsOf(dimStrategy, nr, e, nr, e), false, bucketExceptional},
		{7, modeBug, labelsOf(dimStrategy, e, e, e, e), false, bucketExceptional},
		{8, modeCode, codeLabels(dimDecomposition, e, dimCorrectness, e, dimUnderstanding, e), true, bucketExceptional},
		{9, modeCode, codeLabels(dimDecomposition, e, dimCorrectness, c, dimUnderstanding, e, dimAIUse, e), true, bucketCompetent},
		{10, modeCode, codeLabels(dimCorrectness, nr, dimDecomposition, e, dimUnderstanding, e, dimAIUse, e), true, bucketCompetent},
		{11, modeCode, codeLabels(dimDecomposition, nr, dimUnderstanding, nr, dimCorrectness, c), true, bucketNotReady},
		{12, modeCode, codeLabels(dimAIUse, nr, dimDecomposition, e, dimCorrectness, e, dimUnderstanding, e), true, bucketCompetent},
		{13, modeCode, codeLabels(dimUnderstanding, e, dimUnderstanding, nr, dimDecomposition, c, dimCorrectness, c, dimAIUse, c), true, bucketCompetent},
		{14, modeCode, codeLabels(dimDecomposition, e), false, bucketNotReady},
		{23, modeCode, codeLabels(dimDecomposition, c, dimCorrectness, e, dimUnderstanding, c, dimUnderstanding, nr, dimUnderstanding, nr, dimAIUse, e), true, bucketNotReady},
		{24, modeCode, codeLabels(dimDecomposition, e, dimCorrectness, e, dimUnderstanding, e, dimAIUse, nr), true, bucketCompetent},
	}
	for _, tc := range cases {
		if got := gradeMode(tc.mode, tc.labels, tc.codePasted); got != tc.want {
			t.Errorf("example %d: gradeMode = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestGradeModeIgnoresOtherModesDimensions(t *testing.T) {
	labels := []gradeLabel{{Dimension: dimConceptual, Level: levelExceptional}, {Dimension: dimAIUse, Level: levelNotReady}}
	if got := gradeMode(modeConceptual, labels, false); got != bucketExceptional {
		t.Fatalf("got %q, want Exceptional", got)
	}
}

func TestApplyD5GradesOverallRating(t *testing.T) {
	const nr, c, e = levelNotReady, levelCompetent, levelExceptional
	codeAll := func(level string) []gradeLabel {
		return codeLabels(dimDecomposition, level, dimCorrectness, level, dimUnderstanding, level, dimAIUse, level)
	}
	cases := []struct {
		n      int
		week   int
		labels map[string][]gradeLabel
		want   [4]string
	}{
		{15, 5, map[string][]gradeLabel{
			modeConceptual: labelsOf(dimConceptual, c),
			modeCode:       codeAll(e),
			modeBug:        labelsOf(dimStrategy, e),
		}, [4]string{bucketCompetent, bucketExceptional, bucketExceptional, bucketCompetent}},
		{16, 5, map[string][]gradeLabel{
			modeConceptual: labelsOf(dimConceptual, e),
			modeCode:       codeAll(e),
			modeBug:        labelsOf(dimStrategy, nr),
		}, [4]string{bucketExceptional, bucketExceptional, bucketNotReady, bucketNotReady}},
		{17, 5, map[string][]gradeLabel{
			modeConceptual: labelsOf(dimConceptual, e),
			modeCode:       codeAll(e),
			modeBug:        labelsOf(dimStrategy, e),
		}, [4]string{bucketExceptional, bucketExceptional, bucketExceptional, bucketExceptional}},
		{18, 1, map[string][]gradeLabel{
			modeConceptual: labelsOf(dimConceptual, c),
		}, [4]string{bucketCompetent, bucketNA, bucketNA, bucketCompetent}},
	}
	for _, tc := range cases {
		state := newAgentSessionState()
		state.CurrentWeekNumber = tc.week
		applyD5Grades(state, tc.labels, true)
		got := [4]string{state.ConceptualAssessmentBucket, state.CodeAssessmentBucket, state.BugAssessmentBucket, state.FinalRating}
		if got != tc.want {
			t.Errorf("example %d: got %v, want %v", tc.n, got, tc.want)
		}
	}
}

func TestNormalizeLevel(t *testing.T) {
	for raw, want := range map[string]string{
		"not_ready": levelNotReady, "Not Yet Ready": levelNotReady, "not ready yet": levelNotReady,
		"Competent": levelCompetent, "**exceptional**": levelExceptional, "strong": "",
	} {
		if got := normalizeLevel(raw); got != want {
			t.Errorf("normalizeLevel(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestAddLabelsOneLabelPerAnswerHigherWins(t *testing.T) {
	s := newD5Session("t")
	s.addLabels(modeCode, []gradeLabel{{Dimension: dimUnderstanding, Level: levelCompetent, AnswerIndex: 6, Source: labelSourceLevels}})
	s.addLabels(modeCode, []gradeLabel{{Dimension: dimUnderstanding, Level: levelNotReady, AnswerIndex: 6, Source: labelSourceEvaluator}})
	if got := s.labelsSnapshot()[modeCode]; len(got) != 1 || got[0].Level != levelCompetent {
		t.Fatalf("labels = %+v, want the higher (competent) label", got)
	}
	s.addLabels(modeCode, []gradeLabel{{Dimension: dimUnderstanding, Level: levelExceptional, AnswerIndex: 6, Source: labelSourceEvaluator}})
	if got := s.labelsSnapshot()[modeCode]; len(got) != 1 || got[0].Level != levelExceptional {
		t.Fatalf("labels = %+v, want exceptional", got)
	}
	s.addLabels(modeConceptual, []gradeLabel{{Dimension: dimConceptual, Level: levelNotReady, AnswerIndex: 2, Source: labelSourceVague}})
	s.addLabels(modeConceptual, []gradeLabel{{Dimension: dimConceptual, Level: levelExceptional, AnswerIndex: 2, Source: labelSourceEvaluator}})
	if got := s.labelsSnapshot()[modeConceptual]; len(got) != 1 || got[0].Source != labelSourceVague {
		t.Fatalf("a vague answer's label must stand, got %+v", got)
	}
}

func TestNoAICapsCodeAtCompetent(t *testing.T) {
	const c, e = levelCompetent, levelExceptional
	labels := codeLabels(dimDecomposition, e, dimCorrectness, e, dimUnderstanding, e, dimAIUse, c)
	if got := gradeMode(modeCode, labels, true); got != bucketCompetent {
		t.Fatalf("all Exceptional but no AI use: got %q, want Competent", got)
	}
}

func TestWithoutOffTargetLabelsKeepsOnlyTheAIQuestionsLabel(t *testing.T) {
	_, sess := d5TestState(5, modeCode)
	sess.Answers[1] = d5Answer{Mode: modeCode, Target: dimDecomposition}
	sess.Answers[2] = d5Answer{Mode: modeCode, Target: dimCorrectness}
	sess.Answers[3] = d5Answer{Mode: modeCode, Target: dimUnderstanding}
	sess.Answers[4] = d5Answer{Mode: modeCode, Target: dimAIUse}
	labels := map[string][]gradeLabel{modeCode: {
		{Dimension: dimDecomposition, Level: levelExceptional, AnswerIndex: 1},
		{Dimension: dimCorrectness, Level: levelExceptional, AnswerIndex: 2},
		{Dimension: dimAIUse, Level: levelCompetent, AnswerIndex: 2}, // the code answer, no AI in it
		{Dimension: dimUnderstanding, Level: levelExceptional, AnswerIndex: 3},
		{Dimension: dimAIUse, Level: levelCompetent, AnswerIndex: 3}, // an explanation of a line
		{Dimension: dimAIUse, Level: levelExceptional, AnswerIndex: 4},
	}}
	if got := gradeMode(modeCode, labels[modeCode], true); got != bucketCompetent {
		t.Fatalf("without the filter the stray AI-use labels outvote the real one: bucket = %s, want %s", got, bucketCompetent)
	}
	filtered, dropped := sess.withoutOffTargetLabels(labels)
	if len(dropped) != 2 || dropped[0].AnswerIndex != 2 || dropped[1].AnswerIndex != 3 {
		t.Fatalf("dropped = %+v, want the AI-use labels on answers 2 and 3", dropped)
	}
	if got := gradeMode(modeCode, filtered[modeCode], true); got != bucketExceptional {
		t.Fatalf("bucket after the filter = %s, want %s", got, bucketExceptional)
	}
	if len(labels[modeCode]) != 6 {
		t.Fatal("the filter changed the labels it was given")
	}
}

func TestWithoutOffTargetLabelsKeepsLabelsWhenAIWasNeverAsked(t *testing.T) {
	_, sess := d5TestState(5, modeCode)
	sess.Answers[1] = d5Answer{Mode: modeCode, Target: dimCorrectness}
	labels := map[string][]gradeLabel{modeCode: {
		{Dimension: dimCorrectness, Level: levelExceptional, AnswerIndex: 1},
		{Dimension: dimAIUse, Level: levelExceptional, AnswerIndex: 1}, // the candidate raised AI use unprompted
	}}
	filtered, dropped := sess.withoutOffTargetLabels(labels)
	if len(dropped) != 0 || len(filtered[modeCode]) != 2 {
		t.Fatalf("with no AI-use question labelled, nothing should be dropped: dropped %+v, kept %+v", dropped, filtered[modeCode])
	}
}

func TestWithoutOffTargetLabelsIgnoresExtrasOnThePlanningAnswerAndDecompositionElsewhere(t *testing.T) {
	_, sess := d5TestState(5, modeCode)
	sess.Answers[1] = d5Answer{Mode: modeCode, Target: dimDecomposition}
	sess.Answers[2] = d5Answer{Mode: modeCode, Target: dimCorrectness}
	sess.Answers[3] = d5Answer{Mode: modeCode, Target: dimUnderstanding}
	sess.Answers[4] = d5Answer{Mode: modeCode, Target: dimUnderstanding}
	labels := map[string][]gradeLabel{modeCode: {
		{Dimension: dimDecomposition, Level: levelExceptional, AnswerIndex: 1},
		{Dimension: dimUnderstanding, Level: levelCompetent, AnswerIndex: 1}, // the plan: no code to explain yet
		{Dimension: dimCorrectness, Level: levelNotReady, AnswerIndex: 1},    // the plan: no code to run yet
		{Dimension: dimCorrectness, Level: levelExceptional, AnswerIndex: 2},
		{Dimension: dimUnderstanding, Level: levelExceptional, AnswerIndex: 3},
		{Dimension: dimDecomposition, Level: levelCompetent, AnswerIndex: 3}, // an explanation, not a plan
		{Dimension: dimDecomposition, Level: levelCompetent, AnswerIndex: 4},
		{Dimension: dimUnderstanding, Level: levelCompetent, AnswerIndex: 4}, // a later explanation: kept
	}}
	filtered, dropped := sess.withoutOffTargetLabels(labels)
	if len(dropped) != 4 {
		t.Fatalf("dropped %+v, want the two planning-answer extras and the two later decomposition labels", dropped)
	}
	for _, l := range filtered[modeCode] {
		if l.AnswerIndex == 4 && l.Dimension == dimUnderstanding {
			return
		}
	}
	t.Fatal("a later understanding label from its own question was dropped")
}

func TestWithoutOffTargetLabelsKeepsPlanningExtrasWhenNothingElseCoversTheDimension(t *testing.T) {
	_, sess := d5TestState(5, modeCode)
	sess.Answers[1] = d5Answer{Mode: modeCode, Target: dimDecomposition}
	labels := map[string][]gradeLabel{modeCode: {
		{Dimension: dimDecomposition, Level: levelExceptional, AnswerIndex: 1},
		{Dimension: dimCorrectness, Level: levelExceptional, AnswerIndex: 1}, // the plan already contained working code
	}}
	if _, dropped := sess.withoutOffTargetLabels(labels); len(dropped) != 0 {
		t.Fatalf("dropped %+v, but nothing else covers correctness", dropped)
	}
}
