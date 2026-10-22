# Week 3 lessons: Input & Type Casting

Lesson plans for iPyTutor. Each `##` heading is one lesson, taught in order, and its bullets
are the points the tutor must cover. The tutor reads this plan when it starts, so edit freely.
Keep every lesson inside this week's scope: the scope test (`go test`) flags later-week ideas.

## Getting input from the user
- `input("prompt")` shows the prompt, waits for the user to type, and returns what they typed.
- Always store the result in a variable: `name = input("Your name: ")`.
- Write clear prompts that say what to enter and in what unit, for example `"Weight in kg: "`.
- `input()` always returns a str, even when the user types digits.

## Converting types with int(), float() and str()
- `int("42")` gives 42, `float("3.5")` gives 3.5, and `str(42)` gives `"42"`.
- Convert as soon as you read a number: `age = int(input("Age: "))`.
- Choose `float()` for values that can have decimals (prices, measurements) and `int()` for whole counts.
- A cast returns a new value: `int(age)` on its own line changes nothing unless you assign it, as in `age = int(age)`.

## Why types cause bugs
- Adding two strings joins them: `"5" + "3"` is `"53"`, not 8, a sign the inputs were never converted.
- `int("4.5")` crashes with a ValueError; `int(float("4.5"))` gives 4 by dropping the decimal part.
- `int(3.9)` is 3: int truncates, it doesn't round.
- When a result looks wrong, print the value and `type()` of each variable to see what you really have.

## Read, convert, compute, display
- Most programs this week follow one pattern: read the input, convert it, compute the result, display it.
- Decompose first: list each input and its type, the calculation, and the output format.
- Use f-strings to display results clearly: `print(f"BMI: {bmi:.1f}")`.
- Use `str()` when you need to join a number onto text with `+`.

## Thinking about bad input
- If the user types text where a number is expected, `int()` or `float()` raises a ValueError and the program stops.
- You can't prevent that yet, but you should know it happens and be able to explain why.
- Mentioning this edge case in the interview is part of an Exceptional answer: it shows you think about what could go wrong.
- Good prompts reduce mistakes: tell the user exactly what format to type.
