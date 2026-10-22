package main

import (
	"regexp"
	"strings"
)

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
	"1": weekSelection{"Week 1 - Problem Decomposition", 1},
	"week 1": {"Week 1 - Problem Decomposition", 1},
	"problem decomposition": {"Week 1 - Problem Decomposition", 1},
	"2": {"Week 2 - Variables & Expressions", 2},
	"week 2": {"Week 2 - Variables & Expressions", 2},
	"variables": {"Week 2 - Variables & Expressions", 2},
	"variables and expressions": {"Week 2 - Variables & Expressions", 2},
	"3": {"Week 3 - Input & Type Casting", 3},
	"week 3": {"Week 3 - Input & Type Casting", 3},
	"input": {"Week 3 - Input & Type Casting", 3},
	"type casting": {"Week 3 - Input & Type Casting", 3},
	"input and type casting": {"Week 3 - Input & Type Casting", 3},
	"4": {"Week 4 - String Methods", 4},
	"week 4": {"Week 4 - String Methods", 4},
	"strings": {"Week 4 - String Methods", 4},
	"string methods": {"Week 4 - String Methods", 4},
	"5": {"Week 5 - Conditionals (if/elif/else)", 5},
	"week 5": {"Week 5 - Conditionals (if/elif/else)", 5},
	"if": {"Week 5 - Conditionals (if/elif/else)", 5},
	"if statements": {"Week 5 - Conditionals (if/elif/else)", 5},
	"conditionals": {"Week 5 - Conditionals (if/elif/else)", 5},
	"elif": {"Week 5 - Conditionals (if/elif/else)", 5},
	"else": {"Week 5 - Conditionals (if/elif/else)", 5},
	"elif else": {"Week 5 - Conditionals (if/elif/else)", 5},
	"if elif else": {"Week 5 - Conditionals (if/elif/else)", 5},
	"6": {"Week 6 - for Loops (Repetition over sequences)", 6},
	"week 6": {"Week 6 - for Loops (Repetition over sequences)", 6},
	"for": {"Week 6 - for Loops (Repetition over sequences)", 6},
	"for loops": {"Week 6 - for Loops (Repetition over sequences)", 6},
	"7": {"Week 7 - while Loops & Menus", 7},
	"week 7": {"Week 7 - while Loops & Menus", 7},
	"while": {"Week 7 - while Loops & Menus", 7},
	"while loops": {"Week 7 - while Loops & Menus", 7},
	"menus": {"Week 7 - while Loops & Menus", 7},
	"while loops and menus": {"Week 7 - while Loops & Menus", 7},
	"8": {"Week 8 - Lists", 8},
	"week 8": {"Week 8 - Lists", 8},
	"lists": {"Week 8 - Lists", 8},
	"9": {"Week 9 - Lists and Files", 9},
	"week 9": {"Week 9 - Lists and Files", 9},
	"files": {"Week 9 - Lists and Files", 9},
	"lists and files": {"Week 9 - Lists and Files", 9},
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

// Longest message still taken for a request for coaching. The phrase used to match
// anywhere in any message, so a code answer with print("coaching ...") in it, or a short
// answer about a coaching business, was taken for a request and the answer was lost.
const (
	coachingRequestMaxWords      = 25
	midInterviewCoachingMaxWords = 8
)

// isCoachingRequest reports whether text asks for coaching. Code and long messages never
// do. Use isMidInterviewCoachingRequest while the interview is still in progress.
func isCoachingRequest(text string) bool {
	normalized := normalizeUserInput(text)
	if strings.Contains(normalized, "```") || len(strings.Fields(normalized)) > coachingRequestMaxWords || d5LooksLikeCodeSubmission(text) {
		return false
	}
	return strings.Contains(normalized, "coaching") ||
		strings.Contains(normalized, "coach mode") ||
		strings.Contains(normalized, "feedback on my assessment") ||
		strings.Contains(normalized, "give me feedback")
}

// requestOpening matches how a request for coaching begins ("could I get some coaching",
// "I'd like coaching", "please"), which an answer that merely mentions coaching
// does not.
var requestOpening = regexp.MustCompile(`^(?:(?:hi|hey|ok|okay|thanks|thank you)[,.! ]+)*(?:please\b|(?:can|could|may|might) (?:i|we)\b|i(?:'d| would) like\b|i want\b|i need\b|let'?s\b|switch\b|give me\b)`)

// bareCoachingWord matches a message that is only the word "coaching".
var bareCoachingWord = regexp.MustCompile(`^coaching(?: please| now)?[.!?]*$`)

// isMidInterviewCoachingRequest is the stricter test for a message sent during the
// interview, where an answer that merely mentions coaching must not be taken for a
// request: it must be short and either name coach mode or open like a request.
func isMidInterviewCoachingRequest(text string) bool {
	if len(strings.Fields(text)) > midInterviewCoachingMaxWords || !isCoachingRequest(text) {
		return false
	}
	normalized := normalizeUserInput(text)
	if bareCoachingWord.MatchString(normalized) {
		return true
	}
	return strings.Contains(normalized, "coach mode") || strings.Contains(normalized, "feedback on my assessment") ||
		strings.Contains(normalized, "give me feedback") || requestOpening.MatchString(normalized)
}
