# Week 5 lessons: Conditionals (if/elif/else)

Lesson plans for iPyTutor. Each `##` heading is one lesson, taught in order, and its bullets
are the points the tutor must cover. The tutor reads this plan when it starts, so edit freely.
Keep every lesson inside this week's scope: the scope test (`go test`) flags later-week ideas.

## Making a decision with if and else
- An `if` statement runs its indented block only when its condition is True; `else` runs when it is False.
- A colon ends the `if` and `else` lines, and the block underneath is indented (4 spaces).
- Exactly one of the two branches runs.
- Decompose by finding the decision points: where does the program need to choose between outcomes?

## Comparison operators and booleans
- Comparisons produce True or False: `==`, `!=`, `<`, `>`, `<=` and `>=`.
- `=` assigns and `==` compares, so `if age = 18:` is a syntax error.
- Boundaries matter: "18 or older" is `age >= 18`, not `age > 18`.
- Compare numbers with numbers: cast input first, because comparing the strings `"9"` and `"10"` compares text, not values.

## Many outcomes with elif
- `elif` adds more conditions to one decision; Python checks them top to bottom and runs only the first branch whose condition is True.
- Order matters: check the highest threshold or most specific case first (`>= 90` before `>= 80`).
- Separate `if` statements are independent decisions, so several can run; use `elif` when the outcomes are mutually exclusive.
- End with `else` to catch every remaining case, including unexpected values.

## Combining conditions with and, or and not
- `and` is True only when both sides are True, `or` when at least one is, and `not` flips True and False.
- Ranges use `and`: `score >= 0 and score <= 100`.
- Each side must be a full comparison: `choice == "a" or choice == "b"`, not `choice == "a" or "b"`.
- Check tricky combinations by evaluating each side separately for a sample value.

## Edge cases and invalid input
- Test the boundary values of every condition (for `>= 90`, try 89, 90 and 91).
- Use Week 4 tools to validate input before casting it: `if text.isdigit():`.
- Give a clear message for values outside the expected range instead of a wrong answer.
- In the interview, naming edge cases and how your branches handle them is a hallmark of an Exceptional answer.
