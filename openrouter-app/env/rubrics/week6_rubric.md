# Week 6 Assessment Rubric

Week 6 covers `for` loops: repeating a body once per value, the loop variable, `range()`, iterating over the characters of a string, and accumulators (running totals and counts). Everything from weeks 1–5 (variables, `input()` and type casting, string methods, `if`/`elif`/`else`, comparisons, and boolean logic) may be used inside or around a loop. `while` loops and menus (week 7), lists (week 8), and files (week 9) are not expected.

## Not Yet Ready

### Conceptual Answer

- significant errors about how a for loop repeats
- cannot say what the loop variable holds on each pass
- confuses the loop body with the code after the loop (indentation)
- misunderstands range() (e.g. thinks range(5) includes 5 or starts at 1)
- fundamental misunderstanding of control flow

### Code Answer

- non-functional or major logic errors
- loop runs the wrong number of times (off-by-one range() bounds)
- accumulator not set before the loop, or reset inside it
- work that should repeat is placed after the loop, or work that should happen once is inside it
- completely misses the prompt

### AI Use Answer

- copying code without understanding
- cannot explain how they verified the AI's output
- unethical use or dependency that inhibits learning

### Example

**Rating:** Not Yet Ready

A for loop just runs the code again. range(5) goes from 1 to 5.

**Issues:**

- vague — does not say the body runs once per value
- range(5) produces 0, 1, 2, 3, 4 (it starts at 0 and stops before 5)
- does not mention the loop variable or what changes each pass

## Competent

### Conceptual Answer

- factually correct
- can describe how a for loop runs its body once per value
- can describe what range(stop), range(start, stop), and range(start, stop, step) produce
- can describe looping over the characters of a string
- can describe an accumulator (running total or count) built up inside a loop
- may lack depth on edge cases (e.g. a range that produces no values)

### Code Answer

- complete and functional
- correct loop header and range() bounds
- accumulator set before the loop and updated inside it
- uses earlier-week tools (input(), string methods, if/elif/else) where the task needs them
- may have minor inefficiencies or minimal comments

### AI Use Answer

- using AI for loops, debugging, or explanation
- can articulate how they tested and integrated the suggestion
- healthy, supplemental use of the tool

### Example

**Rating:** Competent

A for loop runs its body once for each value it is given. With for i in range(3): the body runs three times, and i is 0, then 1, then 2. To add up a fixed number of prices, I set total = 0 before the loop and add each price to total inside it.

**Strengths:**

- correct description of repetition and the loop variable
- correct range() values
- concrete accumulator example with setup before the loop

## Exceptional

### Conceptual Answer

- correct, clear, comprehensive
- concrete examples
- connects to problem decomposition (what repeats, what changes each pass, what is accumulated)
- may discuss edge cases (a count of zero, off-by-one bounds)
- may discuss combining a loop with week 5 conditionals (e.g. counting only the values that meet a condition)

### Code Answer

- complete, functional, clear
- well-chosen range() bounds or iteration target
- meaningful loop variable names
- sensible handling of edge cases such as a count of zero
- readable structure

### AI Use Answer

- AI to critique loop logic, check range() bounds, or generate tests
- strategic use to deepen understanding
- improve structure and edge-case handling

### Example

**Rating:** Exceptional

A for loop repeats its body once per value — each number from range() or each character of a string. range stops before its end value, so to number items 1 through count I use range(1, count + 1). For a total I set total = 0 before the loop, add inside it, and print after it, so the print happens once. Inside the loop I can use an if from week 5, for example to count only the scores that are 70 or higher. If count is 0 the loop body never runs and the total stays 0.

**Strengths:**

- clear model of repetition and the loop variable
- handles off-by-one with range(1, count + 1)
- separates setup, repeated work, and output
- combines the loop with a conditional and considers the zero case

## Code Answer Integrated Dimensions

### Decomposition

| Level | Guidance |
| --- | --- |
| Not Ready Yet | Cannot break problem into meaningful parts or gives no rationale |
| Competent | Breaks problem into reasonable parts with some justification |
| Exceptional | Clear, well-structured decomposition with strong justification and awareness of alternatives |

### Implementation Correctness

| Level | Guidance |
| --- | --- |
| Not Ready Yet | Code does not run or fails core requirements |
| Competent | Code runs and solves main problem with minor issues |
| Exceptional | Code is correct, robust, and handles edge cases appropriately |

### Code Understanding

| Level | Guidance |
| --- | --- |
| Not Ready Yet | Cannot explain key lines or logic |
| Competent | Can explain general flow and most lines |
| Exceptional | Can clearly explain specific lines, control flow, and data transformations |

### AI Use Reasoning

| Level | Guidance |
| --- | --- |
| Not Ready Yet | Cannot describe how AI was used or relies blindly on output |
| Competent | Describes basic AI use and some verification |
| Exceptional | Demonstrates intentional AI use, validation, and thoughtful modification |

## Overall Guidance

| Level | Guidance |
| --- | --- |
| Not Ready Yet | Weakness in multiple dimensions or major gap in understanding |
| Competent | Solid correctness with reasonable understanding and decomposition |
| Exceptional | Strong across all dimensions including deep understanding and intentional AI use |
