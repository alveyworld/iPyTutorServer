# Week 8 Assessment Rubric

**Scope:** lists only — indexing, methods, iteration, `len()`. Dictionaries are not assessed and must not appear in prompts or examples.

## Not Yet Ready

### Conceptual Answer

- significant errors about lists
- confuses list indexing with function calls or other syntax
- cannot articulate when a list is appropriate vs scalar variables
- fundamental misunderstanding of list methods or iteration

### Code Answer

- non-functional or major syntax/logic errors
- wrong list usage (e.g. wrong indexing, `.append()` on a non-list, off-by-one in loops)
- does not process or format list data correctly
- completely misses the prompt

### AI Use Answer

- copying code without understanding
- cannot explain how they verified the AI's output
- unethical use or dependency that inhibits learning

### Example

**Rating:** Not Yet Ready

```python
first = items(0)
last = items(-1)
```

**Issues:**

- uses items(0) instead of items[0]—confuses function call with indexing
- items(-1) same error—indexing uses square brackets
- fundamental confusion between list indexing and function syntax

## Competent

### Conceptual Answer

- factually correct about lists
- can describe indexing, negative indices, and common list methods
- can describe iterating over a list with `for`
- may lack depth on edge cases or efficient patterns

### Code Answer

- complete and functional
- correct list usage (indexing, methods, iteration as required)
- correct processing (loop, aggregate, format output)
- may have minimal error handling or comments

### AI Use Answer

- using AI for list operations, debugging, or explanation
- can articulate how they tested and integrated the suggestion
- healthy, supplemental use of the tool

### Example

**Rating:** Competent

```python
first = items[0]
last = items[-1]
```

**Strengths:**

- correct indexing with square brackets
- items[0] for first, items[-1] for last
- functional and correct

## Exceptional

### Conceptual Answer

- correct, clear, comprehensive
- concrete examples
- connects to problem decomposition (collect items, process collection, output)
- may discuss empty list or invalid index handling

### Code Answer

- complete, functional, clear
- appropriate list usage throughout
- clear processing and variable names
- readable formatted output
- may handle empty list or boundary cases

### AI Use Answer

- AI to critique list logic, edge cases, or generate test data
- strategic use to deepen understanding
- improve code quality

### Example

**Rating:** Exceptional

```python
first = items[0]   # first element (index 0)
last = items[-1]   # last element (negative index)
```

**Strengths:**

- correct indexing [0] and [-1]
- brief comments clarify meaning
- clear and correct

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
