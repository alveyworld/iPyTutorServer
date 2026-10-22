# Week 1: Problem Decomposition

## Problem Decomposition Context

Introduce problem decomposition as core skill. Input → Process → Output pattern. CRITICAL: Personas MUST present a concrete, specific problem first (e.g. 'Here is a lab procedure: prepare a slide, observe at 40x and 100x, record observations.'). Then ask exactly ONE student-directed decomposition question (e.g. 'What would you identify as the input before any processing begins?') and stop — wait for the student. Do NOT decompose input/process/output together with the student in the same message. Do NOT ask vague reflective questions. The tool provides the problem; the student decomposes it. Real-world tasks only, framed around student's major. Do NOT mention Python, code, programming, or programming languages. Students have no programming experience yet.

## Key Concepts Overview

Course structure (10 weeks of new topics, 3 modules). Problem decomposition as foundational skill: identify what goes in (input), what happens (process), what comes out (output). Real-world tasks from student's domain (lab procedures, event planning, data collection, project organization).

## Simple Example Demonstration

**Grading reference only — never read aloud to the student during the interview.** Example for Biology major: lab procedure decomposition. Input: prepared slides, microscope, specimen. Process: place slide, focus, observe, record observations. Output: lab notes. Example for Business major: survey planning. Input: research question, target audience. Process: design questions, choose format, determine sample size. Output: survey instrument.

## Connection to AI Tools

Practice decomposition with Pythonista2. No code or programming in Week 1 conceptual assessment.

## Code Problem mode (Weeks 2–10) — decomposition plus code

Week 1 **conceptual** assessment covers real-world decomposition only (no Python, no code paste). Starting in **Week 2**, the **Code Problem** portion (Taylor and Morgan, after conceptual completes) assesses **both**:

1. **Task decomposition** — the student breaks down the **Python coding task** (input, process, output, variables, steps) using the same decomposition skills taught in Week 1, applied to a programming problem at the company.
2. **Code creation and entry** — after decomposition is answered, interviewers **must explicitly ask the student to paste their Python code** for the task. The student may use external AI to write the code. The interviewers evaluate the **pasted code** (correctness per `week{N}_rubric.md` codeAnswer), ask explain-code and AI-use questions, and **then** assign `codeAssessmentBucket`.

**CRITICAL for Code Problem mode:** Completing the code portion after decomposition alone is **invalid**. Do not send `"codeAssessmentPhase": "complete"` until pasted code has been requested, received, and assessed. The sequence is: present task → one decomposition question → student answers → **ask for pasted Python code** → student pastes → evaluate code → explain/AI reflection → complete plus bucket.

## Assessment ratings (Week 1 conceptual)

When the decomposition interview is finished, the closing reply must include ```_ipyintervu``` with `"conceptualAssessmentPhase": "complete"` and `"conceptualAssessmentBucket": "Not Ready Yet"|"Competent"|"Exceptional"`. Do not send `"in_progress"` while saying the portion is done or that you are moving on. The server then shows Assessment Results automatically.

When reporting results, use only **Not Ready Yet**, **Competent**, or **Exceptional** for the conceptual assessment and overall rating. Do not use Strong, Good, or similar labels.
