# Week 2 lessons: Variables & Expressions

Lesson plans for iPyTutor. Each `##` heading is one lesson, taught in order, and its bullets
are the points the tutor must cover. The tutor reads this plan when it starts, so edit freely.
Keep every lesson inside this week's scope: the scope test (`go test`) flags later-week ideas.

## Variables and assignment
- A variable is a name that refers to a value; `=` assigns the value on the right to the name on the left.
- Assignment works right to left: `total = price * quantity` computes the right side first, then stores it.
- Variables can be reassigned, and the old value is replaced: `count = count + 1`.
- Use clear, meaningful names in snake_case (`unit_price`, not `x`); names can't start with a digit or contain spaces.

## Data types: int, float, str and bool
- `int` is a whole number (`42`), `float` a number with a decimal point (`3.5`), `str` text in quotes (`"Ava"`), and `bool` is `True` or `False`.
- `type(value)` shows a value's type, which helps when a result looks wrong.
- The type matters: `"5"` is text, not the number 5, so `"5" + "5"` joins to `"55"`.
- Pick the type that fits the data: a count of samples is an int, a temperature reading a float, a name a str, a yes/no flag a bool.

## Arithmetic expressions and operator precedence
- Operators: `+`, `-`, `*`, `/` (always gives a float), `//` (whole-number division), `%` (remainder) and `**` (power).
- Python follows precedence: `**` first, then `*`, `/`, `//` and `%`, then `+` and `-`; use parentheses to make the order you mean explicit.
- Classic mistake: `average = a + b / 2` divides only `b`; write `average = (a + b) / 2`.
- Mixing an int and a float in one expression gives a float.

## Showing results with print()
- `print()` displays values; separate several values with commas: `print("Total:", total)`.
- An f-string puts values inside text: `print(f"Total: {total}")`.
- Joining text and a number with `+` (`"Total: " + 5`) is a TypeError; use a comma or an f-string instead.
- Show money or measurements to a set number of decimal places with `{value:.2f}` inside an f-string.

## Tracing code: predicting what a variable holds
- Read code top to bottom and track each variable's value after every line, like a table of values.
- Reassignment changes a value only from that line on; earlier lines used the old value.
- Predicting the exact output before running code is a key interview skill and the fastest way to find bugs.
- Decompose calculations: store intermediate results in well-named variables (`subtotal`, `tax`, `total`) so each step is easy to check.
