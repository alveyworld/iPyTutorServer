package main

import (
	"fmt"
	"log"
	"strings"
)

type assessmentViolations struct {
	Content               bool
	SevereContent         bool
	MissingSync           bool
	CompleteWithoutBucket bool
	// MissingSyncClosing: a follow-up reply with no sync block and no question —
	// almost always the mode's closing reply with the complete+bucket fence dropped.
	MissingSyncClosing bool
	// RepeatedQuestion: a follow-up reply re-asks the question the student just answered.
	RepeatedQuestion bool
	// ClosingOverdue: the closing reply was due (see modeClosingDue) but the reply
	// kept interviewing instead of sending complete plus bucket.
	ClosingOverdue bool
	// OutOfScope lists later-week concepts the reply used (see outOfScopeConcepts).
	OutOfScope []string
	// NoQuestion: a reply that keeps interviewing (sync block present, phase not finished)
	// but asks the student nothing — e.g. an explanation of the concept.
	NoQuestion bool
	// UnfilledPlaceholder: a template placeholder such as "[companyName]" was left in the reply.
	UnfilledPlaceholder bool
	// AddressesPersona: the reply greets or questions an interviewer as if the student were one.
	AddressesPersona bool
	// ReasoningLeak: the reply contains the model's grading notes ("the student", "rubric").
	ReasoningLeak bool
	// InstructorFraming: the reply spoke as an instructor or referred to the course.
	InstructorFraming bool
}

func (v assessmentViolations) Any() bool {
	return v.Content || v.SevereContent || v.MissingSync || v.CompleteWithoutBucket || v.RepeatedQuestion || v.ClosingOverdue || len(v.OutOfScope) > 0 || v.NoQuestion || v.UnfilledPlaceholder || v.AddressesPersona || v.ReasoningLeak || v.InstructorFraming
}

func (v assessmentViolations) NeedsTruncateOnly() bool {
	return v.Content && !v.SevereContent && !v.MissingSync && !v.CompleteWithoutBucket && !v.RepeatedQuestion && !v.ClosingOverdue && len(v.OutOfScope) == 0 && !v.NoQuestion && !v.UnfilledPlaceholder && !v.AddressesPersona && !v.ReasoningLeak && !v.InstructorFraming
}

func (v assessmentViolations) NeedsCorrectiveRetry() bool {
	return v.MissingSync || v.CompleteWithoutBucket || v.SevereContent || v.RepeatedQuestion || v.ClosingOverdue || len(v.OutOfScope) > 0 || v.NoQuestion || v.UnfilledPlaceholder || v.AddressesPersona || v.ReasoningLeak || v.InstructorFraming
}

func (v assessmentViolations) StillInvalidAfterRetry() bool {
	return v.Any()
}

func assessmentViolationCheckEnabled(state *AgentSessionState) bool {
	return questionGuardEnabledForState(state)
}

func detectAssessmentViolations(state *AgentSessionState, assistant string) assessmentViolations {
	var v assessmentViolations
	if !assessmentViolationCheckEnabled(state) {
		return v
	}

	trimmed := strings.TrimSpace(stripIPyIntervuTail(assistant))
	// A reply that asks a question without a sync block is implicitly in_progress:
	// nothing is lost, so it is not a violation. Prior assistant turns reach the model
	// with their fences stripped, so it drops the fence often; retrying every such
	// reply burned the one corrective retry and failed the turn closed. Only a
	// question-less reply without the block loses state (a closing reply's bucket).
	if !hasIPyIntervuBlock(assistant) && !strings.Contains(trimmed, "?") {
		v.MissingSync = true
		v.MissingSyncClosing = isFollowUpAssessmentTurn(state)
	}
	if shouldForceAssessmentSync(state, assistant) {
		v.CompleteWithoutBucket = true
	}
	if looksLikeSelfAnsweredQuestion(trimmed) || looksLikeCompositeAssessmentReply(trimmed) {
		v.Content = true
		if looksLikeSevereCompositeReply(trimmed) {
			v.SevereContent = true
		}
	}
	if looksLikeEvaluationOrStagedTurn(trimmed) {
		v.Content = true
		v.SevereContent = true
	}
	if repeatsAnsweredQuestion(state, assistant) {
		v.RepeatedQuestion = true
	}
	if modeClosingDue(state) && (strings.Contains(trimmed, "?") || looksLikeCodeRequest(strings.ToLower(trimmed))) {
		v.ClosingOverdue = true
	}
	// Without a sync block, a question-less reply is already MissingSync.
	if hasIPyIntervuBlock(assistant) && !asksStudentSomething(trimmed) {
		v.NoQuestion = true
	}
	v.UnfilledPlaceholder = hasUnfilledPlaceholder(trimmed)
	v.AddressesPersona = addressesPersona(trimmed)
	v.ReasoningLeak = leaksReasoning(trimmed)
	v.InstructorFraming = speaksAsInstructor(state, trimmed)
	v.OutOfScope = outOfScopeConcepts(state.CurrentWeekNumber, clientVisibleAssistantContent(assistant))
	return v
}

func logAssessmentViolations(sessionID, turnID string, modeTurn int, v assessmentViolations) {
	if !v.Any() {
		return
	}
	log.Printf("[openrouter] assessment_violations session=%s turn_id=%s mode_turn=%d content=%v severe_content=%v missing_sync=%v complete_without_bucket=%v repeated_question=%v closing_overdue=%v out_of_scope=%q no_question=%v unfilled_placeholder=%v addresses_persona=%v reasoning_leak=%v instructor_framing=%v",
		truncateSessionID(sessionID), truncateTurnID(turnID), modeTurn, v.Content, v.SevereContent, v.MissingSync, v.CompleteWithoutBucket, v.RepeatedQuestion, v.ClosingOverdue, v.OutOfScope, v.NoQuestion, v.UnfilledPlaceholder, v.AddressesPersona, v.ReasoningLeak, v.InstructorFraming)
}

func buildViolationIssueText(v assessmentViolations, phaseField, bucketField string) string {
	var issues []string
	if v.Content {
		issues = append(issues, "answered your own question, explained or graded the student's answer, simulated the student's reply, stacked multiple interview questions, or continued as if the student already responded")
	}
	if v.NoQuestion {
		issues = append(issues, "asked the student no interview question (a question addressed to another interviewer does not count)")
	}
	if v.UnfilledPlaceholder {
		issues = append(issues, "left a template placeholder such as [companyName] instead of the real name")
	}
	if v.AddressesPersona {
		issues = append(issues, "spoke to another interviewer (e.g. \"Hey Taylor!\") instead of to the student")
	}
	if v.InstructorFraming {
		issues = append(issues, "spoke like an instructor or referred to the course (weeks, class, homework, or what the student has learned) instead of as a job interviewer at the company")
	}
	if v.ReasoningLeak {
		issues = append(issues, "included your private grading notes (references to \"the student\", the rubric, or the portion) in the reply")
	}
	if v.RepeatedQuestion {
		issues = append(issues, "re-asked a question the student had already answered")
	}
	if v.MissingSync {
		issues = append(issues, "omitted the required ```_ipyintervu``` sync block")
	}
	if v.CompleteWithoutBucket {
		issues = append(issues, "did not send \""+phaseField+"\": \"complete\" with "+bucketField)
	}
	return strings.Join(issues, "; ")
}

func assessmentFinishRule(phaseField, bucketField string) string {
	return "While still interviewing: {\"" + phaseField + "\": \"in_progress\"} with no bucket. When finishing this mode: {\"" + phaseField + "\": \"complete\", \"" + bucketField + "\": \"Not Ready Yet\"|\"Competent\"|\"Exceptional\"}."
}

// correctiveNoteRule: handoffs reach the model as user-role messages, so without this it
// thanks the "student" for them ("I appreciate you providing the scenario context").
const correctiveNoteRule = "This bracketed note is from the server, not the student: do not thank, acknowledge, or reply to it, and do not re-introduce yourselves. Speak as job interviewers at the company, never as instructors: do not mention weeks, the course, class, homework, or what the student has learned. "

func buildRewindCorrectiveHandoff(state *AgentSessionState, label, issueText, finishRule string) string {
	_ = state
	return "[System: Your last " + label + " reply violated assessment protocol because it " + issueText + ". Re-present the SAME concrete scenario (or code task / bug snippet) from your previous reply — do not drop or shorten it. Then one brief neutral lead-in (optional) plus exactly ONE student-directed interview question, OR (if this mode is finished) one brief neutral closing sentence with no new question. Do NOT write what the student would say, do not put an answer on the line after your question, and do not stack acknowledgments with extra questions. " + correctiveNoteRule + "Do NOT mention re-presenting, correcting, server retries, or [System] in user-facing text — continue in persona voice only. End with ```_ipyintervu``` as the last lines. " + finishRule + " Use exact bucket strings. Then stop and wait for the student unless you sent complete plus bucket.]"
}

// buildForwardContentCorrectiveHandoff corrects a stacked or self-answered reply after the
// student has already answered. Rewinding to the scenario here restarts the interview.
// buildOpeningCorrectiveHandoff redoes a phase's first reply that never presented a
// scenario and question, where "re-present the same scenario" has nothing to re-present.
func buildOpeningCorrectiveHandoff(label, issueText, finishRule string) string {
	return "[System: Your last " + label + " reply did not open this part of the interview: it " + issueText + ". Discard it. Write the first reply now: the interviewer introduces themselves in one or two sentences as an employee at a company in the student's major, presents ONE concrete scenario (or code task / bug snippet for this part), and asks the student exactly ONE interview question about it — addressed to the student, never to another interviewer (no 'Hey Taylor!'), and do not hand off to or invite a colleague to begin. Write only what the student should read — no grading notes or reasoning about the student. Use the real company name you choose; never leave a bracketed placeholder such as [companyName]. Do not explain concepts, define terms, or give examples. " + correctiveNoteRule + "Do NOT mention correcting, server retries, or [System] in user-facing text — continue in persona voice only. End with ```_ipyintervu``` as the last lines. " + finishRule + " Then stop and wait for the student.]"
}

func buildForwardContentCorrectiveHandoff(state *AgentSessionState, label, issueText, finishRule string) string {
	forward := forwardInterviewMoveGuidance(state)
	if len(state.ModeQuestionsAsked) > 0 {
		forward = "The student already answered " + quotedQuestions(state.ModeQuestionsAsked) + " — do not ask any of them again in any wording. " + forward
	}
	return "[System: Your last " + label + " reply violated assessment protocol because it " + issueText + ". Discard that reply. The student's most recent message is their real answer to your previous question — respond to it. Do NOT re-present the opening scenario, restart the interview, or re-ask a question the student already answered. " + forward + " Optional brief neutral lead-in (Got it./Thanks.) plus exactly ONE new interview question, OR (if this mode is finished) one brief neutral closing sentence with no new question. Do NOT write what the student would say, and do NOT praise, say whether the answer was right, or explain the answer. Use only values the student actually gave. " + correctiveNoteRule + "Do NOT mention correcting, server retries, or [System] in user-facing text — continue in persona voice only. End with ```_ipyintervu``` as the last lines. " + finishRule + " Use exact bucket strings. Then stop and wait for the student unless you sent complete plus bucket.]"
}

func buildForwardMissingSyncCorrectiveHandoff(state *AgentSessionState, label, finishRule string) string {
	forward := forwardInterviewMoveGuidance(state)
	return "[System: Your last " + label + " reply omitted the required ```_ipyintervu``` sync block. The student already answered your previous question — do NOT re-present the opening scenario or repeat the same question. " + forward + " Optional brief neutral lead-in (Got it./Thanks.) plus exactly ONE new interview question, OR (if this mode is finished) one brief neutral closing sentence with no new question. " + correctiveNoteRule + "Do NOT mention re-presenting, correcting, server retries, or [System] in user-facing text — continue in persona voice only. End with ```_ipyintervu``` as the last lines. " + finishRule + " Use exact bucket strings. Then stop and wait for the student unless you sent complete plus bucket.]"
}

func buildClosingMissingSyncCorrectiveHandoff(label, phaseField, bucketField, finishRule string) string {
	return "[System: Your last " + label + " reply asked no interview question and omitted the required ```_ipyintervu``` sync block, so the server could not record it. If that reply was closing this portion, send one brief neutral closing sentence — no new question and no new scenario — and end with ```_ipyintervu``` containing \"" + phaseField + "\": \"complete\" and \"" + bucketField + "\" based on the student's answers so far. Only if you genuinely need more evidence, ask exactly ONE follow-up question about the SAME scenario and use \"in_progress\". " + correctiveNoteRule + "Do NOT mention correcting, server retries, or [System] in user-facing text — continue in persona voice only. " + finishRule + " Use exact bucket strings. Then stop.]"
}

func buildClosingOverdueHandoff(state *AgentSessionState, label, phaseField, bucketField, finishRule string) string {
	return "[System: Your last " + label + " reply asked another question, but the interview is finished: " + closingDueReason(state) + ". Discard that reply. Ask NO question and present no scenario. Send one brief neutral closing sentence and end with ```_ipyintervu``` containing \"" + phaseField + "\": \"complete\" and \"" + bucketField + "\" based on all of the student's answers. " + correctiveNoteRule + "Do NOT mention correcting, server retries, or [System] in user-facing text. " + finishRule + " Use exact bucket strings. Then stop.]"
}

func buildOutOfScopeHandoff(state *AgentSessionState, found []string, label, finishRule string) string {
	position := "Keep your introduction and scenario, but rewrite any part of the scenario, task, code, or question that needs those concepts."
	if isFollowUpAssessmentTurn(state) {
		position = "The student's most recent message is their real answer to your previous question — respond to it and do NOT re-present the opening scenario or restart the interview. " + forwardInterviewMoveGuidance(state)
	}
	return "[System: Your last " + label + " reply used concepts the student has not learned yet: " + strings.Join(found, ", ") + ". Discard that reply. The student is being assessed on week " + fmt.Sprint(state.CurrentWeekNumber) + " and has learned only: " + learnedConceptsSummary(state.CurrentWeekNumber) + ". Write the reply again so that nothing in it needs or mentions a later-week concept — not even to tell the student not to use it. " + position + " Optional brief neutral lead-in plus exactly ONE interview question. Do NOT praise or explain the answer. " + correctiveNoteRule + "Do NOT mention correcting, server retries, or [System] in user-facing text — continue in persona voice only. End with ```_ipyintervu``` as the last lines. " + finishRule + " Use exact bucket strings. Then stop and wait for the student unless you sent complete plus bucket.]"
}

func buildCompleteWithoutBucketHandoff(state *AgentSessionState, label, issueText, finishRule string) string {
	_ = state
	return "[System: Your last " + label + " reply violated assessment protocol because it " + issueText + ". If you are finishing this mode, send one brief neutral closing sentence with no new question and end with ```_ipyintervu``` containing \"complete\" plus the bucket in the same reply. If you are still interviewing, use \"in_progress\" only. " + correctiveNoteRule + "Do NOT mention correcting, server retries, or [System] in user-facing text. " + finishRule + " Use exact bucket strings. Then stop.]"
}

func buildUnifiedCorrectiveHandoff(state *AgentSessionState, v assessmentViolations) string {
	phaseField, bucketField, label := currentModeSyncFields(state)
	if phaseField == "" {
		return ""
	}

	issueText := buildViolationIssueText(v, phaseField, bucketField)
	finishRule := assessmentFinishRule(phaseField, bucketField)

	if v.CompleteWithoutBucket {
		return buildCompleteWithoutBucketHandoff(state, label, issueText, finishRule)
	}
	if v.ClosingOverdue {
		return buildClosingOverdueHandoff(state, label, phaseField, bucketField, finishRule)
	}
	if len(v.OutOfScope) > 0 {
		return buildOutOfScopeHandoff(state, v.OutOfScope, label, finishRule)
	}
	if v.NoQuestion && modeClosingDue(state) {
		return buildClosingMissingSyncCorrectiveHandoff(label, phaseField, bucketField, finishRule)
	}
	if (v.NoQuestion || v.UnfilledPlaceholder || v.AddressesPersona || v.ReasoningLeak || v.InstructorFraming) && !isFollowUpAssessmentTurn(state) {
		return buildOpeningCorrectiveHandoff(label, issueText, finishRule)
	}
	if (v.Content || v.RepeatedQuestion || v.NoQuestion || v.UnfilledPlaceholder || v.AddressesPersona || v.ReasoningLeak || v.InstructorFraming) && isFollowUpAssessmentTurn(state) {
		return buildForwardContentCorrectiveHandoff(state, label, issueText, finishRule)
	}
	if v.Content {
		return buildRewindCorrectiveHandoff(state, label, issueText, finishRule)
	}
	if v.MissingSyncClosing {
		return buildClosingMissingSyncCorrectiveHandoff(label, phaseField, bucketField, finishRule)
	}
	if v.MissingSync && isFollowUpAssessmentTurn(state) {
		return buildForwardMissingSyncCorrectiveHandoff(state, label, finishRule)
	}
	if v.MissingSync {
		return buildRewindCorrectiveHandoff(state, label, issueText, finishRule)
	}
	return ""
}

func buildServerAssessmentResultsMessage(state *AgentSessionState) string {
	conceptual := state.ConceptualAssessmentBucket
	code := state.CodeAssessmentBucket
	bug := state.BugAssessmentBucket
	if state.isProblemDecompositionWeek() {
		code = bucketNA
		bug = bucketNA
	}
	if code == "" {
		code = bucketNA
	}
	if bug == "" {
		bug = bucketNA
	}
	selected := state.SelectedKeyConcept
	if selected == "" {
		selected = "your selected key concept"
	}
	return strings.Join([]string{
		"Assessment Results — " + selected,
		"",
		"- Conceptual Assessment: " + conceptual,
		"- Code Assessment: " + code,
		"- Bug Assessment: " + bug,
		"",
		"Overall Rating: " + state.FinalRating,
		"",
		"If you would like to see how to improve your assessment, type 'switch to coach mode'.",
	}, "\n")
}

func buildAssessmentTurnFailureMessage() string {
	return "I'm sorry — I hit a temporary issue finishing that assessment step. Please send your last answer again or ask to continue, and we'll pick up from here."
}
