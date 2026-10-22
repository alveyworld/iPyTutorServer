# Week 5: Conditionals (if/elif/else)

## Problem Decomposition Context

Many problems require decisions. Identify decision points, then identify how many outcomes each decision has. A single `if` handles one decision; `elif`/`else` handle a decision with several mutually exclusive outcomes. Example: Determine pass/fail → decision: score >= 70? Example: Assign a letter grade → one decision, four mutually exclusive branches.

## Key Concepts Overview

if statements. elif and else branches. Multi-branch conditionals and why branch order matters. Boolean expressions with `and`, `or`, `not`. Comparison operators: ==, !=, <, >, <=, >=. Problem decomposition: where decisions are needed and how many outcomes each has.

## Simple Example Demonstration

Determine if student passed. Input (score), Process (check score >= 70), Output (pass/fail). if score >= 70: print('Pass').

Extend to several outcomes with an elif chain. Input (score), Process (check thresholds highest first), Output (letter grade). if score >= 90: print('A') / elif score >= 80: print('B') / else: print('F') — each score matches exactly one branch.

## Connection to AI Tools

Pythonista2: How do if statements work? What are comparison operators? When do you need elif instead of another if? Practice decision points in domain.
