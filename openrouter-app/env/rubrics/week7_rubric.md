# Week 7 Assessment Rubric

## Assessment scope (Week 7 — while loops and menus)

**In scope:** `while` loops with a condition in the header (e.g. `while choice != 'Q'`), menus, conditionals, repeat-until-quit logic.

**Never prompt or require:** `while True`, `break`, or `continue`. Do not design tasks or lead conceptual questions to elicit those constructs.

**If the student uses them:** When submitted code (or a voluntary explanation) includes `while True`, `break`, or `continue`, treat it as acceptable. Ask pointed interview follow-ups: why they chose that approach and what advantage it had for this problem. Do not penalize for using them; assess understanding of their choice.

**Examples in this rubric** use condition-driven `while` loops only; do not introduce `while True` or `break` in model solutions.

**Forbidden constructs when assessing Week 7:** lists, file I/O, or any task that requires storing multiple records in a collection or reading external data files. Prior weeks (variables, input, strings, conditionals, `for`, `while` menus) are allowed as support.

## Not Yet Ready

### Conceptual Answer

- significant errors about conditionals or loops
- confuses if/elif/else or when each applies
- cannot articulate when to use for vs while
- does not understand boolean expressions (and, or, not)
- fundamental misunderstanding of control flow

### Code Answer

- non-functional or major logic errors
- wrong or missing if/elif/else branches
- infinite loop or loop that never runs
- wrong comparison operators (e.g. = instead of ==)
- no clear menu or control structure
- completely misses the prompt

### AI Use Answer

- copying code without understanding
- cannot explain how they verified the AI's output
- unethical use or dependency that inhibits learning

### Example

**Rating:** Not Yet Ready

```python
choice = input('A or B or Q')
if choice = 'A':
  print('A')
elif choice = 'B':
  print('B')
elif choice == 'Q':
  print('Goodbye.')
else:
  print('Invalid choice.')
```

**Issues:**

- uses = instead of == for comparison (syntax/assignment error)
- no `while` loop—menu runs once and exits
- fails to repeat until user quits
- fails to implement a menu structure

## Competent

### Conceptual Answer

- factually correct
- can describe if/elif/else and when each applies
- can describe boolean expressions and comparison operators
- can explain for vs while with reasonable criteria
- can describe menu loop (repeat until quit)
- may lack depth on edge cases or combining structures

### Code Answer

- complete and functional
- correct if/elif/else and loop logic
- menu repeats until user quits
- correct comparison operators (==, !=, etc.)
- may have long if/elif chains or minimal error handling

### AI Use Answer

- using AI for conditionals/loops, debugging, or explanation
- can articulate how they tested and integrated the suggestion
- healthy, supplemental use of the tool

### Example

**Rating:** Competent

```python
choice = ''
while choice != 'Q':
    print('A - Option A')
    print('B - Option B')
    print('Q - Quit')
    choice = input('Choice: ').strip().upper()
    if choice == 'A':
        print('You chose A.')
    elif choice == 'B':
        print('You chose B.')
    elif choice != 'Q':
        print('Invalid. Try A, B, or Q.')
```

**Strengths:**

- while loop until Q
- correct == for comparisons
- if/elif for A, B, and invalid
- strip().upper() for flexible input
- functional menu

## Exceptional

### Conceptual Answer

- correct, clear, comprehensive
- concrete examples
- connects to problem decomposition (decision points, repetition, exit condition)
- may discuss combining conditionals and loops
- may discuss handling invalid input

### Code Answer

- complete, functional, clear
- well-structured conditionals and loops
- clear menu with explicit exit condition
- sensible handling of invalid input
- readable variable names and structure

### AI Use Answer

- AI to critique control flow, suggest conditions, or generate tests
- strategic use to deepen understanding
- improve structure and edge-case handling

### Example

**Rating:** Exceptional

```python
choice = ''
while choice != 'Q':
    print('\n--- Menu ---')
    print('A - Option A')
    print('B - Option B')
    print('Q - Quit')
    choice = input('Your choice: ').strip().upper()

    if choice == 'A':
        print('You chose A.')
    elif choice == 'B':
        print('You chose B.')
    elif choice != 'Q':
        print('Please enter A, B, or Q.')
print('Goodbye.')
```

**Strengths:**

- clear `while` loop; exit when choice becomes Q via the loop condition
- readable menu layout and prompt
- correct if/elif/else with invalid-input handling
- goodbye message after loop
- strip().upper() for flexible input

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
