package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed env/instructions/IPyIntervu-entrypoint.md
//go:embed env/instructions/IPyIntervu-flow.md
//go:embed env/instructions/IPyIntervu-protocols.md
//go:embed env/instructions/IPyIntervu-week-scope.md
//go:embed env/instructions/IPyIntervu-modes-shared.md
//go:embed env/instructions/IPyIntervu-modes-conceptual.md
//go:embed env/instructions/IPyIntervu-modes-code.md
//go:embed env/instructions/IPyIntervu-modes-bug.md
//go:embed env/instructions/IPyIntervu-modes-coaching.md
//go:embed env/IPYIntervu_support_files/week1_key_concepts.md
//go:embed env/IPYIntervu_support_files/week2_key_concepts.md
//go:embed env/IPYIntervu_support_files/week3_key_concepts.md
//go:embed env/IPYIntervu_support_files/week4_key_concepts.md
//go:embed env/IPYIntervu_support_files/week5_key_concepts.md
//go:embed env/IPYIntervu_support_files/week6_key_concepts.md
//go:embed env/IPYIntervu_support_files/week7_key_concepts.md
//go:embed env/IPYIntervu_support_files/week8_key_concepts.md
//go:embed env/IPYIntervu_support_files/week9_key_concepts.md
//go:embed env/IPYIntervu_support_files/week1_competency_guide.md
//go:embed env/IPYIntervu_support_files/week2_competency_guide.md
//go:embed env/IPYIntervu_support_files/week3_competency_guide.md
//go:embed env/IPYIntervu_support_files/week4_competency_guide.md
//go:embed env/IPYIntervu_support_files/week5_competency_guide.md
//go:embed env/IPYIntervu_support_files/week6_competency_guide.md
//go:embed env/IPYIntervu_support_files/week7_competency_guide.md
//go:embed env/IPYIntervu_support_files/week8_competency_guide.md
//go:embed env/IPYIntervu_support_files/week9_competency_guide.md
//go:embed env/rubrics/week1_rubric.md
//go:embed env/rubrics/week2_rubric.md
//go:embed env/rubrics/week3_rubric.md
//go:embed env/rubrics/week4_rubric.md
//go:embed env/rubrics/week5_rubric.md
//go:embed env/rubrics/week6_rubric.md
//go:embed env/rubrics/week7_rubric.md
//go:embed env/rubrics/week8_rubric.md
//go:embed env/rubrics/week9_rubric.md
//go:embed env/rubrics/final_assessment_rubric.md
var instructionFS embed.FS

type promptFile struct {
	displayName string
	path        string
}

func selectPromptFiles(state *AgentSessionState) ([]promptFile, string) {
	files := []promptFile{
		{displayName: "IPyIntervu-entrypoint.md", path: "env/instructions/IPyIntervu-entrypoint.md"},
	}
	bundleParts := []string{"core"}

	switch state.ConversationPhase {
	case phaseAwaitingMajor, phaseAwaitingKeyConcept:
		files = append(files, promptFile{"IPyIntervu-flow.md", "env/instructions/IPyIntervu-flow.md"})
		bundleParts = append(bundleParts, "flow")
	case phaseAssessmentInProgress, phaseAssessmentResults:
		files = append(files,
			promptFile{"IPyIntervu-protocols.md", "env/instructions/IPyIntervu-protocols.md"},
			promptFile{"IPyIntervu-week-scope.md", "env/instructions/IPyIntervu-week-scope.md"},
			promptFile{"IPyIntervu-modes-shared.md", "env/instructions/IPyIntervu-modes-shared.md"},
		)
		bundleParts = append(bundleParts, "assessment")
	}

	if state.ActiveMode == modeCoaching {
		files = append(files, promptFile{"IPyIntervu-modes-coaching.md", "env/instructions/IPyIntervu-modes-coaching.md"})
		bundleParts = append(bundleParts, "coaching")
	} else if state.ConversationPhase == phaseAssessmentInProgress || state.ConversationPhase == phaseAssessmentResults {
		switch state.ActiveMode {
		case modeConceptual:
			files = append(files, promptFile{"IPyIntervu-modes-conceptual.md", "env/instructions/IPyIntervu-modes-conceptual.md"})
			bundleParts = append(bundleParts, "mode-conceptual")
		case modeCode:
			files = append(files, promptFile{"IPyIntervu-modes-code.md", "env/instructions/IPyIntervu-modes-code.md"})
			bundleParts = append(bundleParts, "mode-code")
		case modeBug:
			files = append(files, promptFile{"IPyIntervu-modes-bug.md", "env/instructions/IPyIntervu-modes-bug.md"})
			bundleParts = append(bundleParts, "mode-bug")
		}
	}

	if state.CurrentWeekNumber > 0 {
		week := state.CurrentWeekNumber
		files = append(files,
			promptFile{fmt.Sprintf("week%d_key_concepts.md", week), fmt.Sprintf("env/IPYIntervu_support_files/week%d_key_concepts.md", week)},
			promptFile{fmt.Sprintf("week%d_competency_guide.md", week), fmt.Sprintf("env/IPYIntervu_support_files/week%d_competency_guide.md", week)},
			promptFile{fmt.Sprintf("week%d_rubric.md", week), fmt.Sprintf("env/rubrics/week%d_rubric.md", week)},
		)
		bundleParts = append(bundleParts, fmt.Sprintf("week%d-kb", week))
	}

	if state.ConversationPhase == phaseAssessmentResults || state.AssessmentComplete {
		files = append(files, promptFile{"final_assessment_rubric.md", "env/rubrics/final_assessment_rubric.md"})
		bundleParts = append(bundleParts, "results")
	}

	return files, strings.Join(bundleParts, "+")
}

func assessmentSyncPromptForState(state *AgentSessionState) string {
	if state.ConversationPhase != phaseAssessmentInProgress {
		return ""
	}
	phaseField, bucketField, label := currentModeSyncFields(state)
	if phaseField == "" {
		return ""
	}
	return fmt.Sprintf(
		"MANDATORY THIS TURN (%s): End your reply with ```_ipyintervu``` JSON as the absolute last lines — the reply is INCOMPLETE without it. Do not stop after Got it./Thanks. alone; append the fence in the same reply. "+
			"While interviewing include ONLY {\"%s\": \"in_progress\"}. "+
			"When finishing this mode include \"%s\": \"complete\" and \"%s\" in the same fence. "+
			"Omitting the block forces a server retry and may fail closed. Never mention the sync block in interview text.\n",
		label, phaseField, phaseField, bucketField,
	)
}

func buildWeekRubric(weekNumber int) (string, error) {
	// Build just the week's rubric for caching
	if weekNumber <= 0 {
		return "", nil
	}

	var b strings.Builder
	weekFile := fmt.Sprintf("env/rubrics/week%d_rubric.md", weekNumber)
	data, err := instructionFS.ReadFile(weekFile)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", weekFile, err)
	}

	b.WriteString("=== ")
	b.WriteString(fmt.Sprintf("week%d_rubric.md", weekNumber))
	b.WriteString(" ===\n")
	b.Write(data)
	b.WriteString("\n")
	return b.String(), nil
}

func buildBaseBundle() (string, error) {
	// Base bundle: static instructions that don't change during session
	// Sent once to cache, then reused for all requests
	var b strings.Builder

	// Load base instruction files
	baseFiles := []promptFile{
		{displayName: "IPyIntervu-entrypoint.md", path: "env/instructions/IPyIntervu-entrypoint.md"},
		{displayName: "IPyIntervu-protocols.md", path: "env/instructions/IPyIntervu-protocols.md"},
		{displayName: "IPyIntervu-week-scope.md", path: "env/instructions/IPyIntervu-week-scope.md"},
		{displayName: "IPyIntervu-modes-shared.md", path: "env/instructions/IPyIntervu-modes-shared.md"},
	}

	b.WriteString("IPyIntervu Base Instructions (cached):\n\n")
	for _, file := range baseFiles {
		data, err := instructionFS.ReadFile(file.path)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", file.path, err)
		}
		b.WriteString("=== ")
		b.WriteString(file.displayName)
		b.WriteString(" ===\n")
		b.Write(data)
		b.WriteString("\n\n")
	}

	return b.String(), nil
}

func buildDynamicPrompt(state *AgentSessionState) (string, []string, error) {
	// Dynamic content: mode-specific and week-specific files + session state
	// Sent with each request (after base bundle is cached)
	var b strings.Builder

	// Session state (includes personaNames and currentWeekConcepts)
	b.WriteString("IPyIntervu server-managed session state (authoritative; instruction modules are subordinate):\n")
	stateJSON, err := json.MarshalIndent(state.snapshotForPrompt(), "", "  ")
	if err != nil {
		return "", nil, err
	}
	b.Write(stateJSON)
	b.WriteString("\n\n")

	// Include allowed concepts markdown
	if state.CurrentWeekConcepts != "" {
		b.WriteString(state.CurrentWeekConcepts)
		b.WriteString("\n")
	}

	// Dynamic instruction files (mode-specific and week-specific only)
	var dynamicFiles []promptFile
	var bundleParts []string

	// Add mode-specific file if in assessment
	if state.ConversationPhase == phaseAssessmentInProgress || state.ConversationPhase == phaseAssessmentResults {
		switch state.ActiveMode {
		case modeConceptual:
			dynamicFiles = append(dynamicFiles, promptFile{"IPyIntervu-modes-conceptual.md", "env/instructions/IPyIntervu-modes-conceptual.md"})
			bundleParts = append(bundleParts, "mode-conceptual")
		case modeCode:
			dynamicFiles = append(dynamicFiles, promptFile{"IPyIntervu-modes-code.md", "env/instructions/IPyIntervu-modes-code.md"})
			bundleParts = append(bundleParts, "mode-code")
		case modeBug:
			dynamicFiles = append(dynamicFiles, promptFile{"IPyIntervu-modes-bug.md", "env/instructions/IPyIntervu-modes-bug.md"})
			bundleParts = append(bundleParts, "mode-bug")
		}
	}

	if state.ActiveMode == modeCoaching {
		dynamicFiles = append(dynamicFiles, promptFile{"IPyIntervu-modes-coaching.md", "env/instructions/IPyIntervu-modes-coaching.md"})
		bundleParts = append(bundleParts, "coaching")
	}

	// Add week-specific files
	if state.CurrentWeekNumber > 0 {
		week := state.CurrentWeekNumber
		dynamicFiles = append(dynamicFiles,
			promptFile{fmt.Sprintf("week%d_key_concepts.md", week), fmt.Sprintf("env/IPYIntervu_support_files/week%d_key_concepts.md", week)},
			promptFile{fmt.Sprintf("week%d_competency_guide.md", week), fmt.Sprintf("env/IPYIntervu_support_files/week%d_competency_guide.md", week)},
			promptFile{fmt.Sprintf("week%d_rubric.md", week), fmt.Sprintf("env/rubrics/week%d_rubric.md", week)},
		)
		bundleParts = append(bundleParts, fmt.Sprintf("week%d-kb", week))
	}

	// Add final assessment rubric if needed
	if state.ConversationPhase == phaseAssessmentResults || state.AssessmentComplete {
		dynamicFiles = append(dynamicFiles, promptFile{"final_assessment_rubric.md", "env/rubrics/final_assessment_rubric.md"})
		bundleParts = append(bundleParts, "results")
	}

	// Load and append dynamic files
	loaded := make([]string, 0, len(dynamicFiles))
	if len(dynamicFiles) > 0 {
		b.WriteString("Dynamic knowledge-base files for this turn:\n\n")
		for _, file := range dynamicFiles {
			data, err := instructionFS.ReadFile(file.path)
			if err != nil {
				return "", nil, fmt.Errorf("read %s: %w", file.path, err)
			}
			b.WriteString("=== ")
			b.WriteString(file.displayName)
			b.WriteString(" ===\n")
			b.Write(data)
			b.WriteString("\n\n")
			loaded = append(loaded, file.displayName)
		}
	}

	state.KBFilesLoaded = loaded
	return b.String(), loaded, nil
}

func buildSystemPrompt(state *AgentSessionState) (string, []string, string, error) {
	// Build prompt with dynamic content only (base bundle already cached at bootstrap)
	var b strings.Builder

	// Add directives that apply to every request
	b.WriteString("ASSESSMENT SYNC (mandatory): Every Conceptual/Code/Bug reply MUST end with ```_ipyintervu``` JSON as the absolute last lines — introductions, acknowledgments (Got it./Thanks.), follow-ups, and mode handoffs. A reply without the fence is incomplete even when the interview text looks done. Do not stop generating until the closing ``` fence is written. Missing sync triggers a server corrective retry and may fail closed after one retry.\n")
	b.WriteString("Use assessmentPhase \"in_progress\" while asking interview questions (omit bucket). Use assessmentPhase \"complete\" plus the bucket when finishing that mode. Mode transitions require complete plus a valid bucket.\n")
	if syncLine := assessmentSyncPromptForState(state); syncLine != "" {
		b.WriteString(syncLine)
	}
	if scope := weekScopeTurnDirective(state); scope != "" {
		b.WriteString(scope)
	}
	if directive := followUpTurnDirective(state); directive != "" {
		b.WriteString(directive)
	}
	b.WriteString("Assessment modes advance forward only (Conceptual → Code → Bug). Never return to a completed or earlier mode; follow activeMode in server state.\n")
	b.WriteString("During assessment (coachingRequested false): never offer explanations, walkthroughs, hints that reveal answers, or coaching. Only Coaching mode may explain or teach.\n")
	if state.ConversationPhase == phaseAssessmentInProgress {
		b.WriteString("Never answer your own questions: ask one question, append the silent _ipyintervu block, then STOP. Do not supply the answer, model response, solution code, or the bug/fix, and never write or simulate the student's reply. Wait for an actual user message before continuing.\n")
		b.WriteString("Ask exactly ONE interview question per reply. Never stack multiple questions, repeat the same question in different words, or combine several acknowledgments with several questions in one message.\n")
	}
	if state.ActiveMode == modeCode && state.ConversationPhase == phaseAssessmentInProgress && !state.isProblemDecompositionWeek() {
		b.WriteString("CODE PROBLEM SEQUENCE: (1) task decomposition question, (2) after student answers ask them to paste Python code, (3) evaluate pasted code and explain/AI questions, (4) complete plus bucket. Never complete after decomposition alone.\n")
	}
	if state.ActiveMode == modeBug && state.ConversationPhase == phaseAssessmentInProgress {
		b.WriteString("Bug Hunting: ask how the student would find the bug (process only). Do not ask for corrected/fixed code. Do not answer your own debugging questions.\n")
	}
	b.WriteString("\n")

	// Add dynamic content (mode-specific and week-specific files + session state)
	dynamicPrompt, loaded, err := buildDynamicPrompt(state)
	if err != nil {
		return "", nil, "", err
	}
	b.WriteString(dynamicPrompt)

	return b.String(), loaded, "", nil
}
