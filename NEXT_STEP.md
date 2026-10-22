# Next step: make Conceptual labelling separate Exceptional from Competent

Status: approved, not started. Branch `d5-redesign`, after commit `6b62bfb`.

## The problem

In the 32-run simulated set (8 each of exceptional, competent, not ready yet and engineer
students, weeks 2–9), no student was rated Exceptional overall.

| Profile | Exceptional | Competent | Not Ready Yet |
|---|---|---|---|
| exceptional | 0 | 7 | 1 |
| engineer | 0 | 7 | 1 |
| competent | 0 | 6 | 2 |
| not ready yet | 0 | 0 | 8 |

The Code and Bug parts are no longer the blocker: exceptional students got an Exceptional
Code part in 6 of 8 runs and an Exceptional Bug part in 7 of 8. The **Conceptual part** is
the blocker. Only 1 of 8 exceptional students got an Exceptional Conceptual part. Their
Conceptual answers were labelled 9 Exceptional, 23 Competent and 8 Not Ready, almost the
same mix as the competent students' answers (10, 27, 3). The Conceptual labels do not tell
the two groups apart.

## Why (from re-labelling 15 exceptional answers, 3 times each, with a reason)

1. **The labels are random.** The same answer came back `[not_ready, exceptional,
   exceptional]` on different calls. Each answer is labelled once, by the Evaluator brief,
   so one unlucky draw decides it.
2. **Using things beyond the week is punished.** A Week 2 answer using `str()` and `round()`
   was marked Not Ready as "not covered in Week 2". Three Week 7 answers using
   `while True`/`break` were marked Not Ready because the rubric says "never prompt or
   require `while True`". The labeller reads that interviewer rule as a penalty for the
   candidate. Both go against the rule that students may use anything they can explain.
3. **The interviewer drifts off the week's topic, and off-topic answers get Not Ready
   instead of no label.** Examples: Week 8 questions about leading zeros in sample IDs and
   deduplication policy, and Week 7 questions about refactoring for file imports. One
   reason given was "not a topic covered in Week 8… no evidence to rate", yet the label was
   Not Ready, although the prompt says an off-topic answer gets no label.

## A. Labelling prompt fixes

Where: `d5Calibration` in `openrouter-app/d5_prompts.go`. Both the Evaluator and the
levels-only call use it.

- Add: using Python beyond what the week covers (for example `round()`, `while True`,
  `break`) is never a reason to lower a label; judge the explanation.
- Add: rubric notes about what the interviewer must never prompt or require (such as
  `while True`, `break` or `continue`) limit the interviewer only, never the candidate.
- Strengthen: `not_ready` is only for an answer that is wrong, confused or too vague. An
  answer to a question that was not about the week's topic gets no label.
- In `d5EvaluatorMessages`, the `LEVELS:` line says "Always include the targeted
  dimension". Change it so the targeted dimension is left out when the question was off
  the week's topic. The levels-only call at part close can then still give `none`.

## B. Label each answer more than once

Each answer's targeted dimension should get 3 labels instead of 1, and the stored level
should be the **most frequent** of them, ties going higher (matching "lean high").

- `d5CloseMode` in `openrouter-app/d5_handler.go`: when a part closes, make levels-only
  calls for **every** answer in the part, not only the unlabelled ones. Use 2 calls when
  the Evaluator already labelled the target, 3 when it did not. Skip correctness, which
  is pass/fail and already decided by one works/broken check. The calls run in parallel
  in the background. Only the final part's results wait for them, within the existing
  5 s `d5LabelJobsWait`.
- `addLabels` in `openrouter-app/d5_session.go`: replace "higher wins" with voting. Keep
  every vote per answer and dimension. Set the stored label to the most frequent level,
  ties going higher. With the current two sources a tie still gives the higher level, so
  nothing changes for those cases. A vague answer's label still stands, and the no-AI cap
  still blocks `ai_use` votes for an answer where the candidate said they did not use AI.
- Share the "most frequent, ties higher" logic between `addLabels` and `combinedLevels`
  in `openrouter-app/grading.go`.
- Update `unlabelledAnswers` callers and tests (`d5_brief_test.go`,
  `d5_postcheck_test.go`) and add unit tests for the voting.

## C. Keep Conceptual follow-ups on the week's topic

- `d5ChooseMove` in `openrouter-app/d5_director.go` (the Conceptual follow-up
  instruction): require that the question tests understanding of how the Python in the
  week's topic works (what a value, expression or construct does and why), applied to
  the scenario. Never ask about business rules, data policy, client decisions or
  workflow design, or anything a non-programmer could answer.
- `d5EvaluatorMessages`: make `NEXT:` say the next question must stay within the week's
  topic, so the Evaluator's suggestions don't steer the interviewer off it.

## Measure

1. **Label probe, before any full run:** `zz_probe_smoke_test.go` (`PROBE_FILE`,
   `PROBE_RUNS`) re-labels saved Conceptual answers. Run it on the same exceptional runs
   (r00, r05, r06) and on competent runs (e.g. r08, r14, r15) from a saved full-run
   transcript. Take the most frequent of 3 labels per answer, and expect exceptional
   answers mostly Exceptional and competent answers mostly Competent. The probe needs
   updating to use the new voting. The last transcript was in the session scratchpad;
   regenerate it with a full run if it's gone.
2. **Full run:** `TestD5FullRuns` with the default four profiles (32 runs). Compare:
   - overall rating per profile;
   - Conceptual labels per profile;
   - reply times (now p50 1.1 s, p95 2.9 s);
   - how long the results reply waits for label jobs.

   ```
   source ~/.openrouter-env
   cd openrouter-app
   D5_FULL_OUT=/tmp/fullrun.txt go test -tags smoke -run TestD5FullRuns -count=1 -timeout 90m -v . > /tmp/fullrun.log 2>&1
   ```

## Also noted in that run, not yet looked at

- r00 (exceptional, week 2): Not Ready Yet overall from a Not Ready Conceptual part. The
  probe gave mostly Exceptional for the same answers, so A and B should fix it.
- r11 (competent, week 5): Code part Not Ready from one `understanding=not_ready` label.
- r13 (competent, week 7) and r30 (engineer, week 8): Not Ready Bug parts. B's voting
  should also cover `strategy`; check these runs again after it.
- After A–C, update `D5-interviewer-evaluator-design.md` (Phase 2 drafting, marked bug
  snippets, voting) and `grading-rules.md` (most-frequent label per answer).
