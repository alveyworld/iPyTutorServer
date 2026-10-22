# Week 7: while Loops & Menus

## Problem Decomposition Context

Some repetition continues until condition is met. Identify stopping conditions. while repeats while condition true. Example: Menu system → repeat until user quits.

## Key Concepts Overview

while loops with a condition in the header. Combining conditionals and loops. Menu systems. Problem decomposition: when to combine structures.

## In scope

- `while condition:` (e.g. `while choice != 'q'`)
- Menus that repeat until the user chooses quit
- Combining `if` / `elif` / `else` inside a loop

## Out of scope for prompts (follow up if student uses voluntarily)

- `while True`
- `break`
- `continue`

Do not assign tasks or ask questions aimed at these constructs. If the student submits code using them, ask pointed follow-ups: why that approach, and what advantage it had for this problem.

## Forbidden: later-week concepts (Week 8–9)

Do not assign programming tasks or conceptual questions that require **lists** or **file I/O**. Those are introduced in later weeks.

**Good Week 7 task shapes:** menu-driven calculator; interactive lab instrument panel; quiz app that loops until the user quits—all using `while choice != 'q'`, `if`/`elif`, variables, and `input()` only.

**Reject and rewrite tasks like:** “keep a list of all calculations”; “load menu options from a file”; “organize records by ID in a lookup structure”; “process a CSV of readings.”

## Simple Example Demonstration

Menu-driven calculator. Input (menu choice, repeat until quit), Process (handle choice), Output (result or menu). `while choice != 'q':` print menu, `choice = input()`.

## Connection to AI Tools

Pythonista2: When while vs for? How combine conditionals and loops? Practice menu-driven scripts.
