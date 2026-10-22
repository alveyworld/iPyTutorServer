# Week 3 Assessment Rubric

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

You use type casting when you want to get a number from the user. input() gives you a string so you have to change it.

**Issues:**

- too vague - does not name int() or float()
- does not explain that input() always returns a string
- does not give an example (e.g. int(input('Enter age: ')))
- does not connect to arithmetic or comparisons needing numbers

## Competent

### Conceptual Answer

- factually correct
- covers main points (variables, types, input, output)
- can describe when to use type casting
- can describe formatted strings (f-strings) for output
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

input() always returns a string. If you need to do math or comparisons with the value, you have to convert it. For whole numbers use int(input('prompt')), for decimals use float(input('prompt')). For example, if you ask for age and want to check if someone is 18 or older, you need int() so you can compare with 18.

**Strengths:**

- correctly states input() returns a string
- names int() and float()
- gives a clear use case (age comparison)
- accurate and practical

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

input() always returns a string, no matter what the user types. So if you need a number for arithmetic (e.g. weight, score, count), you must convert: int() for whole numbers, float() for decimals. For example, '42' from input() can't be used in weight * 2 until you do weight = float(input('Weight?')). If you only need to display or concatenate the value, you can keep it as a string. Type casting is part of the 'input' step in problem decomposition—you get raw input, then convert to the types your process step needs.

**Strengths:**

- clear and complete
- explains input() always returns string
- distinguishes int() vs float() with purpose
- concrete example with weight
- notes when casting is not needed (display only)
- connects to problem decomposition

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
