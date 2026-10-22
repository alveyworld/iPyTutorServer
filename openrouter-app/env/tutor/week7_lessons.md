# Week 7 lessons: while Loops & Menus

Lesson plans for iPyTutor. Each `##` heading is one lesson, taught in order, and its bullets
are the points the tutor must cover. The tutor reads this plan when it starts, so edit freely.
Keep every lesson inside this week's scope: the scope test (`go test`) flags later-week ideas.

## Repeating while a condition is true
- A `while` loop checks its condition before each pass and repeats its block for as long as the condition is True.
- Use `while` when you don't know in advance how many passes you need; use `for` when you do.
- Something inside the loop must change the condition's variable, or the loop never ends (an infinite loop).
- Set up the loop variable before the loop so the first check has a value to test.

## Repeat until the user quits
- A sentinel value such as `"q"` tells the loop to stop: `while choice != "q":`.
- Read the choice before the loop and again at the end of each pass, so the condition sees the new value.
- Handle case differences by cleaning the input: `choice = input("Choice: ").strip().lower()`.
- Put the quit test in the `while` condition itself; the interview expects a condition in the loop header.

## Validating input with a loop
- A validation loop keeps asking until the input is acceptable: `while not age_text.isdigit():`.
- Ask once before the loop, then ask again inside it with a message saying what was wrong.
- When the loop ends you know the value is valid, so it's safe to cast it.
- Test the loop with a bad value first, then a good one.

## Building a menu program
- A menu program repeats: show the options, read a choice, act on it with `if`/`elif`/`else`, until the user quits.
- Give every option its own branch, and use `else` to answer unknown choices with a helpful message.
- Keep running totals or counts in variables set up before the loop.
- Decompose a menu program into setup, the loop condition, the menu display, handling each choice, and what happens on quit.

## Debugging loops that never end or end too soon
- If a loop never ends, check whether the condition's variable changes inside the loop.
- Compound conditions are easy to get wrong: `choice != "q" or choice != "Q"` is always True; use `and`, or clean the case first.
- Print the condition's variable at the top of each pass to watch it change.
- Count the passes for a small example and compare with what you expected.
