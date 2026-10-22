# iPyInterVu grading rules

_Spec for the hard-coded grading in `grading.go`. Decided 2026-10-01; revised 2026-10-02 to give the benefit of the doubt: grading should lean high rather than low. Never sent to a model._

The model's only grading job is to label each answer with a rubric level, using the level descriptions in `env/rubrics/weekN_rubric.md`. Everything below is done by Go.

## Levels and labels

- Levels, from lowest to highest: `not_ready` < `competent` < `exceptional`.
- Buckets shown to students: **Not Ready Yet**, **Competent**, **Exceptional**, and **N/A** for a mode that doesn't run.
- The labelled-line parser also accepts "Not Yet Ready", "not ready" and "not ready yet" as `not_ready`, case-insensitively. Anything else is ignored and logged.

## Step 1 — Recording labels

Each student answer can produce labels: a level for each dimension that answer gave evidence for.

| Mode | Dimensions |
| --- | --- |
| Conceptual | `conceptual` |
| Code | `decomposition`, `correctness`, `understanding`, `ai_use` |
| Bug | `strategy` |

**Where labels come from**
- The Evaluator's brief after each answer (`LEVELS:` line).
- The levels-only call at mode close, which labels the final answer.

**One label per answer and dimension.** If an answer is labelled twice for the same dimension (a slow Evaluator brief and the levels-only call at close), only one counts:
1. a Go-set vague label always stands;
2. otherwise the **higher** of the two levels counts (benefit of the doubt).

**Each question targets one dimension**, and labels go on that dimension. The Evaluator adds another dimension only when the answer directly addresses it; the levels-only call labels only the targeted dimension.

**Correctness is pass/fail.** Code is assessed for the candidate's understanding, not its quality. The code only has to run and produce the correct output for the task: if it does, `correctness` is `exceptional`; if it errors or gives wrong output, `not_ready`. Style, names, comments, robustness and edge cases are not graded, and a `competent` correctness label from any labeller is recorded as `exceptional`. This overrides the rubric's Implementation Correctness row ("robust, handles edge cases"). Understanding is judged from the candidate's explanation of the code.

**No AI used.** Students are expected and encouraged to use AI. When the candidate answers the AI-use question by saying they did not use AI ("No", "I wrote it without AI"), Go labels that answer `ai_use = competent`, and no labeller overrides it. Not using AI therefore caps Code at Competent (Code Exceptional needs every dimension Exceptional) but never by itself causes Not Ready Yet. An answer that says AI was used (even partly: "…but I asked ChatGPT about the syntax") is graded normally, and using AI is never marked down.

**Labels Go sets itself**
- **Vague answers.** A vague answer is non-committal: empty, a bare non-answer ("idk", "pass", "no idea"), or a reply of 12 words or fewer built on a hedge ("not sure, maybe a print"). A short answer is **not** vague by length alone; "boolean" can be a complete answer. When `d5IsVagueAnswer` is true, the answer gets `not_ready` on the dimension its question targeted. This applies whatever the Evaluator says.
    - Conceptual and Bug: the mode's single dimension.
    - Code: the dimension of the move that asked the question:

      | Question asked by | Dimension |
      | --- | --- |
      | Opening, or a decomposition follow-up | `decomposition` |
      | REQUEST_CODE | `correctness` |
      | CODE_FOLLOW_UP about a line or choice | `understanding` |
      | CODE_FOLLOW_UP about AI use | `ai_use` |

      The vague answer counts as one Not Ready vote (Step 2), so a good answer after the one allowed redirect can still carry the mode.
- **No pasted code.** If Code mode closes without pasted code, its bucket is Not Ready Yet (Step 2).

**Labels that are not recorded:** answers handled by the CLARIFY move, where the student asked for clarification rather than answering.

## Step 2 — Mode buckets

**Combining several labels: the most frequent level wins, and a tie goes to the higher level.** When a dimension has more than one label in a mode, its level is the one given most often. If two levels are given equally often, the higher one wins.

**Conceptual**
- Bucket = the most frequent `conceptual` level (ties higher).
- No labels at all → Not Ready Yet.

**Bug**
- Bucket = the most frequent `strategy` level (ties higher).
- No labels at all → Not Ready Yet.

**Code**
1. No pasted code → Not Ready Yet.
2. Each dimension's level is its most frequent label (ties higher).
3. **Dimensions with no labels are ignored**; they neither raise nor lower the bucket.
4. `understanding` Not Ready → Not Ready Yet. Code assesses whether the candidate understands their code; one who cannot explain it is not ready, however well the code runs.
5. Otherwise, Not Ready Yet if 2 or more dimensions are `not_ready`. One other weak dimension alone does not decide the mode.
6. Otherwise, Exceptional if every dimension with labels is `exceptional`.
7. Otherwise, Competent.
8. No labels in any dimension → Not Ready Yet.

## Step 3 — Overall rating

This is unchanged from today's `computeFinalRating`:
1. Any mode bucket is Not Ready Yet → **Not Ready Yet**.
2. Otherwise, any mode bucket is Competent → **Competent**.
3. Otherwise (every bucket Exceptional) → **Exceptional**.

**Week 1:** only Conceptual runs. Code and Bug are N/A, and the overall rating equals the Conceptual bucket.

## Step 4 — Results message

This is unchanged from today's `buildServerAssessmentResultsMessage`: the selected concept, the three buckets (N/A where a mode doesn't run), the overall rating, and the offer to switch to coach mode. Evidence for each bucket is explained in coaching, not here.

## Examples (also the test table for `grading.go`)

| # | Mode | Labels in order | Bucket | Why |
| --- | --- | --- | --- | --- |
| 1 | Conceptual | competent, exceptional, exceptional | Exceptional | Most frequent |
| 2 | Conceptual | not_ready, not_ready, competent | Not Ready Yet | Most frequent |
| 3 | Conceptual | exceptional, exceptional, exceptional | Exceptional | |
| 4 | Conceptual | vague (→ not_ready), then redirect answer exceptional, exceptional | Exceptional | The vague answer is one vote |
| 5 | Conceptual | (none) | Not Ready Yet | No labels |
| 19 | Conceptual | competent, competent, not_ready, competent | Competent | One weak answer doesn't decide the mode |
| 20 | Conceptual | competent, exceptional | Exceptional | Tie goes higher |
| 21 | Conceptual | not_ready, competent | Competent | Tie goes higher |
| 6 | Bug | strategy: competent, exceptional, competent | Competent | Most frequent |
| 22 | Bug | strategy: not_ready, exceptional, not_ready, exceptional | Exceptional | Tie goes higher |
| 7 | Bug | strategy: exceptional ×4 | Exceptional | |
| 8 | Code | decomposition=exceptional; correctness=exceptional; understanding=exceptional; ai_use never discussed | Exceptional | Missing dimension ignored |
| 9 | Code | decomposition=exceptional; correctness=competent; understanding=exceptional; ai_use=exceptional | Competent | Not all Exceptional |
| 10 | Code | correctness=not_ready; all others exceptional | Competent | Only 1 Not Ready dimension |
| 11 | Code | decomposition=not_ready; understanding=not_ready; correctness=competent | Not Ready Yet | 2 Not Ready |
| 12 | Code | ai_use=not_ready; all others exceptional | Competent | 1 Not Ready, not correctness |
| 13 | Code | understanding: exceptional, then not_ready; others competent | Competent | `understanding` = exceptional (tie goes higher); others competent |
| 14 | Code | no code pasted; decomposition=exceptional | Not Ready Yet | No pasted code |
| 23 | Code | decomposition=competent; correctness=exceptional; understanding: competent, not_ready, not_ready; ai_use=exceptional | Not Ready Yet | Cannot explain the code (understanding Not Ready) |
| 24 | Code | ai_use=not_ready; all others exceptional | Competent | One Not Ready dimension, not understanding |

| # | Conceptual | Code | Bug | Week | Overall |
| --- | --- | --- | --- | --- | --- |
| 15 | Competent | Exceptional | Exceptional | 5 | Competent |
| 16 | Exceptional | Exceptional | Not Ready Yet | 5 | Not Ready Yet |
| 17 | Exceptional | Exceptional | Exceptional | 5 | Exceptional |
| 18 | Competent | N/A | N/A | 1 | Competent |
