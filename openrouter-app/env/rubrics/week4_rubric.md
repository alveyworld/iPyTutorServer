# Week 4 Assessment Rubric

## Not Yet Ready

### Conceptual Answer

- significant factual errors about variables, types, or I/O
- overly vague or simplistic
- fundamental misunderstanding of type casting or input()
- cannot articulate when to use int(), float(), or str()
- confuses data types or expression evaluation

### Code Answer

- non-functional or major syntax errors
- uses input() but does not cast to int/float when needed
- wrong or missing variables for the problem
- incorrect expressions or arithmetic
- no or incorrect use of print() or formatted strings
- completely misses the prompt

### AI Use Answer

- copying code from an AI without understanding it
- cannot explain how they verified the AI's output
- unethical use or dependency that inhibits learning

### Example

**Rating:** Not Yet Ready

```python
You just print it. Maybe use upper.
```

**Issues:**

- does not name .strip() for removing spaces
- vague about .upper() - no example
- no code showing chaining (e.g. text.strip().upper())
- does not connect to input or variables

## Competent

### Conceptual Answer

- factually correct
- covers main points (variables, types, input, output)
- can describe when to use type casting
- can describe formatted strings or string methods
- solid and accurate
- may lack depth on edge cases or best practices

### Code Answer

- complete and functional
- correct variables and data types
- correct type casting (int(), float()) where needed
- correct use of print() and formatted strings (f-strings)
- follows the prompt
- may lack clear names, comments, or invalid-input handling

### AI Use Answer

- using AI for boilerplate, debugging, or explanation
- can articulate how they tested the AI's suggestion
- can articulate how they integrated it into their work
- healthy, supplemental use of the tool

### Example

**Rating:** Competent

```python
text = input('Enter something: ')
cleaned = text.strip()
print(cleaned.upper())
```

**Strengths:**

- uses .strip() for spaces
- uses .upper() for display
- functional and correct
- could chain: text.strip().upper() but separate steps are fine

## Exceptional

### Conceptual Answer

- correct, clear, comprehensive
- concrete examples
- connects to problem decomposition
- may discuss edge cases or best practices
- meaningful variable names
- when to use str() for output

### Code Answer

- complete, functional, clear
- meaningful variable names
- correct types and type casting
- clean formatted output
- structure reflects input-process-output
- may handle invalid input or include brief comments

### AI Use Answer

- AI as collaborative partner
- e.g. critique code, suggest names, generate test cases
- strategic use to deepen understanding
- improve code quality

### Example

**Rating:** Exceptional

```python
# Get input and clean in one step: strip leading/trailing spaces, then uppercase for display
user_input = input('Enter your response: ')
display_text = user_input.strip().upper()
print(display_text)
```

**Strengths:**

- uses .strip() and .upper() correctly
- method chaining (.strip().upper())
- meaningful variable names
- brief comment clarifies intent

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
