# Week 9 Assessment Rubric

## Not Yet Ready

### Conceptual Answer

- significant errors about lists 
- does not understand file open/read/close or with
- cannot articulate when to use list
- fundamental misunderstanding of data processing from files

### Code Answer

- non-functional or major syntax/logic errors
- wrong list usage (e.g. wrong indexing or append logic)
- file not opened correctly (e.g. write mode when reading)
- no with statement or file not closed
- does not process or format file data correctly
- completely misses the prompt

### AI Use Answer

- copying code without understanding
- cannot explain how they verified the AI's output
- unethical use or dependency that inhibits learning

### Example

**Rating:** Not Yet Ready

```python
f = open('numbers.txt')
data = f.read()
numbers = data
total = sum(numbers)
print(total, total / len(numbers))
```

**Issues:**

- f.read() returns one string, not a list of numbers
- numbers is a string; sum() and len() on a string don't give sum/count of numbers
- no splitting by newline or conversion to int/float
- file not closed (no with or close())
- fails to implement reading numbers and computing sum/average

## Competent

### Conceptual Answer

- factually correct
- can describe lists and when to use them with file data
- can describe file open/read/readlines and with
- can describe basic data processing (loop, convert, aggregate)
- may lack depth on edge cases or efficient patterns

### Code Answer

- complete and functional
- correct list usage
- correct file open and read (with open, readlines or read)
- correct processing (loop, convert, aggregate)
- correct formatted output
- may have minimal error handling or comments

### AI Use Answer

- using AI for file I/O, lists, debugging, or explanation
- can articulate how they tested and integrated the suggestion
- healthy, supplemental use of the tool

### Example

**Rating:** Competent

```python
numbers = []
with open('numbers.txt') as f:
    for line in f:
        numbers.append(float(line.strip()))
total = sum(numbers)
avg = total / len(numbers)
print(f'Sum: {total}, Average: {avg}')
```

**Strengths:**

- uses with open for safe file handling
- reads line by line, strips, converts to float
- stores in list and uses sum() and len()
- formatted output with f-string
- correct logic

## Exceptional

### Conceptual Answer

- correct, clear, comprehensive
- concrete examples
- connects to problem decomposition (input file, process, output)
- may discuss choosing list operations for file data
- may discuss empty file or bad data handling

### Code Answer

- complete, functional, clear
- appropriate list usage
- correct file handling (with open, clear reading)
- clear processing and variable names
- readable formatted output
- may handle empty file or invalid lines

### AI Use Answer

- AI to critique list usage, file handling, or generate test files
- strategic use to deepen understanding
- improve code quality

### Example

**Rating:** Exceptional

```python
# Input: read numbers from file into a list
numbers = []
with open('numbers.txt') as f:
    for line in f:
        line = line.strip()
        if line:
            numbers.append(float(line))

# Process: compute sum and average
if numbers:
    total = sum(numbers)
    average = total / len(numbers)
    print(f'Sum: {total}')
    print(f'Average: {average}')
else:
    print('No numbers in file.')
```

**Strengths:**

- clear input-process-output structure
- with open; loop over lines; strip and skip blank lines
- converts to float and appends to list
- handles empty file (if numbers: avoids divide by zero)
- formatted output; clear message when file is empty

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
