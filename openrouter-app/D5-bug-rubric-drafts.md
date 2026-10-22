# Draft: Bug Hunting level descriptions, weeks 2–9

_Draft for instructor review, 2026-10-01. Week 1 has no Bug Hunting mode._

Once approved, each week's section becomes a **Bug Hunting** section in `env/rubrics/weekN_rubric.md`. These are level descriptions for the Evaluator only. The rules that turn levels into a bucket live in Go (`grading-rules.md`).

Bug Hunting assesses **how the candidate would find a defect**, not whether they can fix it. A candidate is never marked down for not naming the fix.

---

## Shared descriptions (all weeks)

Bug Hunting has **one graded dimension, `strategy`**, and its level is the Bug bucket. Each level description covers four aspects of a debugging strategy:
- the overall approach;
- the hypotheses formed;
- how the candidate narrows down the cause;
- the link to the week's concept.

These aspects are not graded separately.

| Level | `strategy` description |
| --- | --- |
| **Not Ready Yet** | No real plan: "run it and see", "rewrite it", or "ask AI to fix it" with no checking. Guesses at random, or names a cause unrelated to the symptom. Can't name a specific check, value or line to inspect, and has no next step if the first idea fails. Doesn't connect the bug to the week's concept, or misstates how the concept works. Answers stay vague after a follow-up. |
| **Competent** | Compares what the code should do with what it does, and reproduces the problem with a concrete input. Names at least one plausible cause that fits the symptom. Places a print or trace at a sensible point, or picks a concrete test value, and has a next step if that doesn't explain the bug. Connects the suspected cause to the right concept in general terms. The approach may be fairly linear. |
| **Exceptional** | A deliberate plan: reproduce, predict the expected values, then check, explaining why each step comes first. Names several candidate causes, ranked, with what would confirm or rule out each. Chooses test values deliberately, including boundary or edge cases, narrows step by step, and revises the hypothesis when the evidence contradicts it. Explains precisely how the concept's behaviour produces the symptom. Any AI use is critical: asking for explanations or test cases, then verifying them. |

**Week 7–9 note:** if the candidate mentions `while True`, `break` or `continue`, don't penalise it. The interviewer asks why they chose that approach.

---

## Week 2 — Variables & Expressions

**Suitable defects** (ideas for the opening draft; no `input()` or `if`):
- missing parentheses changing evaluation order, e.g. an average computed as `a + b / 2`;
- `//` used where `/` was intended;
- the wrong variable used in an expression;
- a variable overwritten before it is used;
- `"Total: " + total`, where `total` is a number.

**What strategy looks like for this week's concept, at each level**
- **Not Ready Yet:** treats the wrong result as "Python being wrong" and doesn't consider evaluation order or types.
- **Competent:** suspects the expression or a variable's value, and would print the intermediate values.
- **Exceptional:** reasons about operator precedence or int-versus-float results, and predicts the exact wrong value the bug produces before checking.

**Example answers**
- **Not Ready Yet:** "I'd just retype the formula."
- **Competent:** "The average is too big, so I'd print `a`, `b`, and the result to see which part is off."
- **Exceptional:** "With 80 and 90 it prints 125, which is 80 + 45, so the division is happening before the addition. I'd print `a + b` on its own to confirm, then check precedence."

## Week 3 — Input & Type Casting

**Suitable defects:**
- `input()` used without a cast, so `"5" + "3"` gives `"53"`;
- `int()` applied to a decimal entry such as `"4.5"`;
- a cast whose result is never assigned (`int(age)` on its own line);
- `int()` used where `float()` was needed, truncating values.

**What strategy looks like for this week's concept, at each level**
- **Not Ready Yet:** doesn't consider that input arrives as text.
- **Competent:** suspects that the value is still a string, or the wrong type, and would check its type or print it.
- **Exceptional:** explains that `input()` always returns a string. Pinpoints where conversion is missing or lost, and chooses inputs (whole number, decimal) that separate the possible causes.

**Example answers**
- **Not Ready Yet:** "Maybe the computer added wrong."
- **Competent:** "It printed 53 instead of 8, so I'd print `type(x)`. I think the numbers are still strings."
- **Exceptional:** "53 is the two entries joined, so they were never converted. I'd check whether the `int()` result is actually stored back in the variable, then test with 4.5 to see if `int` versus `float` matters too."

## Week 4 — String Methods

**Suitable defects:**
- `name.strip()` called without assigning the result;
- `name.upper` with no parentheses;
- `split()` on the wrong separator;
- an f-string missing its `f`;
- `replace()` with its arguments reversed.

**What strategy looks like for this week's concept, at each level**
- **Not Ready Yet:** expects the original string to change, with no reason given.
- **Competent:** suspects the method call or the format string, and would print the string before and after the call.
- **Exceptional:** explains that string methods return a new string and leave the original unchanged. Uses visible markers, such as printing `repr()` or wrapping the value in brackets, to reveal spaces or separators.

**Example answers**
- **Not Ready Yet:** "I'd ask AI to fix the formatting."
- **Competent:** "The output still has spaces, so I'd print `name` right after the `strip` line."
- **Exceptional:** "If `name` still has spaces after `strip()`, the result probably isn't being saved. Strings don't change in place. I'd print `name` and `name.strip()` side by side to confirm."

## Week 5 — Conditionals

**Suitable defects:**
- `elif` branches in the wrong order (`>= 60` checked before `>= 90`);
- `>` where `>=` was intended at a boundary;
- `and` and `or` swapped;
- comparing an uncast `input()` string with a number.

**What strategy looks like for this week's concept, at each level**
- **Not Ready Yet:** tests one random value, with no idea of which branch ran.
- **Competent:** would find out which branch runs, using prints in each branch, and tests a value that should hit each one.
- **Exceptional:** tests boundary values deliberately (89, 90, 91). Reasons that only the first true branch runs, so order matters, and explains why the `and`/`or` logic accepts or rejects the value.

**Example answers**
- **Not Ready Yet:** "I'd change the numbers until it works."
- **Competent:** "A 95 gets a D, so I'd add a print in each branch to see which one runs."
- **Exceptional:** "95 also satisfies `>= 60`, and that check comes first. Only the first true branch runs. I'd test 59, 60, 89 and 90 to confirm every boundary lands in the right branch."

## Week 6 — for Loops

**Suitable defects:**
- off-by-one `range()` bounds (`range(1, n)`);
- the accumulator reset inside the loop;
- the total updated after the loop instead of inside it, because of indentation;
- the loop variable used where the running total was intended.

**What strategy looks like for this week's concept, at each level**
- **Not Ready Yet:** doesn't think about how many times the loop runs, or what changes on each pass.
- **Competent:** would print the loop variable and the accumulator on each pass, or count the passes.
- **Exceptional:** predicts the expected values for each pass with a small input (say, 3 items). Spots exactly where the trace diverges, and links it to `range()` bounds, where the accumulator is set up, or what the indentation includes in the loop.

**Example answers**
- **Not Ready Yet:** "Loops are confusing, so I'd rewrite it."
- **Competent:** "The total is only the last price, so I'd print `total` inside the loop each time."
- **Exceptional:** "With 3 items I expect totals of 2, 5, 9. If each pass shows only that item's price, `total` is being reset inside the loop. Then I'd check that `range` runs exactly 3 times."

## Week 7 — while Loops & Menus

**Suitable defects** (condition-driven `while` only; never `while True`, `break` or `continue`):
- the menu choice never re-read inside the loop, so the loop never ends;
- a quit check of `'Q'` when the user types `'q'`;
- `while choice != 'q' or choice != 'Q'`, which is always true;
- a counter never incremented.

**What strategy looks like for this week's concept, at each level**
- **Not Ready Yet:** says only that "it loops forever", with no idea where to look.
- **Competent:** checks whether the variable in the `while` condition is updated inside the loop, and prints it each pass.
- **Exceptional:** traces the condition's value each pass. Shows how a compound condition can never become false, or how a case mismatch keeps it true, and tests the quit path deliberately.

**Example answers**
- **Not Ready Yet:** "I'd stop the program and start over."
- **Competent:** "It never quits, so I'd print `choice` at the top of each loop to see whether it changes."
- **Exceptional:** "The loop runs while `choice != 'q' or choice != 'Q'`. For 'q', the second half is still true, so the condition never becomes false. I'd evaluate both halves for 'q' to confirm."

## Week 8 — Lists

**Suitable defects:**
- an index of `len(scores)`, past the end;
- `sorted_scores = scores.sort()`, which stores `None`;
- `append` placed outside the loop;
- an average divided by the wrong count;
- `range(1, len(scores))` skipping the first item.

**What strategy looks like for this week's concept, at each level**
- **Not Ready Yet:** doesn't consider indexes or what a list method returns.
- **Competent:** prints the list and its length at key points, and checks the index range used.
- **Exceptional:** reasons about zero-based indexing and that the last index is `len - 1`. Knows some methods change the list in place and return `None`. Tests with a list of 1 or 2 items to expose edge cases.

**Example answers**
- **Not Ready Yet:** "I'd ask AI why the list is broken." (with no follow-up about checking)
- **Competent:** "It crashes on the last item, so I'd print `len(scores)` and the index it's using."
- **Exceptional:** "`IndexError` on the last pass means the index reaches `len(scores)`. Indexes stop at `len - 1`. I'd print `i` each pass with a 2-item list to confirm."

## Week 9 — Lists and Files

**Suitable defects:**
- lines not stripped, so `'\n'` breaks comparisons or `int()`;
- a header line not skipped, causing a `ValueError`;
- splitting on the wrong delimiter;
- reading the file twice, so the second pass is empty;
- appending values as strings, then summing or sorting them as text.

**What strategy looks like for this week's concept, at each level**
- **Not Ready Yet:** blames "the file", with no plan to inspect what was read.
- **Competent:** prints the first few raw lines and the values after `split()`, and checks their types.
- **Exceptional:** knows file lines are strings ending in newlines. Checks the header, the delimiter and the conversion step by step on a tiny test file, and recognises an exhausted file as the cause of an empty second loop.

**Example answers**
- **Not Ready Yet:** "The file must be corrupted."
- **Competent:** "It fails on the first line, so I'd print each raw line before converting it."
- **Exceptional:** "If the first line is 'name,score', `int()` will fail on 'score'. I'd print `repr(line)` for the first two lines to see the header and the `'\n'`, then test with a 3-line file."

---

## Review questions

1. Can AI use appear in Bug answers? Code mode has its own `ai_use` dimension. Here, asking AI to fix the code without checking counts as Not Ready Yet. Is that right?
2. Are the suitable-defect lists too prescriptive? They are ideas for the Evaluator's opening draft, not fixed questions. The model chooses and adapts one to the company scenario.
3. Should Exceptional require deliberately chosen boundary or edge-case tests, as drafted, or is a well-ordered trace enough?
