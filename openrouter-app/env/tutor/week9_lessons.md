# Week 9 lessons: Lists and Files

Lesson plans for iPyTutor. Each `##` heading is one lesson, taught in order, and its bullets
are the points the tutor must cover. The tutor reads this plan when it starts, so edit freely.
Keep every lesson inside this week's scope: the scope test (`go test`) flags later-week ideas.

## Opening and reading a file
- `with open("scores.txt", "r") as f:` opens a file and closes it automatically when the block ends.
- `f.read()` reads the whole file as one string, and `for line in f:` reads it one line at a time.
- Every line read from a file is a string that ends with a newline character, `"\n"`.
- If the file isn't where the program expects, `open()` raises a FileNotFoundError.

## Cleaning and splitting lines
- Remove the newline with `line.strip()` before comparing or converting a line.
- Split a comma-separated line into fields: `name, score = line.strip().split(",")`.
- Cast each field to the right type: `score = int(score)`.
- A header line such as `name,score` must be skipped before casting, or `int()` raises a ValueError.

## Loading a file into a list
- Start with an empty list and `append()` each cleaned, cast value as you read the lines.
- Once the data is in a list, use Week 8 skills to total, average, sort or search it.
- Values appended as strings sort and add as text, so cast them before appending.
- An open file can be read through only once: a second loop over the same open file finds nothing.

## Writing to a file
- Open with `"w"` to write (it replaces the file's contents) or `"a"` to add to the end.
- `f.write(text)` doesn't add a newline, so write the newline yourself: `f.write(f"{name},{score}\n")`.
- `write()` takes a string, so cast numbers with `str()` or use an f-string.
- Use `with open(...)` so the file is closed and saved properly.

## Read, process, write: putting it together
- Decompose file programs into read (open, loop over lines, clean, cast, store in a list), process (compute), and write or display the results.
- Test with a tiny file of two or three lines so you can predict every value.
- When something fails, print `repr(line)` to see hidden newlines and spaces.
- In the interview, explain why each step is needed and what happens with a missing file or a bad line.
