# Final Per-Concept Assessment Rubric

## Rating Buckets

- Not Ready Yet
- Competent
- Exceptional

## Mode Buckets

- **Conceptual Assessment Bucket:** Mode: ConceptUnderstandingMode
- **Code Assessment Bucket:** Mode: CodeProblemMode (derived from decomposition quality, correctness, code understanding, and AI-use reasoning)
- **Bug Assessment Bucket:** Mode: BugHuntingMode

## Aggregation Rules

### Rule 1

**Condition:** If ANY of the three mode buckets is 'Not Ready Yet'.

**Result:** Final rating is 'Not Ready Yet'.

The most conservative rule: a single Not Ready Yet in any mode means the student is not yet ready on this key concept.

**Example:**

- conceptualAssessmentBucket: Competent
- codeAssessmentBucket: Exceptional
- bugAssessmentBucket: Not Ready Yet
- finalRating: Not Ready Yet

### Rule 2

**Condition:** Otherwise, if at least one mode bucket is 'Competent' and NONE are 'Not Ready Yet'.

**Result:** Final rating is 'Competent'.

If the student is at least competent in one or more modes, and not flagged as Not Ready Yet anywhere, the overall rating is Competent.

**Example:**

- conceptualAssessmentBucket: Competent
- codeAssessmentBucket: Competent
- bugAssessmentBucket: Exceptional
- finalRating: Competent

### Rule 3

**Condition:** Otherwise (no Not Ready Yet, no Competent), all three mode buckets are 'Exceptional'.

**Result:** Final rating is 'Exceptional'.

Only when all three assessment modes rate the student as Exceptional is the final rating Exceptional.

**Example:**

- conceptualAssessmentBucket: Exceptional
- codeAssessmentBucket: Exceptional
- bugAssessmentBucket: Exceptional
- finalRating: Exceptional

## Aggregation Algorithm

Apply aggregation rules in priority order to determine a single final rating for the chosen key concept.

### Step 1

**Action:** Check if conceptualAssessmentBucket == 'Not Ready Yet' OR codeAssessmentBucket == 'Not Ready Yet' OR bugAssessmentBucket == 'Not Ready Yet'.

**If true:** Return 'Not Ready Yet' and stop.

**If false:** Continue to step 2.

### Step 2

**Action:** Check if at least one of the three buckets is 'Competent' AND none of them is 'Not Ready Yet'.

**If true:** Return 'Competent' and stop.

**If false:** Continue to step 3.

### Step 3

**Action:** By elimination, all three buckets must be 'Exceptional'.

**Result:** Return 'Exceptional'.

## Final Rating Levels

### Not Ready Yet

At least one of the three assessment modes raised a Not Ready Yet flag. The student has important gaps to address before this key concept can be considered ready.

**Requirements:**

- conceptualAssessmentBucket == 'Not Ready Yet' OR codeAssessmentBucket == 'Not Ready Yet' OR bugAssessmentBucket == 'Not Ready Yet'.

### Competent

None of the three assessment modes rated the student as Not Ready Yet, and at least one mode rated them as Competent. The student has solid performance on this key concept.

**Requirements:**

- No mode bucket is 'Not Ready Yet'.
- At least one mode bucket is 'Competent'.

### Exceptional

All three assessment modes rated the student as Exceptional. The student shows outstanding mastery of this key concept across conceptual understanding, coding, and debugging strategy.

**Requirements:**

- conceptualAssessmentBucket == 'Exceptional'.
- codeAssessmentBucket == 'Exceptional'.
- bugAssessmentBucket == 'Exceptional'.

## Usage Notes

- This rubric is applied ONLY to a single weekly key concept per IPyIntervu interaction.
- There is no aggregation across multiple key concepts; each concept receives its own final rating using these rules.
- Mode-specific buckets should themselves be derived from the appropriate weekly rubric (week{N}_rubric.jsonld) based on currentWeekNumber in week{N}_key_concepts.md.
- The conservative ordering ensures that Not Ready Yet in any mode dominates, Competent is the middle ground when there are no Not Ready Yet flags and at least one Competent, and Exceptional is reserved for the rare case where all three modes rate the student as Exceptional.
- SPECIAL CASE - Week 1 Problem Decomposition: Only conceptualAssessmentBucket is graded. Set finalRating = conceptualAssessmentBucket (must be exactly **Not Ready Yet**, **Competent**, or **Exceptional**). Report codeAssessmentBucket and bugAssessmentBucket as **N/A**. Never report Strong, Good, or other synonyms in user-facing results.

## Assessment Evidence Dimensions

- **Conceptual Mode:** Concept understanding, explanation, and scenario reasoning.
- **Code Mode:** Problem decomposition, correctness of submitted code, line-level code understanding, and AI-use reasoning.
- **Bug Mode:** Debugging strategy, hypotheses, narrowing process, and relation to the selected concept.
