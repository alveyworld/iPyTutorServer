# Week 6 lessons: for Loops

Lesson plans for iPyTutor. Each `##` heading is one lesson, taught in order, and its bullets
are the points the tutor must cover. The tutor reads this plan when it starts, so edit freely.
Keep every lesson inside this week's scope: the scope test (`go test`) flags later-week ideas.

## Repeating with for and range()
- A `for` loop runs its indented block once for each value in a sequence, and the loop variable takes each value in turn.
- `range(5)` gives 0, 1, 2, 3, 4: it starts at 0 and stops before the end value.
- `range(start, stop)` and `range(start, stop, step)` set where counting starts, where it stops and how it steps (`range(10, 0, -2)` counts down).
- Off-by-one errors are common: `range(1, n)` runs n - 1 times, so use `range(1, n + 1)` to include n.

## Looping over the characters of a string
- `for ch in word:` visits each character of a string in order.
- This lets you count or examine characters: vowels, digits, spaces.
- Combine it with Week 4 string methods, for example `if ch.isdigit():` inside the loop.
- The loop variable is a new value on each pass; changing it doesn't change the string.

## Accumulators: running totals and counters
- An accumulator collects a result across passes: set it up before the loop and update it inside.
- Running total: `total = 0` before the loop, `total = total + price` inside it.
- Counter: `count = 0` before the loop, `count = count + 1` inside an `if` that checks what you're counting.
- Classic bugs: resetting the accumulator inside the loop, or updating it after the loop because of indentation.

## Decisions inside a loop
- An `if` inside the loop body runs on every pass, so each value can be treated differently.
- Use it to count matches, find the largest or smallest value, or ignore values that don't qualify.
- To find the largest value, start from the first value (or a safe starting value) and replace it whenever you see a bigger one.
- Indentation shows what belongs to the loop and what belongs to the `if`, and misplaced indentation changes the result.

## Tracing and decomposing loops
- Decompose repetition by asking: what repeats, what changes each pass, and what is accumulated?
- Trace a loop with a small example: write down the loop variable and the accumulator after each pass.
- Predict the final result before running the code; where the trace and the output differ is where the bug is.
- In the interview, explain how many times your loop runs and why.
