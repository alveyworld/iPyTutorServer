# D5 rules carry-over checklist

_Extracted 2026-10-01 from `env/instructions/*.md`, the directive strings in the Go code, and the week 1 and week 7 rubrics. Each rule is to be marked keep, change or drop. Companion to `D5-interviewer-evaluator-design.md`._

## Decisions (2026-10-01)

| Topic | Decision |
| --- | --- |
| Tone (A3) | Warm, professional interviewer. The neutral "Got it."-only rule is dropped. Still no correctness verdicts, teaching or hints. |
| Education-major exception (A1, I2) | Dropped. Every major gets company interviewers. |
| Redirect after a vague answer (B7) | Allowed once per mode. |
| Vague close forces Not Ready Yet in every mode (B7, D17) | No. Only Conceptual; Code and Bug are graded from labels. |
| Before code (D6) | Ask for problem decomposition, then for the code. The task is framed as the data available and what is wanted. The CD:30 rationale/alternatives step is dropped. |
| Say AI use is allowed (D7) | Yes, in the request for code. |
| `while True`/`break`/`continue` ban (C6) | Applies in weeks 7, 8 and 9. |
| Evidence behind buckets (G3) | Explained in coaching, not in the results message. |
| Coach role (I2, H3–H5) | A recruiter on the company's HR team who coaches interview skills only: vagueness, wording, strengths, weaknesses. No code improvement or Python practice (a separate tool covers learning the code). This replaces the week-guide practice suggestions in H3–H4. |

All other rows are carried into the design doc as suggested in their Home column.

**Source abbreviations** (paths relative to `openrouter-app/`)

| Abbreviation | Source |
| --- | --- |
| EP | `env/instructions/IPyIntervu-entrypoint.md` |
| FL | `IPyIntervu-flow.md` |
| PR | `IPyIntervu-protocols.md` |
| WS | `IPyIntervu-week-scope.md` |
| SH | `IPyIntervu-modes-shared.md` |
| CO | `IPyIntervu-modes-conceptual.md` |
| CD | `IPyIntervu-modes-code.md` |
| BG | `IPyIntervu-modes-bug.md` |
| CH | `IPyIntervu-modes-coaching.md` |
| IP | `interview_progress.go` |
| QG | `question_guard.go` |
| AV | `assessment_violations.go` |
| WSG | `week_scope.go` |
| PRG | `prompt_router.go` |
| SM | `state_machine.go` |

**Home abbreviations**

| Abbreviation | Meaning |
| --- | --- |
| Tmpl | Interviewer system template (design §4) |
| Move | Per-move instruction (§6) |
| Draft | Evaluator opening-draft prompt |
| Eval | Evaluator brief or `LEVELS` prompt |
| Go | Director rule or post-check |
| Scope | Week-scope lists in `week_scope.go` |
| Coach | Coaching template |

**Pre-existing defect.** Several files refer to sections of PR that don't exist: "Persona identity", "Question repetition limits", "Student expresses difficulty" and "Week 7 while-loop scope" (EP:136, SH:13/21, BG:20/22, CO:23/28). Their content lives only in the mode files and the Go strings cited below.

---

## A. Interviewer conduct and voice

| # | Rule | Source | Home | Notes |
|---|---|---|---|---|
| A1 | Speak as an employee of a company in the student's major field, never as an instructor, teacher, tutor, professor or TA. **Exception:** instructor roles are allowed when the major is education or teaching. | EP:136; SH:21; CO:28; CD:47; BG:9; CH:9; QG `speaksAsInstructor` / `isEducationMajor` | Tmpl + Go post-check | **Decision needed.** The D5 template has no exception. In `speaksAsInstructor`, only the role words are exempted for education majors; classroom framing (A2) is flagged for every major. |
| A2 | Never mention weeks, the course or class, CSE numbers, the syllabus, lectures, modules, homework, or what the candidate has learned. Frame everything around work at the company. | WSG `weekScopeTurnDirective`; AV `correctiveNoteRule`; CD:48; QG `classroomFramingPattern` | Tmpl + Go post-check | |
| A3 | Acknowledge answers only neutrally ("Got it.", "Thanks."). No praise, no saying whether the answer was right, no explaining why, no summarising what they got right. | EP:134; SH:16; CO:17,31,38; CD:38,51; BG:15,34,40; QG `evaluativePraisePattern` | Tmpl + Go post-check | The D5 template's "Warm… react in at most two sentences" must become "a brief neutral acknowledgement". Add `looksLikeEvaluationOrStagedTurn` to the §9 post-checks. |
| A4 | During the assessment, give no explanations, teaching, walkthroughs, answer-revealing hints, improvement tips or coaching offers. | EP:132; SH:11-12; PRG:275; CO:31,38; CD:51; BG:20,34,40 | Tmpl | |
| A5 | Never answer your own question. That means no model answer, expected output, solution code, decomposition, bug or fix, and no "For example," followed by sample answers. | EP:16,133; PR:12; SH:15; CO:15; CD:34; BG:15; PRG:277; QG `answerExplanationPattern` | Tmpl + Go post-check | |
| A6 | Never write or simulate the candidate's reply, or continue as though they had already answered (stacked mini-interviews, a second "Got it." paragraph). | EP:16,21-30; PR:12; CO:15; CD:34; QG `findSimulatedStudentIndex`, `looksLikeSelfAnsweredQuestion`, staged-turn check | Tmpl + stop sequences + Go post-check | Add `looksLikeSelfAnsweredQuestion` and the staged-turn check to §9. |
| A7 | Use only values the candidate actually gave; never invent their answer details when reacting. | AV:170 | Tmpl | |
| A8 | Talk only to the candidate. Never greet, address or hand off to another interviewer. | AV:162; QG `addressesPersona` | Go post-check | Mostly moot with one persona per phase. Build `personaNameAlt` from the name pools. |
| A9 | Never show internal vocabulary: persona IDs, state fields, mode names, the rubric, grading notes, or "the student". | EP:138; CO:31; CD:51,57; QG `leaksReasoning` | Go post-check | Add `leaksReasoning` to §9. |
| A10 | Never leave a placeholder such as `[companyName]`. | AV:121,162; QG `hasUnfilledPlaceholder` | Go post-check | Already in §9. |
| A11 | On follow-up turns, don't welcome the candidate again, re-introduce yourself, restate the scenario or restart. | IP `followUpTurnDirective`; CO:36; SH:9; AV:170 | Tmpl | The server writes all introductions. |
| A12 | Never say you are wrapping up, moving on or transitioning; the server alone closes a mode. | CO:22,24; CD:31; EP:51 | Tmpl | |
| A13 | Never ask the candidate to choose or confirm the next mode. | EP:70; SM:707-709 | Go (inherent) | |

## B. Question structure

| # | Rule | Source | Home | Notes |
|---|---|---|---|---|
| B1 | Ask exactly one question per reply, with at most one brief lead-in. No stacked questions, no restating the same question in other words, no extra lead-ins. Then stop. | EP:15,131; FL:22; PR:11,41-45; SH:14; CO:14; CD:33; BG:14; PRG:278; QG `countInterviewQuestions`, `countNeutralAssessmentLeadIns` | Tmpl + Go post-check | |
| B2 | Every non-closing reply must actually ask the candidate something. | AV:24-26,93; QG `asksStudentSomething` | Go post-check | Add to §9. On a hit, Go appends the brief's `FALLBACK` question. |
| B3 | Within a mode, never re-ask a question the candidate already answered, in any wording, unless they asked for clarification. | SH:13; IP:209,247; AV:133; IP `repeatsAnsweredQuestion` | `AVOID` + Tmpl + Go post-check | Pass the list of questions already asked to the Evaluator. |
| B4 | The same ground may be asked at most twice per mode; after a re-ask, the mode closes on the evidence so far. | SH:13; IP `similarQuestionLimitReached` | Go (CLOSE_MODE) | Already in D5. |
| B5 | Each follow-up opens new ground (a different tool, assumption, narrowing step or failure case; reasoning and edge cases in Conceptual). | BG:21; IP:221; CO:37 | Eval `NEXT` | |
| B6 | **Clarification:** restate the question more simply without giving the answer. It doesn't count as a repeat or a vague answer, and it blocks closing on that turn. | IP `studentAskedForClarification` | Go (CLARIFY move) | **Defect, confirmed.** Any "?" counts as clarification. Tighten the detector before it drives a move. |
| B7 | **Vague answers:** two vague answers close the mode. Don't rephrase to fish for a better answer. Vague or no strategy means Not Ready Yet. | SH:13; BG:22; IP:373-396,502-537 | Go (CLOSE_MODE) + Go grading | **Decision needed.** (1) Does D5's REDIRECT_VAGUE move count as forbidden fishing? (2) Should a vague close mean Not Ready Yet in every mode? D5 currently applies it only to Conceptual. |
| B8 | In Code mode, the vague and repetition caps apply only after code is pasted. | IP `overfitLimitsApply` | Go | Go must also ignore a brief's `RECOMMEND: close` before the paste. |

## C. Week and scope content

| # | Rule | Source | Home | Notes |
|---|---|---|---|---|
| C1 | Use only concepts from weeks 1 to N. Earlier weeks are allowed as support; later-week concepts never appear. | WS:5-9; EP:135; CO:10-11; CD:10-11,49; BG:17; WSG `scopeRule`, `weekScopeTurnDirective` | Tmpl `{allowed_concepts}` + Go `outOfScopeConcepts` | |
| C2 | Don't mention a forbidden concept even to rule it out ("without using an if statement"). | WSG:107,235; AV:191 | Tmpl | Consider passing a short forbidden list as well. |
| C3 | Dictionaries are never in scope, in any week: no dict syntax, the words "dict"/"dictionary", or list-vs-dict comparisons, in questions, tasks, snippets or coaching. | CO:12; CD:12; BG:18; CH:13; SH:51; WSG:151-152 | Scope + Tmpl + Go post-check | |
| C4 | Before week 8, tasks must not need multiple records in a collection or external data. Week 8 is lists only; week 9 is lists plus file I/O. | `week7_rubric.md`:13; CD:11-12 | Scope + Draft | |
| C5 | Detector specifics: "user input", "the user types" and "prompts the user" count as week 3; "loop"/"iterate" as week 6; "file(s)"/"csv" as week 9; `[` or `len(` in code as week 8. | WSG:132-153 | Scope | **Defects, confirmed.** "user input" fires on Week 1 input/process/output scenarios. "files" fires on ordinary business wording ("customer files"). Fix both. |
| C6 | **Week 7:** never prompt for or design tasks that elicit `while True`, `break` or `continue`. If the candidate uses them, accept them without penalty and ask why. In Bug mode this is about process. | `week7_rubric.md`:5-11; CO:23; CD:43; BG:27 | Tmpl/Draft (when N=7) + Eval | **Decision needed:** does the ban continue in weeks 8-9? |
| C7 | Content comes from the selected key concept's key-concepts file and competency guide. | CO:10; CD:10; BG:17; CH:12 | Draft + Eval | **Gap in D5.** The live first opening receives no key-concepts text. Add a one-line concept focus to its move. |

## D. Mode-specific flow

### Conceptual

| # | Rule | Source | Home | Notes |
|---|---|---|---|---|
| D1 | Ask 3-5 questions (cap 5), closing earlier only if answers clearly show Not Ready Yet or Exceptional. | CO:13; IP:262-263,322 | Go cap + Eval `RECOMMEND` | Enforce the minimum of 3 in Go. |
| D2 | The opening invents a company in the candidate's major field, then presents one scenario and one question. | CO:5,29-30 | Tmpl (first opening, `COMPANY:` line) | |
| D3 | Conceptual questions stay conceptual: no code-level details. | CO:16 | Move | |

### Code (weeks 2-9)

| # | Rule | Source | Home | Notes |
|---|---|---|---|---|
| D4 | One coherent Python task framed as company work, describing only the outcome. No numbered steps, no solution, solvable with weeks 1 to N. | CD:9-13,31,49,51 | Draft + scope validation | |
| D5 | The first code question asks for decomposition as one ask only. | CD:13,21,31,33; SM:707 | Draft + `countInterviewQuestions` on the draft | |
| D6 | **Sequence:** decomposition answer, then ask for pasted code, then explain-code and AI-use questions, then close. Never close without pasted code; never repeat decomposition instead of asking for code. | CD:15-28,32,42,50,56; EP:57-62; FL:25; SH:18; PRG:281; IP:164-170 | Go (moves; block close until code is pasted) | **Decision needed.** CD:30 asks for rationale and alternatives before code, but Go asks for code immediately. Recommendation: keep the Go behaviour. |
| D7 | Candidates may use external AI to write the code; the interviewer explicitly asks for the paste. | EP:60; CD:22,35 | Move (REQUEST_CODE) | **Decision needed:** should the interviewer say AI use is allowed? |
| D8 | Once code is pasted, never ask for it again. | IP:168,216,542-545 | Go + post-check | |
| D9 | Close Code when code is pasted and either 3 post-code questions have been asked, or 2 including one about AI use. | IP `codeClosingDue` | Go (CLOSE_MODE) | **Missing from D5; confirmed.** The CODE_FOLLOW_UP move should ask explain-code first, then AI use if not yet asked. |
| D10 | Don't judge correctness until code is pasted. No paste means correctness is Not Ready. | CD:36-39 | Eval + Go grading | Already in D5 §8. |

### Bug Hunting

| # | Rule | Source | Home | Notes |
|---|---|---|---|---|
| D11 | One short snippet with one week-appropriate defect and its intended behaviour, framed as an internal tool. Ask how they would *find* the bug. | BG:10-13,17,32-33 | Draft | |
| D12 | Never reveal, hint at or explain the bug, the fix or the correct line. | BG:12,15,34,40 | Tmpl + Move | |
| D13 | Never ask for corrected or rewritten code, or "what should this line be?". | BG:16,34,40; PRG:284; SM:709 | Move + new Go post-check | |
| D14 | Follow-ups stay on debugging process: tools, prints and logs, narrowing hypotheses, assumptions, tests, what to check first, what if the first idea fails. Never require running code. | BG:11,14,19,26,39 | Eval `NEXT` + Move | |
| D15 | If the candidate is stuck, give a neutral acknowledgement and a narrower process question. Never offer to explain or walk through the bug, and never suggest coaching. | BG:20,34,40 | Move | Probably meant for all modes. |
| D16 | Ask at most 4 questions, including the opening. | BG:21; IP:359-371 | Go | Already in D5. |
| D17 | A systematic strategy earns a higher bucket; vague means Not Ready Yet; prefer conservative. | BG:23 | Eval + Go grading | See B7. |

## E. Week 1 special case

| # | Rule | Source | Home | Notes |
|---|---|---|---|---|
| E1 | Week 1 is Conceptual only; Code and Bug are N/A, and the final rating equals the Conceptual bucket. | EP:53; FL:26,32; WS:13; SH:10,29; CH:10; `week1_rubric.md`; SM:651-664 | Go | Already present. |
| E2 | Real-world decomposition only (input, process, output, ordered steps), with no code or syntax. | CO:19 | Move + Tmpl variant | The D5 template is Python-centric; Week 1 needs a no-code variant. |
| E3 | One scenario for the whole interview; never present a second one. | CO:19; IP:20 | Move | |
| E4 | The interviewer never supplies input, process or output. | CO:19; FL:26 | Tmpl + Move | |
| E5 | Steer follow-ups to whichever of input, process and output hasn't been asked yet; close once all three are covered, or at 5 questions. | IP:179-180,265-323 | Go (`decompositionPartsRemaining`) | **Missing from D5's CLOSE_MODE list.** |
| E6 | The Week 1 closing must not imply another part follows. | CO:24 | Go (closing text) | `phaseClosingMessage` needs a Week 1 variant. |

## F. Session flow (Go)

| # | Rule | Source | Home | Notes |
|---|---|---|---|---|
| F1 | Modes run forward only: Conceptual, Code, Bug, Results. | EP:71; SH:8-9; PRG:274; `agent_state.go`; SM:626 | Go | Already present. |
| F2 | Setup asks for the major only, shows the week list verbatim, re-shows it after an invalid choice, says nothing about the assessment before a concept is chosen, and allows one concept per session. | EP:82-130; FL:7-16; SH:7 | Go (`setup_messages.go`) | Already present. Stale text to drop: "10-week" (FL:15), "Weeks 2–10" (EP:55). |
| F3 | A new mode starts directly with the new persona; no repeated wrap-up or mode-switch talk. | CD:31; BG:10 | Go (canned transition) | |

## G. Results and labels

| # | Rule | Source | Home | Notes |
|---|---|---|---|---|
| G1 | The only labels are Exceptional, Competent, Not Ready Yet and N/A. "Not Yet Ready" maps to Not Ready Yet. | EP:64,137; FL:31; SH:25-26; CO:20; `week1_rubric.md`; `agent_state.go` | Go results text + `LEVELS` parser | |
| G2 | The results message uses a fixed format with the stored values. | SH:31-43; AV:241-270 | Go | Already present. |
| G3 | Briefly explain the evidence behind each bucket. | SH:28 | Coach or drop | **Decision needed.** The server-built results message contains no evidence. |
| G4 | Don't offer coaching unless the candidate asks. | FL:33; SH:11 | Go | The results template offers coaching itself. Treat this rule as applying only during the assessment. |

## H. Coaching (after results only)

| # | Rule | Source | Home | Notes |
|---|---|---|---|---|
| H1 | Coaching starts only on an explicit request. | CH:3,7; SH:11,47; FL:36-38 | Go | The current code allows mid-assessment coaching and resume (SM:105-115). Remove it, per the decision. |
| H2 | Never change or re-grade buckets or the final rating. | CH:5,15; SH:50; FL:34,38 | Coach | |
| H3 | Per bucket: 1-3 strengths, 1-3 growth areas tied to the week guide, 1-2 action items, and a check-in question. | CH:11,25-26 | Coach | Give the coach the competency guide and key concepts. |
| H4 | Practice ideas stay within weeks 1 to N; no dictionaries. Week 7: condition-driven `while` menus only. | CH:12-14 | Coach + `{allowed_concepts}` | |
| H5 | Supportive and growth-focused, framed as on-the-job growth. No complete solutions, no off-topic tutoring. | SH:49; CH:15,19 | Coach | |
| H6 | Week 1: coach through the conceptual lens only. | CH:10 | Coach | |

## I. Personas

| # | Rule | Source | Home | Notes |
|---|---|---|---|---|
| I1 | Each persona introduces themselves by name and job title at the company, plus how the company relates to the candidate's major. | CO:29; CD:48; BG:32; CH:20 | Go (canned intro) | Include `{company_domain}` in the canned introduction. |
| I2 | Conceptual: a domain or technical employee. Code: an engineer. Bug: QA or tooling. Coach: a mentor or team lead at the company (an education organisation for education majors). | CO:28; CD:47,55; BG:31,38; CH:9 | Go (canned roles) | **Decision needed.** "Career coach" breaks the "mentor at the company" framing. |
| I3 | A style per phase. Conceptual: reflective. Code: exacting. Bug: methodical. Coach: rephrases until it clicks. | CO:28,35-37; CD:47,55-56; BG:31,38; CH:19,25 | Drop, or one adjective in the role | Optional. |

## Dropped automatically

- **Sync fence and JSON:** the `_ipyintervu` tail, phase and bucket fields, last-lines rules, `businessDomain` in the fence.
- **Corrective retries:** all `[System: …]` handoffs and "don't mention retries" rules.
- **System-message handling:** rules about `[System]` lines.
- **Server-state plumbing:** "state JSON is authoritative", `kbFilesLoaded`, progress snapshots, `personaNames`, `snapshotForPrompt` policies, `modeContinuationUserMessage` wording.
- **Model-assigned buckets:** replaced by Evaluator `LEVELS` plus Go grading.
- **Two personas per phase:** alternation, and "conservative bucket when personas disagree".
- **Mid-assessment coaching and its resume path:** moot under the coaching-after-results decision.
- **Model-written setup and results text:** already done in Go.
- **Unproducible-content fallback text:** replaced by Go fallbacks.
