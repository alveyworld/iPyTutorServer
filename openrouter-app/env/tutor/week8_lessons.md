# Week 8 lessons: Lists

Lesson plans for iPyTutor. Each `##` heading is one lesson, taught in order, and its bullets
are the points the tutor must cover. The tutor reads this plan when it starts, so edit freely.
Keep every lesson inside this week's scope: the scope test (`go test`) flags later-week ideas.

## Creating lists and reading items
- A list stores several values in order: `scores = [88, 92, 75]`, and an empty list is `[]`.
- Items are numbered from 0: `scores[0]` is the first item and `scores[-1]` the last.
- `len(scores)` is the number of items, so the last index is `len(scores) - 1`, and going past it raises an IndexError.
- A list can hold any type, but usually holds values of one kind, such as all scores or all names.

## Changing a list
- Assign to an index to replace an item: `scores[1] = 95`.
- `append(x)` adds to the end, `insert(i, x)` adds at a position, `remove(x)` deletes the first matching value, and `pop()` removes and returns the last item.
- `sort()` sorts the list in place and returns None, so `sorted_scores = scores.sort()` stores None.
- Lists change in place: every variable that refers to the list sees the change.

## Looping over a list
- `for score in scores:` visits each item; use it to total, count, search or find the largest value.
- `for i in range(len(scores)):` gives you the index too, when you need positions.
- Combine it with an accumulator: `total = total + score` inside the loop, then `average = total / len(scores)` after it.
- Guard against an empty list before dividing by its length.

## Building a list from input
- Start with an empty list and call `append()` inside a loop to collect values.
- With a Week 7 menu or validation loop, you can keep adding until the user is done.
- Decide during decomposition when a list is needed: when you must keep several values to use later (to sort, average or search them).
- Use `in` to check whether a value is present: `if name in names:`.

## Common list bugs
- Off-by-one: `range(1, len(scores))` skips the first item, and an index of `len(scores)` is past the end.
- Calling `append()` after the loop instead of inside it adds only one item.
- Removing items from a list while looping over it skips items; build a new list instead.
- When a result looks wrong, print the list and its length at key points.
