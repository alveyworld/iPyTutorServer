# Week 4 lessons: String Methods

Lesson plans for iPyTutor. Each `##` heading is one lesson, taught in order, and its bullets
are the points the tutor must cover. The tutor reads this plan when it starts, so edit freely.
Keep every lesson inside this week's scope: the scope test (`go test`) flags later-week ideas.

## Strings and methods
- A string is a sequence of characters; a method is a function attached to a value, called with a dot: `name.upper()`.
- String methods return a new string and the original never changes, so assign the result: `name = name.strip()`.
- Forgetting the parentheses (`name.upper` instead of `name.upper()`) gives the method itself, not its result.
- Methods can be chained left to right: `raw.strip().lower()`.

## Cleaning text: strip(), upper(), lower() and replace()
- `strip()` removes spaces and newlines from both ends, which is essential for cleaning what a user types.
- `upper()` and `lower()` change letter case so text can be displayed or compared consistently.
- `replace(old, new)` swaps every occurrence of one piece of text for another: `phone.replace("-", "")`.
- Putting the arguments to `replace()` in the wrong order is a common bug.

## Checking content: isdigit(), isalpha(), isalnum(), isupper() and islower()
- These methods return True or False instead of a new string.
- `isdigit()` is True only when every character is a digit: True for `"42"`, False for `"-5"`, `"3.5"` or an empty string.
- `isalpha()` checks for letters only, `isalnum()` for letters or digits, and `isupper()` and `islower()` check letter case.
- They tell you whether text is valid before you cast it, which you'll act on when you learn decisions next week.

## Splitting text with split()
- `split()` breaks a string into pieces at spaces; `split(",")` breaks it at commas.
- With a known number of pieces you can unpack them straight into variables: `first, last = full_name.split()`.
- Splitting on the wrong separator is a common bug, and `"a, b".split(",")` keeps the space before b, so strip each piece.
- Unpacking raises a ValueError if the number of pieces doesn't match the number of variables.

## Formatting output with f-strings
- An f-string starts with `f` and puts expressions inside braces: `f"Student: {name}, Score: {score}%"`.
- Format numbers inside the braces: `{price:.2f}` shows two decimal places.
- Forgetting the `f` prints the braces literally, a common output bug.
- Plan the exact output format during decomposition: what the user should see, in what order and style.
