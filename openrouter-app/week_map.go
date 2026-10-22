package main

import "strings"

type weekSelection struct {
	SelectedKeyConcept string
	CurrentWeekNumber  int
}

var weeklyKeyConceptSelections = []weekSelection{
	{SelectedKeyConcept: "Week 1 - Problem Decomposition", CurrentWeekNumber: 1},
	{SelectedKeyConcept: "Week 2 - Variables & Expressions", CurrentWeekNumber: 2},
	{SelectedKeyConcept: "Week 3 - Input & Type Casting", CurrentWeekNumber: 3},
	{SelectedKeyConcept: "Week 4 - String Methods", CurrentWeekNumber: 4},
	{SelectedKeyConcept: "Week 5 - Conditionals (if/elif/else)", CurrentWeekNumber: 5},
	{SelectedKeyConcept: "Week 6 - for Loops (Repetition over sequences)", CurrentWeekNumber: 6},
	{SelectedKeyConcept: "Week 7 - while Loops & Menus", CurrentWeekNumber: 7},
	{SelectedKeyConcept: "Week 8 - Lists", CurrentWeekNumber: 8},
	{SelectedKeyConcept: "Week 9 - Lists and Files", CurrentWeekNumber: 9},
}

var weekInputAliases = map[string]weekSelection{
	"1":                         {"Week 1 - Problem Decomposition", 1},
	"week 1":                    {"Week 1 - Problem Decomposition", 1},
	"problem decomposition":     {"Week 1 - Problem Decomposition", 1},
	"2":                         {"Week 2 - Variables & Expressions", 2},
	"week 2":                    {"Week 2 - Variables & Expressions", 2},
	"variables":                 {"Week 2 - Variables & Expressions", 2},
	"variables and expressions": {"Week 2 - Variables & Expressions", 2},
	"3":                         {"Week 3 - Input & Type Casting", 3},
	"week 3":                    {"Week 3 - Input & Type Casting", 3},
	"input":                     {"Week 3 - Input & Type Casting", 3},
	"type casting":              {"Week 3 - Input & Type Casting", 3},
	"input and type casting":    {"Week 3 - Input & Type Casting", 3},
	"4":                         {"Week 4 - String Methods", 4},
	"week 4":                    {"Week 4 - String Methods", 4},
	"strings":                   {"Week 4 - String Methods", 4},
	"string methods":            {"Week 4 - String Methods", 4},
	"5":                         {"Week 5 - Conditionals (if/elif/else)", 5},
	"week 5":                    {"Week 5 - Conditionals (if/elif/else)", 5},
	"if":                        {"Week 5 - Conditionals (if/elif/else)", 5},
	"if statements":             {"Week 5 - Conditionals (if/elif/else)", 5},
	"conditionals":              {"Week 5 - Conditionals (if/elif/else)", 5},
	"elif":                      {"Week 5 - Conditionals (if/elif/else)", 5},
	"else":                      {"Week 5 - Conditionals (if/elif/else)", 5},
	"elif else":                 {"Week 5 - Conditionals (if/elif/else)", 5},
	"if elif else":              {"Week 5 - Conditionals (if/elif/else)", 5},
	"6":                         {"Week 6 - for Loops (Repetition over sequences)", 6},
	"week 6":                    {"Week 6 - for Loops (Repetition over sequences)", 6},
	"for":                       {"Week 6 - for Loops (Repetition over sequences)", 6},
	"for loops":                 {"Week 6 - for Loops (Repetition over sequences)", 6},
	"7":                         {"Week 7 - while Loops & Menus", 7},
	"week 7":                    {"Week 7 - while Loops & Menus", 7},
	"while":                     {"Week 7 - while Loops & Menus", 7},
	"while loops":               {"Week 7 - while Loops & Menus", 7},
	"menus":                     {"Week 7 - while Loops & Menus", 7},
	"while loops and menus":     {"Week 7 - while Loops & Menus", 7},
	"8":                         {"Week 8 - Lists", 8},
	"week 8":                    {"Week 8 - Lists", 8},
	"lists":                     {"Week 8 - Lists", 8},
	"9":                         {"Week 9 - Lists and Files", 9},
	"week 9":                    {"Week 9 - Lists and Files", 9},
	"files":                     {"Week 9 - Lists and Files", 9},
	"lists and files":           {"Week 9 - Lists and Files", 9},
}

func normalizeUserInput(text string) string {
	return strings.TrimSpace(strings.ToLower(text))
}

func matchWeekSelection(userMessage string) (weekSelection, bool) {
	normalized := normalizeUserInput(userMessage)
	if normalized == "" {
		return weekSelection{}, false
	}
	if sel, ok := weekInputAliases[normalized]; ok {
		return sel, true
	}
	for _, concept := range weeklyKeyConceptSelections {
		if strings.EqualFold(strings.TrimSpace(userMessage), concept.SelectedKeyConcept) {
			return concept, true
		}
	}
	return weekSelection{}, false
}

var greetingPhrases = []string{
	"hi", "hello", "hey", "good morning", "good afternoon", "good evening",
	"thanks", "thank you", "ok", "okay", "start",
}

func isGreetingMessage(text string) bool {
	normalized := normalizeUserInput(text)
	for _, phrase := range greetingPhrases {
		if normalized == phrase {
			return true
		}
	}
	return false
}
