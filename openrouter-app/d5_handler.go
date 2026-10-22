package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	d5BriefWait     = 2 * time.Second
	d5LabelJobsWait = 9 * time.Second
)

// handleD5Chat runs one /api/chat turn on the D5 engine (D5-interviewer-evaluator-design.md).
// The request and response contract is the original one, so the UI is unchanged.
// skipPreChat is set when a failed turn is resumed: its message was already applied.
func handleD5Chat(p chatRunParams, skipPreChat bool) {
	state := p.state
	sess := d5SessionFor(state, p.sessionID)
	sess.turnMu.Lock()
	defer sess.turnMu.Unlock()
	defer p.states.set(p.sessionID, state)

	msg := strings.TrimSpace(p.userMessage)
	phaseBefore := state.ConversationPhase

	// A session that never got the welcome but arrives with an existing conversation was
	// lost in a server restart (state is in memory; the login cookie survives). Taking the
	// student's answer as their major would silently restart the interview.
	if phaseBefore == phaseAwaitingMajor && !state.StartupPromptShown && d5HasPriorConversation(p.req.Messages) {
		log.Printf("[d5] session_lost session=%s turn_id=%s", truncateSessionID(p.sessionID), truncateTurnID(p.turnID))
		d5Respond(p, d5SessionLostMessage)
		return
	}

	// Setup is server-authored, exactly as in the original engine.
	if phaseBefore == phaseAwaitingMajor || phaseBefore == phaseAwaitingKeyConcept {
		if !skipPreChat {
			applyPreChatUserUpdate(state, msg)
		}
		if reply, ok := serverSetupReply(phaseBefore, state); ok {
			d5Respond(p, reply)
			return
		}
		// A week was chosen: the assessment starts with the first opening.
		sess.Personas = pickD5Personas(p.sessionID)
		d5RunOpening(p, sess, "")
		return
	}

	if !skipPreChat {
		state.MessageIndex++
		state.LastUserMessageRaw = msg
	}

	if state.ConversationPhase == phaseAssessmentResults {
		d5ResultsTurn(p, sess, msg, skipPreChat)
		return
	}
	if state.ConversationPhase != phaseAssessmentInProgress {
		d5Respond(p, setupReaskMajorMessage)
		return
	}

	if sess.NeedsOpening {
		d5RunOpening(p, sess, "")
		return
	}
	if isMidInterviewCoachingRequest(msg) {
		reply := d5CoachingAfterwards
		if q := d5PendingQuestion(sess); q != "" {
			reply += " To pick up where we left off: " + q
		}
		d5Respond(p, reply)
		return
	}

	brief := d5AwaitBrief(p, sess)
	clarification := d5AskedForClarification(msg)
	if !clarification && !skipPreChat {
		d5RecordAnswer(state, sess, msg)
		log.Printf("[d5] answer session=%s mode=%s answer_index=%d words=%d vague=%v code=%v",
			truncateSessionID(p.sessionID), state.ActiveMode, sess.AnswerIndex, len(strings.Fields(msg)),
			sess.VagueAnswers[sess.AnswerIndex], d5LooksLikeCodeSubmission(msg))
	} else if clarification && !skipPreChat {
		sess.Transcript = append(sess.Transcript, d5Message{Role: "user", Content: msg, Mode: state.ActiveMode})
	}
	answerIndex, answerTarget := sess.AnswerIndex, sess.LastAskTarget

	move := d5ChooseMove(state, sess, brief, msg, clarification)
	log.Printf("[d5] move session=%s turn_id=%s week=%d mode=%s move=%s reason=%q answer_index=%d",
		truncateSessionID(p.sessionID), truncateTurnID(p.turnID), state.CurrentWeekNumber, state.ActiveMode, move.Kind, move.Reason, answerIndex)

	if move.Kind == moveCloseMode {
		d5CloseMode(p, sess)
		return
	}

	reply := d5Interview(p, sess, move, brief)
	d5Respond(p, reply)
	if move.Kind != moveClarify {
		d5LaunchEvaluator(p, sess, answerIndex, answerTarget)
	}
}

const d5SessionLostMessage = "Sorry — the interview server restarted and this interview's progress was lost. Please reload the page to start a new interview."

// d5HasPriorConversation reports a request carrying earlier interview turns: any
// assistant message, or more than one user message.
func d5HasPriorConversation(msgs []chatMessage) bool {
	users := 0
	for _, m := range msgs {
		if m.Role == "assistant" && strings.TrimSpace(m.Content) != "" {
			return true
		}
		if m.Role == "user" {
			users++
		}
	}
	return users > 1
}

// d5AwaitBrief waits up to 2 s for an in-flight Evaluator brief, then returns the newest
// brief available (design §9).
func d5AwaitBrief(p chatRunParams, sess *d5Session) *d5Brief {
	sess.mu.Lock()
	ch := sess.briefInFlight
	sess.mu.Unlock()
	if ch != nil {
		started := time.Now()
		ready := true
		select {
		case <-ch:
		default:
			select {
			case <-ch:
			case <-time.After(d5BriefWait):
				ready = false
			}
		}
		log.Printf("[d5] brief_wait session=%s ready_before_next=%v waited_ms=%d",
			truncateSessionID(p.sessionID), ready, time.Since(started).Milliseconds())
	}
	return sess.latestBrief()
}

// d5Interview makes the live Interviewer call for a move, applies the post-checks, falls
// back to a fixed question on failure, and records the delivered reply.
func d5Interview(p chatRunParams, sess *d5Session, move d5Move, brief *d5Brief) string {
	state := p.state
	mode := state.ActiveMode
	system := d5InterviewerSystemPrompt(state, sess, mode, move, brief)
	req := interviewerRequest(p.req.Model, d5InterviewerMessages(system, sess, mode, false), move.MaxTokens)
	res, err := runInterviewerCall(p.apiKey, p.sessionID, req)
	fallback := d5FallbackQuestion(state, sess, brief, move)

	var reply string
	var issues []string
	if err != nil {
		log.Printf("[d5] interviewer_failed session=%s turn_id=%s move=%s attempts=%d err=%q",
			truncateSessionID(p.sessionID), truncateTurnID(p.turnID), move.Kind, res.Attempts, err.Error())
		reply = d5FallbackLeadIn + " " + fallback
		if move.Kind == moveClarify || move.Kind == moveRequestCode {
			reply = fallback
		}
	} else {
		checked := d5PostCheck(state, sess, mode, move, res.Content, fallback)
		reply, issues = checked.Reply, checked.Issues
	}
	d5LogInterviewer(p, move, res, len(issues))
	if len(issues) > 0 {
		log.Printf("[d5] postcheck session=%s turn_id=%s move=%s issues=%q",
			truncateSessionID(p.sessionID), truncateTurnID(p.turnID), move.Kind, issues)
	}
	sess.PendingIssues = issues
	d5ApplyInterviewerReply(state, sess, move, reply)
	return reply
}

func d5LogInterviewer(p chatRunParams, move d5Move, res d5CallResult, issues int) {
	log.Printf("[d5] interviewer_done session=%s turn_id=%s move=%s ms=%d ttft_ms=%d in_tokens=%d out_tokens=%d reasoning_tokens=%d cached_tokens=%d provider=%s attempts=%d cut=%v issues=%d",
		truncateSessionID(p.sessionID), truncateTurnID(p.turnID), move.Kind, res.Elapsed.Milliseconds(), res.TTFT.Milliseconds(),
		res.Usage.PromptTokens, res.Usage.CompletionTokens, res.Usage.CompletionTokensDetails.ReasoningTokens,
		res.Usage.PromptTokensDetails.CachedTokens, res.Provider, res.Attempts, res.Cut, issues)
}

// d5RunOpening opens the active mode with a live call (Phase 1; pre-drafted openings are
// Phase 2). prefix is sent before the introduction (the previous mode's closing line).
func d5RunOpening(p chatRunParams, sess *d5Session, prefix string) {
	state := p.state
	mode := state.ActiveMode
	move := d5OpeningMove(state, sess, mode)
	var draft d5OpeningDraft
	if prepared := d5TakePrepared(p, sess, mode); prepared != nil {
		draft = d5OpeningDraft{res: d5CallResult{Content: prepared.Content}, verifiedDefect: prepared.Verified, reasoned: true}
	} else {
		draft = d5DraftOpening(p, sess, mode, move)
	}
	res, err := draft.res, draft.err

	content := strings.TrimSpace(res.Content)
	if err == nil && move.Kind == moveOpenFirst {
		name, domain, rest, ok := parseCompanyLine(content)
		if ok {
			sess.CompanyName, sess.CompanyDomain = name, domain
		} else {
			log.Printf("[d5] company_line_missing session=%s", truncateSessionID(p.sessionID))
		}
		content = rest
	}
	var defect string
	if mode == modeBug {
		content, defect = splitDefectLine(content)
		// A live (no-reasoning) draft's own DEFECT line was often wrong, so the checker's
		// description of the actual bug is used. A reasoned draft's own line was reliable
		// and the checker's occasionally was not, so it is kept.
		if draft.verifiedDefect != "" && (!draft.reasoned || defect == "") {
			defect = draft.verifiedDefect
		}
		// The marked line is what the candidate sees; it leads, with the description after it.
		if marked := d5MarkedBugLine(content); marked != "" {
			defect = "the marked line `" + marked + "`; " + defect
		}
	}
	if err != nil || content == "" || (mode == modeBug && !strings.Contains(content, "```")) {
		log.Printf("[d5] opening_failed session=%s mode=%s err=%v", truncateSessionID(p.sessionID), mode, err)
		sess.NeedsOpening = true
		reply := "Sorry — I couldn't get the next part of the interview ready just now. Please send any message when you're ready to continue."
		if prefix != "" {
			reply = prefix + "\n\n" + reply
		}
		d5Respond(p, reply)
		return
	}
	if mode == modeBug {
		sess.BugDefect = defect
		log.Printf("[d5] bug_defect session=%s defect=%q", truncateSessionID(p.sessionID), truncateSummary(defect, 200))
	}
	sess.NeedsOpening = false
	sess.Material[mode] = content
	checked := d5PostCheck(state, sess, mode, move, content, "")
	if len(checked.Issues) > 0 {
		log.Printf("[d5] postcheck session=%s move=%s issues=%q", truncateSessionID(p.sessionID), move.Kind, checked.Issues)
	}
	sess.PendingIssues = checked.Issues
	opening := sess.personaIntro(mode) + "\n\n" + checked.Reply
	d5ApplyInterviewerReply(state, sess, move, opening)
	if prefix != "" {
		opening = prefix + "\n\n" + opening
	}
	d5Respond(p, opening)
	// Openings do not depend on earlier parts, so every later part is drafted now: the
	// Bug draft then has the whole Conceptual and Code parts to finish.
	for next := d5NextMode(state, mode); next != ""; next = d5NextMode(state, next) {
		sess.mu.Lock()
		_, started := sess.draftDone[next]
		sess.mu.Unlock()
		if !started {
			d5StartDraft(p, sess, next)
		}
	}
}

// d5OpeningProblems checks a Code or Bug opening before it is sent and returns the problems
// found (feedback for a regeneration) plus, for a Bug snippet the defect check confirmed,
// its description of the actual bug. A Code task must be a plain-words story problem; a
// Bug snippet must contain code and a clear hidden DEFECT line and really have a bug;
// both must pass the word-based scope check.
func d5OpeningProblems(p chatRunParams, mode, content string, callErr error, withReasoning bool) ([]string, string) {
	if callErr != nil || (mode != modeCode && mode != modeBug) || strings.TrimSpace(content) == "" {
		return nil, ""
	}
	var problems []string
	verified := ""
	if mode == modeBug {
		var defect string
		content, defect = splitDefectLine(content)
		switch {
		case !strings.Contains(content, "```"):
			problems = append(problems, "Your previous draft had no code snippet. Begin with the hidden DEFECT line, then describe the tool, show the snippet in a ```python block, and ask your question.")
		case defect == "":
			problems = append(problems, "Your previous draft did not begin with the hidden DEFECT line. Begin with \"DEFECT: <the bug, its line, and what goes wrong>\", then write a snippet that really contains that one bug.")
		case d5DefectLooksBad(defect):
			problems = append(problems, "Your previous DEFECT line showed the snippet had no clear, real bug. Decide on one real bug first, state it in the DEFECT line in one sentence, then write code that contains exactly that bug.")
		default:
			var noBug bool
			verified, noBug = d5DescribeDefect(p, content, defect, withReasoning)
			// Only a reasoning check may reject: without reasoning its "no bug" verdicts
			// were wrong too often to act on.
			if withReasoning && noBug {
				problems = append(problems, "Your previous code did not actually contain a bug. Write the snippet again so the code really has the one bug in your DEFECT line.")
			}
		}
	}
	if mode == modeBug && strings.Contains(content, "```") && !bugMarkerPattern.MatchString(content) {
		problems = append(problems, "Your previous snippet did not mark the bug. Put a trailing comment \"# Bug: <what is wrong>\" on the line where the bug is.")
	}
	if marked := d5MarkedBugLine(content); mode == modeBug && marked != "" && defectSelfAdmitPattern.MatchString(marked) {
		problems = append(problems, "Your previous \"# Bug:\" comment argued with itself or said the line was fine. Decide on one real bug first, then mark it with one short, certain comment.")
	}
	if lower := strings.ToLower(normalizeQuotes(content)); mode == modeCode && (!strings.Contains(lower, "data available") || !strings.Contains(lower, "what's wanted") || !strings.Contains(content, "?")) {
		problems = append(problems, "Your previous draft was not in the required form. Write a short story problem, then the \"Data available:\" and \"What's wanted:\" lines, then one question about how they would break the problem down.")
	}
	if mode == modeCode && d5OpeningGivesCode(content) {
		problems = append(problems, "Your previous draft contained code. Rewrite it with no code at all: describe the data and the result only in plain words.")
	}
	week := p.state.CurrentWeekNumber
	found := d5ScopeIssues(week, content, "")
	log.Printf("[d5] opening_check session=%s mode=%s week=%d problems=%d out_of_scope=%q",
		truncateSessionID(p.sessionID), mode, week, len(problems), found)
	if len(found) > 0 {
		problems = append(problems, "Your previous draft used things the candidate has not learned: "+strings.Join(found, ", ")+". Write a different one that uses none of them.")
	}
	return problems, verified
}

// d5OpeningDraft is the result of generating a mode's opening.
type d5OpeningDraft struct {
	res            d5CallResult
	err            error
	firstContent   string
	firstProblems  []string
	retryProblems  []string
	regenerated    bool
	keptFirst      bool
	verifiedDefect string
	// reasoned is set for a pre-drafted opening, written with reasoning on.
	reasoned bool
}

// d5DraftOpening generates a mode's opening, checks it, and regenerates once when it has
// problems. The regenerated draft is checked too, and the draft with fewer problems is kept
// (the regenerated one on a tie), so a false alarm cannot swap a good draft for a worse one.
func d5DraftOpening(p chatRunParams, sess *d5Session, mode string, move d5Move) d5OpeningDraft {
	state := p.state
	req := interviewerRequest(p.req.Model, d5InterviewerMessages(d5InterviewerSystemPrompt(state, sess, mode, move, nil), sess, mode, true), move.MaxTokens)
	res, err := runInterviewerCallWithin(p.apiKey, p.sessionID, req, d5OpeningTimeout)
	d5LogInterviewer(p, move, res, 0)
	d := d5OpeningDraft{res: res, err: err, firstContent: res.Content}
	d.firstProblems, d.verifiedDefect = d5OpeningProblems(p, mode, res.Content, err, false)
	if len(d.firstProblems) == 0 {
		return d
	}
	retryMove := move
	retryMove.Instruction += "\n" + strings.Join(d.firstProblems, "\n")
	retryReq := interviewerRequest(p.req.Model, d5InterviewerMessages(d5InterviewerSystemPrompt(state, sess, mode, retryMove, nil), sess, mode, true), move.MaxTokens)
	retry, retryErr := runInterviewerCallWithin(p.apiKey, p.sessionID, retryReq, d5OpeningTimeout)
	if retryErr != nil || strings.TrimSpace(retry.Content) == "" {
		d.keptFirst = true
		return d
	}
	d5LogInterviewer(p, retryMove, retry, 0)
	d.regenerated = true
	var retryVerified string
	d.retryProblems, retryVerified = d5OpeningProblems(p, mode, retry.Content, nil, false)
	if len(d.retryProblems) > len(d.firstProblems) {
		d.keptFirst = true
		log.Printf("[d5] opening_kept_first session=%s mode=%s first_problems=%d retry_problems=%d",
			truncateSessionID(p.sessionID), mode, len(d.firstProblems), len(d.retryProblems))
		return d
	}
	d.res, d.err, d.verifiedDefect = retry, nil, retryVerified
	return d
}

// d5DescribeDefect asks for an accurate description of the bug in a Bug snippet, for the
// Evaluator: the model's own DEFECT line was often inaccurate, the check's description
// usually right. It never rejects a snippet: over 82 live snippets, rejecting on its
// "no bug" verdicts did not reduce bug-free snippets (12% vs 15%), because without
// reasoning this model cannot reliably tell whether code is correct. withReasoning is for
// pre-drafted openings (Phase 2), off the live path. Returns the description ("" when
// none) and whether the check said the snippet has no bug.
func d5DescribeDefect(p chatRunParams, snippet, defect string, withReasoning bool) (actual string, noBug bool) {
	req := defectCheckRequest(p.req.Model, d5VerifyDefectMessages(snippet, defect))
	timeout := d5LevelsTimeout
	if withReasoning {
		req = withReasoningBudget(req)
		timeout = d5DraftTimeout
	}
	res, err := runBackgroundCall(p.apiKey, p.sessionID, req, timeout, 1)
	text := strings.TrimSpace(res.Content)
	verdict := strings.ToLower(strings.Trim(text, "\"'*` "))
	if err == nil && strings.HasPrefix(verdict, "yes") {
		if i := strings.Index(text, ":"); i >= 0 {
			actual = strings.TrimSpace(text[i+1:])
		}
	}
	noBug = err == nil && strings.HasPrefix(verdict, "no")
	log.Printf("[d5] defect_check session=%s reasoning=%v confirmed=%v ms=%d actual=%q err=%v",
		truncateSessionID(p.sessionID), withReasoning, actual != "", res.Elapsed.Milliseconds(), truncateSummary(actual, 160), err)
	return actual, noBug
}

// d5CloseMode closes the active mode: the final answer is labelled by the levels-only
// call, then either the next mode opens or Go grades and the results are sent.
func d5CloseMode(p chatRunParams, sess *d5Session) {
	state := p.state
	mode := state.ActiveMode
	final := d5FinalMode(state)

	// Label every answer in the mode that has no label yet: the final answer (briefs run a
	// turn behind) and any whose Evaluator run failed. Calls run in parallel; the final
	// mode waits for them because its results go out in this reply.
	for idx, a := range sess.unlabelledAnswers(mode) {
		req := levelsRequest(p.req.Model, d5LevelsMessages(state, mode, a.Target, a.Question, a.Answer, sess.Material[mode]))
		idx, a := idx, a
		sess.labelJobs.Add(1)
		go func() {
			defer sess.labelJobs.Done()
			res, err := runBackgroundCall(p.apiKey, p.sessionID, req, d5LevelsTimeout, 1)
			labels := parseTargetLevel(res.Content, mode, a.Target, idx)
			log.Printf("[d5] levels_done session=%s mode=%s answer_index=%d ms=%d labels=%d err=%v",
				truncateSessionID(p.sessionID), mode, idx, res.Elapsed.Milliseconds(), len(labels), err)
			if len(labels) == 0 && err == nil {
				log.Printf("[d5] levels_unparsed session=%s raw=%q", truncateSessionID(p.sessionID), truncateSummary(res.Content, 200))
			}
			sess.addLabels(mode, withSource(labels, labelSourceLevels))
		}()
	}

	// The Bug part is rated once from its whole conversation (see d5BugStrategyMessages).
	// The per-answer labels still run and are the fallback if this call fails.
	var bugLabel chan []gradeLabel
	if final && mode == modeBug {
		bugLabel = make(chan []gradeLabel, 1)
		idx := sess.lastAnswerIndex(modeBug)
		req := levelsRequest(p.req.Model, d5BugStrategyMessages(state, sess))
		sess.labelJobs.Add(1)
		go func() {
			defer sess.labelJobs.Done()
			res, err := runBackgroundCall(p.apiKey, p.sessionID, req, d5BugStrategyTimeout, 1)
			labels := parseTargetLevel(res.Content, modeBug, dimStrategy, idx)
			log.Printf("[d5] bug_strategy_done session=%s ms=%d labels=%d err=%v", truncateSessionID(p.sessionID), res.Elapsed.Milliseconds(), len(labels), err)
			bugLabel <- withSource(labels, labelSourceHolistic)
		}()
	}

	if final {
		d5WaitLabelJobs(p, sess)
		labels := sess.labelsSnapshot()
		for _, m := range []string{modeConceptual, modeCode, modeBug} {
			var parts []string
			for _, l := range labels[m] {
				parts = append(parts, fmt.Sprintf("#%d %s=%s (%s)", l.AnswerIndex, l.Dimension, l.Level, l.Source))
			}
			log.Printf("[d5] labels session=%s mode=%s labels=%q", truncateSessionID(p.sessionID), m, strings.Join(parts, ", "))
		}
		if bugLabel != nil {
			select {
			case l := <-bugLabel:
				if len(l) > 0 {
					log.Printf("[d5] bug_strategy session=%s level=%s per_answer_labels=%d", truncateSessionID(p.sessionID), l[0].Level, len(labels[modeBug]))
					labels[modeBug] = l
				}
			default:
			}
		}
		labels, dropped := sess.withoutOffTargetLabels(labels)
		if len(dropped) > 0 {
			var parts []string
			for _, l := range dropped {
				parts = append(parts, fmt.Sprintf("#%d %s=%s", l.AnswerIndex, l.Dimension, l.Level))
			}
			log.Printf("[d5] label_filter session=%s dropped_off_target=%q", truncateSessionID(p.sessionID), strings.Join(parts, ", "))
		}
		applyD5Grades(state, labels, sess.CodePasted)
		d5FinishAssessment(state)
		log.Printf("[d5] results session=%s conceptual=%q code=%q bug=%q overall=%q",
			truncateSessionID(p.sessionID), state.ConceptualAssessmentBucket, state.CodeAssessmentBucket, state.BugAssessmentBucket, state.FinalRating)
		reply := d5ClosingInterview + "\n\n" + buildServerAssessmentResultsMessage(state)
		sess.Transcript = append(sess.Transcript, d5Message{Role: "assistant", Content: reply, Mode: phaseAssessmentResults})
		d5Respond(p, reply)
		return
	}

	d5AdvanceMode(state)
	d5RunOpening(p, sess, phaseClosingMessage)
}

func d5WaitLabelJobs(p chatRunParams, sess *d5Session) {
	done := make(chan struct{})
	go func() {
		sess.labelJobs.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d5LabelJobsWait):
		log.Printf("[d5] label_jobs_timeout session=%s", truncateSessionID(p.sessionID))
	}
}

// d5QuestionBefore returns the interviewer question the latest answer in mode replied to.
func d5QuestionBefore(sess *d5Session, mode string) string {
	msgs := sess.modeMessages(mode)
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" {
			if q := lastQuestionSentence(msgs[i].Content); q != "" {
				return q
			}
			return msgs[i].Content
		}
	}
	return ""
}

// d5LaunchEvaluator starts the background brief for the latest answer (design §5).
func d5LaunchEvaluator(p chatRunParams, sess *d5Session, answerIndex int, answerTarget string) {
	state := p.state
	mode := state.ActiveMode
	if answerIndex == 0 || state.ConversationPhase != phaseAssessmentInProgress {
		return
	}
	vague := sess.VagueAnswers[answerIndex]
	prev := sess.latestBrief()
	msgs := d5EvaluatorMessages(state, sess, mode, answerIndex, answerTarget, prev, sess.PendingIssues)
	req := evaluatorRequest(p.req.Model, msgs)
	done := make(chan struct{})
	sess.mu.Lock()
	sess.briefInFlight = done
	sess.mu.Unlock()

	go func() {
		defer close(done)
		// One attempt: a retry doubles the wait, and missing labels are filled in at close.
		res, err := runBackgroundCall(p.apiKey, p.sessionID, req, d5EvaluatorTimeout, 1)
		brief, ok := parseBrief(res.Content, mode, answerIndex)
		log.Printf("[d5] evaluator_done session=%s mode=%s answer_index=%d ms=%d out_tokens=%d reasoning_tokens=%d attempts=%d parsed=%v recommend=%s levels=%d err=%v",
			truncateSessionID(p.sessionID), mode, answerIndex, res.Elapsed.Milliseconds(), res.Usage.CompletionTokens,
			res.Usage.CompletionTokensDetails.ReasoningTokens, res.Attempts, ok, brief.Recommend, len(brief.Levels), err)
		if err == nil && !ok {
			log.Printf("[d5] evaluator_unparsed session=%s raw=%q", truncateSessionID(p.sessionID), truncateSummary(res.Content, 300))
		}
		if err != nil || !ok {
			return
		}
		if !vague {
			sess.addLabels(mode, withSource(brief.Levels, labelSourceEvaluator))
		}
		sess.mu.Lock()
		defer sess.mu.Unlock()
		if sess.Brief == nil || answerIndex >= sess.Brief.AnswerIndex {
			sess.Brief = &brief
		}
		sess.ModeBriefs[mode] = &brief
		sess.Evidence[mode] = append(sess.Evidence[mode], brief.Evidence...)
	}()
}

func withSource(labels []gradeLabel, source string) []gradeLabel {
	out := make([]gradeLabel, len(labels))
	for i, l := range labels {
		l.Source = source
		out[i] = l
	}
	return out
}

// d5ResultsTurn handles messages after the results: coaching on request, otherwise a
// reminder of how to get it.
func d5ResultsTurn(p chatRunParams, sess *d5Session, msg string, skipPreChat bool) {
	state := p.state
	if state.ActiveMode != modeCoaching && !isCoachingRequest(msg) {
		d5Respond(p, d5ResultsReminder)
		return
	}
	firstCoaching := state.ActiveMode != modeCoaching
	state.ActiveMode = modeCoaching
	state.CoachingRequested = true
	if !skipPreChat {
		sess.Transcript = append(sess.Transcript, d5Message{Role: "user", Content: msg, Mode: modeCoaching})
	}

	msgs := []chatMessage{{Role: "system", Content: d5CoachingSystemPrompt(state, sess)}}
	history := sess.modeMessages(modeCoaching)
	if len(history) > 6 {
		history = history[len(history)-6:]
	}
	for _, m := range history {
		msgs = append(msgs, chatMessage{Role: m.Role, Content: m.Content})
	}
	req := interviewerRequest(p.req.Model, msgs, d5CoachingMaxTokens)
	res, err := runInterviewerCallWithin(p.apiKey, p.sessionID, req, d5CoachingTimeout)
	d5LogInterviewer(p, d5Move{Kind: moveCoaching}, res, 0)

	reply := strings.TrimSpace(res.Content)
	if err != nil || reply == "" {
		reply = "Sorry — I couldn't put my feedback together just now. Please ask again in a moment."
	} else if firstCoaching {
		reply = sess.personaIntro(modeCoaching) + "\n\n" + reply
	}
	sess.Transcript = append(sess.Transcript, d5Message{Role: "assistant", Content: reply, Mode: modeCoaching})
	d5Respond(p, reply)
}

// d5Respond writes the reply in the OpenRouter completion shape the UI already reads and
// completes the turn so replays and followers get the same reply.
func d5Respond(p chatRunParams, content string) {
	body, err := json.Marshal(map[string]any{
		"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": content}}},
	})
	if err != nil {
		http.Error(p.w, "internal error", http.StatusInternalServerError)
		if p.managesTurn() {
			p.turns.failTurn(p.sessionID, p.turnID, content, content, nil, http.StatusInternalServerError)
		}
		return
	}
	p.state.LastAssistantSummary = truncateSummary(content, 500)
	writeChatResponse(p.w, body, http.StatusOK)
	if p.managesTurn() {
		p.turns.completeTurn(p.sessionID, p.turnID, content, content, body, http.StatusOK)
	}
}
