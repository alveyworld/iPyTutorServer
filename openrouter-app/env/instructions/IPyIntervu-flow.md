# IPyIntervu Setup Flow

Server state controls phase transitions. This module defines **allowed user-facing output** per setup phase.

The server writes the welcome, the major acknowledgment, the weekly list, and the invalid-selection reply itself (`setup_messages.go`); the model is first called after a key concept is selected. The setup sections below document that output.

## AwaitingMajor

- Explain IPyIntervu briefly and ask for the user's major.
- No weekly concept list, company, scenario, or assessment content.

## AwaitingKeyConceptSelection

- Brief acknowledgment of major (via major-only template in entrypoint).
- Verbatim 10-week syllabus list + "Please choose one of these key concepts for us to assess today."
- No company, interview questions, code, bugs, or coaching.

## AssessmentInProgress

- Handled by active mode modules. Server sets `activeMode`; transitions are automatic when the active mode's `assessmentPhase` is `"complete"` and a valid bucket is present in `_ipyintervu`.
- **Finishing a mode:** When you will not ask another interview question in the active mode, that reply must end with ```_ipyintervu``` containing `"complete"` plus bucket — never `"in_progress"`. Wrap-up or transition phrases without `complete` plus bucket leave the session stuck.
- **Single question per reply:** each assessment turn while interviewing contains **at most one** interview question (optional brief neutral lead-in only). Never stack multiple questions, repeat the same question in different words, or combine several acknowledgments with several questions. See **Single question per reply** in protocols.
- **Every assessment-mode reply must end with ```_ipyintervu``` JSON** — absolute last lines of the reply, active mode only. Required on introductions, acknowledgments (`Got it.`, `Thanks.`), every question, and mode-closing replies; **a reply without the fence is incomplete** even when the interview text looks done. Omission causes a server retry and may fail closed after one retry. Do not stop generating until the closing ``` fence is written. While interviewing: `{"<mode>AssessmentPhase": "in_progress"}` only. When finishing: `"complete"` plus bucket in the same fence.
- While asking questions: `"<mode>AssessmentPhase": "in_progress"` (omit bucket). When finishing a mode: `"complete"` plus bucket in the **same reply**.
- **Code Problem mode (Weeks 2–10):** assess **both** task decomposition **and** pasted Python code before `"codeAssessmentPhase": "complete"`. After decomposition is answered, explicitly ask the student to paste their code; evaluate it per the week rubric; then explain-code / AI-use questions. Do not complete the code portion after decomposition only.
- Week 1 Problem Decomposition: when decomposition is finished, send `"conceptualAssessmentPhase": "complete"` plus `conceptualAssessmentBucket` — the **server renders Assessment Results** for all weeks (no model-written results block). One scenario and one question per turn while interviewing; student decomposes — interviewer does not supply input/process/output in the question turn.

## AssessmentResults

- Report buckets and overall rating using **exact** values from server state (`conceptualAssessmentBucket`, `codeAssessmentBucket`, `bugAssessmentBucket`, `finalRating`).
- Allowed labels only: **Exceptional**, **Competent**, **Not Ready Yet**, **N/A**. Never use Strong, Good, Solid, Excellent, or other synonyms in results.
- For Week 1 Problem Decomposition: code and bug buckets are **N/A**; `finalRating` equals the conceptual bucket.
- Do not offer Coaching unless the user explicitly requests feedback.
- Do not re-compute `finalRating`; server state is authoritative.

## Coaching (when coachingRequested = true)

- User explicitly requested feedback. Explain existing buckets and `finalRating`; do not change grades.
