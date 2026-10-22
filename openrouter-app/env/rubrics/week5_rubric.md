# Week 5 Assessment Rubric

Week 5 covers conditionals end to end: `if`, `elif`, `else`, comparison operators, and boolean logic. Everything from weeks 1–4 (variables, `input()` and type casting, string methods) may be used alongside conditionals; loops (week 6), `while` loops and menus (week 7), lists (week 8), and files (week 9) are not expected. Each level below gives two worked examples — one single-branch `if`/`else`, one multi-branch `elif` chain — so both halves of the concept are graded against the same criteria.

## Not Yet Ready

### Conceptual Answer

- significant errors about conditionals, comparisons, or boolean logic
- confuses if/elif/else or when each applies
- cannot articulate when to use elif instead of a separate if
- does not understand boolean expressions (and, or, not)
- fundamental misunderstanding of control flow

### Code Answer

- non-functional or major logic errors
- wrong or missing if/elif/else branches
- a branch that can never run, or overlapping conditions so more than one branch runs
- wrong comparison operators (e.g. = instead of ==)
- no clear decision structure (branches do not match the cases in the prompt)
- completely misses the prompt

### AI Use Answer

- copying code without understanding
- cannot explain how they verified the AI's output
- unethical use or dependency that inhibits learning

### Examples

**Rating:** Not Yet Ready

Single-branch conditional:

```python
if age = 18:
  print('adult')
else print('minor')
```

**Issues:**

- uses = instead of == (assignment not comparison)
- missing colon after else
- does not use >= for 18 or more
- syntax and logic errors

Multi-branch conditional:

```python
if grade >= 90: print('A')
if grade >= 80: print('B')
if grade >= 70: print('C')
if grade < 70: print('F')
```

**Issues:**

- uses separate ifs instead of elif—multiple letters print for same grade
- does not use elif so one grade can match several branches
- fundamental misunderstanding of mutually exclusive branches

## Competent

### Conceptual Answer

- factually correct
- can describe if/elif/else and when each applies
- can describe boolean expressions and comparison operators
- can explain why the order of elif conditions matters
- can describe combining conditions with and, or, and not
- may lack depth on edge cases (boundary values such as exactly 18)

### Code Answer

- complete and functional
- correct if/elif/else logic
- every case in the prompt reaches exactly one branch
- correct comparison operators (==, !=, etc.)
- may have long if/elif chains or minimal error handling

### AI Use Answer

- using AI for conditionals, debugging, or explanation
- can articulate how they tested and integrated the suggestion
- healthy, supplemental use of the tool

### Examples

**Rating:** Competent

Single-branch conditional:

```python
if age >= 18:
    print('adult')
else:
    print('minor')
```

**Strengths:**

- correct >= for 18 or more
- correct if/else with proper colons
- functional and clear

Multi-branch conditional:

```python
if grade >= 90:
    print('A')
elif grade >= 80:
    print('B')
elif grade >= 70:
    print('C')
else:
    print('F')
```

**Strengths:**

- correct elif chain—mutually exclusive
- correct thresholds (90+, 80-89, 70-79)
- functional and correct

## Exceptional

### Conceptual Answer

- correct, clear, comprehensive
- concrete examples
- connects to problem decomposition (decision points and the outcomes each one has)
- may discuss nested conditionals or combining conditions with and/or/not
- may discuss handling invalid input

### Code Answer

- complete, functional, clear
- well-structured conditionals
- a clear branch for every case in the prompt, including else for unexpected values
- sensible handling of invalid input
- readable variable names and structure

### AI Use Answer

- AI to critique control flow, suggest conditions, or generate tests
- strategic use to deepen understanding
- improve structure and edge-case handling

### Examples

**Rating:** Exceptional

Single-branch conditional:

```python
if age >= 18:
    print('adult')
else:
    print('minor')
# Edge case: if age could be invalid, we might check age >= 0 first.
```

**Strengths:**

- correct >= and if/else
- clear and readable
- brief comment on edge case

Multi-branch conditional:

```python
if grade >= 90:
    print('A')
elif grade >= 80:
    print('B')
elif grade >= 70:
    print('C')
else:
    print('F')
# Order matters: check highest first so each grade hits exactly one branch.
```

**Strengths:**

- correct elif chain and thresholds
- comment explains why order matters
- clear structure

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
