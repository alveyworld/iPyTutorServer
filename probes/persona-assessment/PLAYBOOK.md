# Persona assessment probe: playbook

This probe measures how accurately the D5 engine assesses students. A Claude Code session plays a simulated student (a **persona**) through a complete assessment on a local D5 server: introduction, Conceptual, Code, Bug, results, then coach mode. It then judges the run. The real engine (interviewer, Evaluator, coach) runs on OpenRouter; only the student is simulated, by Claude.

Runs accumulate in `results/runs/`. `probe report` combines every judged run, so repeating sessions builds a larger sample and narrows the confidence intervals.

## Personas

Each persona answers at one level in every part and every week:

| Persona | File | Expected |
|---|---|---|
| `not_ready` | `personas/not_ready.md` | Not Ready Yet |
| `competent` | `personas/competent.md` | Competent |
| `exceptional` | `personas/exceptional.md` | Exceptional |
| `engineer` | `personas/engineer.md` | Exceptional (experienced engineer, best-practice Python) |

The persona file says how the persona answers. The week's rubric (`openrouter-app/env/rubrics/weekN_rubric.md`) says what that level looks like for this week's concept. Read both before the first answer.

## Setup (once per session)

```
cd probes/persona-assessment
go build -o probe .
./probe serve          # leave running (run_in_background); prints the server URL
```

## Playing one run

1. `./probe start -persona <persona> -week <1-9> -player <model>` (the model playing, e.g. `haiku` or `sonnet`) prints the run ID, the files to read, and the interviewer's opening.
2. Read the persona file and the week's rubric. Read the key-concepts file too if the persona is a student, so you know what the course has covered.
3. Answer every interviewer message in persona, one message at a time:
   ```
   ./probe say -run <ID> <<'EOF'
   <the student's message, nothing else>
   EOF
   ```
   Each call prints the reply and a `NEXT:` line saying what to do.
4. When the results appear, ask for coaching in persona. The message must contain the word **coaching** (for example "Could I get some coaching on how I did?").
5. Ask natural follow-up questions about the feedback until the probe says you've had enough coaching (3 coach replies).
6. Judge the run (below).

### Rules for playing a persona

- Write only what the student types: no narration, labels or stage directions.
- Answer the question that was asked, at the persona's level, every time. Don't drift up or down as the interview goes on, and don't react to how well you seem to be doing.
- When asked for code, put it in one ```python block.
- Never mention the rubric, levels, personas, simulation or that you are an AI.
- Every persona, at every level, always says it used an AI assistant to generate the code. Never say you wrote the code without AI, and say it whenever the interviewer asks about AI use. How well you checked what the AI produced still follows your persona.
- If the interviewer asks something unclear, a short clarifying question is fine.
- If `say` fails with an error, wait a few seconds and send the same message again.

## Judging

Judge only after the coaching. Do it as an impartial assessor: grade what the student actually wrote, not what the persona was meant to write.

1. `./probe show -run <ID>` prints the transcript with each answer's number (`answer #N`) and the coach turns (`to coach`).
2. For every answer, decide its level against the week's rubric (`not_ready`, `competent` or `exceptional`), and whether it really was at the persona's intended level (`onPersona`).
3. For every coach reply, decide whether it was appropriate, and list any issues. An appropriate coach reply:
   - is consistent with the results and doesn't change or second-guess them;
   - refers to this candidate's actual answers, quoting or paraphrasing them;
   - coaches interview skills (clarity, specificity, structure, hedging), not code, and doesn't expose internals;
   - names real strengths and only weaknesses that really appear, and suits this candidate (an engineer shouldn't be told to learn basics; a struggling student shouldn't be praised for depth they didn't show);
   - is concrete and answers the student's follow-up question.
4. Give your own overall level for the performance as given.
5. Save it:
   ```
   ./probe judge -run <ID> <<'EOF'
   {
     "answers": [
       {"answer": 1, "level": "competent", "onPersona": true, "note": "correct main idea, no edge cases"}
     ],
     "coach": [
       {"turn": 17, "appropriate": true, "issues": []},
       {"turn": 18, "appropriate": false, "issues": ["suggested a code change"]}
     ],
     "coachOverall": {"accurate": true, "specific": true, "inScope": true, "helpful": true, "note": ""},
     "overall": {"level": "competent", "note": ""}
   }
   EOF
   ```
   Every answer number and every coach turn must be judged. Keep issue phrases short and reusable (e.g. "suggested a code change", "generic, not tied to answers", "contradicted the rating") so the report can count them.

`judge` then shows the engine's labels for each answer next to your level and the persona's. The engine's labels are hidden until then so they can't anchor the judgement.

## Report

```
./probe report                 # writes results/report.md
./probe report -player sonnet  # only runs played by one model
./probe report -since 2026-10-03T19:30  # only runs started after a change
```

The report covers overall and part ratings against each persona, per-answer label accuracy by dimension (against the persona and against the judge), persona fidelity, coach-mode quality with the issues counted, a list of every engine/judge disagreement, and accuracy by week.

## Running many at once

Several runs can share one server. From the chat session, one subagent per run works well: give each the persona, the week, this playbook's path, and the instruction to finish with `probe judge`. A full sweep is 4 personas × 9 weeks = 36 runs.
