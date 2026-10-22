package main

import (
	"fmt"
	"regexp"
	"strings"
)

// lastSyllabusWeek is the final week of the CSE 110 syllabus.
const lastSyllabusWeek = 9

// CSE 110 syllabus: concepts first introduced each week (weeks 1..N are allowed support).
var conceptsIntroducedByWeek = map[int][]string{
	1: {
		"problem decomposition",
		"input / process / output",
		"breaking tasks into steps",
	},
	2: {
		"variables",
		"expressions",
		"assignment",
		"int, float, str, bool",
	},
	3: {
		"input()",
		"type casting",
		"int()",
		"float()",
		"str()",
	},
	4: {
		"string methods",
		"strip()",
		"upper()",
		"lower()",
		"split()",
		"replace()",
		"isdigit()",
		"isalpha()",
		"isalnum()",
		"isupper()",
		"islower()",
	},
	5: {
		"if statements",
		"elif",
		"else",
		"multi-branch conditionals",
		"comparison operators",
		"boolean expressions",
		"and",
		"or",
		"not",
	},
	6: {
		"for loops",
		"range()",
		"iterating over sequences",
	},
	7: {
		"while loops (condition in header)",
		"menus",
		"repeat until quit via while condition",
	},
	8: {
		"lists",
		"list indexing",
		"list methods (.append, .remove, .sort, etc.)",
		"len() on lists",
	},
	9: {
		"file I/O",
		"open()",
		"read()",
		"write()",
		"with open(...) for files",
		"reading and writing data files",
	},
}

func allowedWeekNumbers(currentWeek int) []int {
	if currentWeek < 1 {
		return nil
	}
	weeks := make([]int, currentWeek)
	for i := 1; i <= currentWeek; i++ {
		weeks[i-1] = i
	}
	return weeks
}

func forbiddenConceptsFromLaterWeeks(currentWeek int) []string {
	if currentWeek < 1 {
		return nil
	}
	var forbidden []string
	for week := currentWeek + 1; week <= lastSyllabusWeek; week++ {
		forbidden = append(forbidden, conceptsIntroducedByWeek[week]...)
	}
	return forbidden
}

// neverInScopeWeek marks concepts that are out of scope for every week (dictionaries).
const neverInScopeWeek = lastSyllabusWeek + 1

// scopeDetector recognizes one later-week concept in a reply. Prose patterns run on the
// whole visible reply; code patterns only on code (fenced blocks and `inline` spans), where
// words like "if" and "for" are Python rather than English.
type scopeDetector struct {
	week  int
	name  string
	prose []*regexp.Regexp
	code  []*regexp.Regexp
}

func scopePatterns(patterns ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		out[i] = regexp.MustCompile(`(?i)` + p)
	}
	return out
}

var scopeDetectors = []scopeDetector{
	{week: 3, name: "input() and type casting",
		prose: scopePatterns(`\binput\(`, `\buser (?:will |would |should |can )?(?:enters?|types?|inputs?)\b`, `\buser input\b`, `\b(?:prompts?|asks?) the user\b`, `\b(?:int|float|str)\(`, `\btype[- ]?cast`, `\bconver(?:t|ts|ted|ting|sion)\b[^.?!\n]{0,40}\bto (?:an? )?(?:int|integer|float|number|string)s?\b`)},
	{week: 4, name: "string methods",
		prose: scopePatterns(`\.(?:strip|upper|lower|split|replace|isdigit|isalpha|isalnum|isupper|islower)\s*\(`, `\bstring methods?\b`)},
	{week: 5, name: "if statements and conditionals",
		prose: scopePatterns(`\bif[- ]statements?\b`, `\bif\s*/\s*else\b`, `\bif-else\b`, `\belif\b`, `\bconditionals?\b`, `\bboolean (?:expression|logic|operator)s?\b`, `\bcomparison operators?\b`),
		code:  scopePatterns(`(?m)^\s*(?:if|elif|else)\b`, `==|!=|<=|>=`, `\b(?:and|or|not)\b`)},
	{week: 6, name: "loops",
		prose: scopePatterns(`\bloop(?:s|ing|ed)?\b`, `\biterat(?:e|es|ed|ing|ion)\b`, `\brange\s*\(`),
		code:  scopePatterns(`(?m)^\s*for\b`)},
	{week: 7, name: "while loops and menus",
		prose: scopePatterns(`\bwhile[- ]loops?\b`, `\bmenu[- ]driven\b`, `\bmenu loops?\b`),
		code:  scopePatterns(`(?m)^\s*while\b`, `\bbreak\b`, `\bcontinue\b`)},
	{week: 8, name: "lists",
		prose: scopePatterns(`\bpython lists?\b`, `\blist (?:index|indexing|methods?)\b`, `\.(?:append|remove|sort|pop|insert)\s*\(`, `\blen\s*\(`),
		code:  scopePatterns(`\[`)},
	{week: 9, name: "files",
		prose: scopePatterns(`\bfiles?\b`, `\bopen\s*\(`, `\bcsv\b`, `\.(?:read|readline|readlines|write)\s*\(`)},
	// Not taught in the course, so out of scope in every week.
	{week: neverInScopeWeek, name: "string methods the course does not teach (title, capitalize, find, startswith, endswith, join, lstrip, rstrip)",
		prose: scopePatterns(`\.(?:title|capitalize|find|startswith|endswith|join|lstrip|rstrip)\s*\(`)},
	{week: neverInScopeWeek, name: "dictionaries",
		prose: scopePatterns(`\bdict(?:ionary|ionaries|s)?\b`, `\bkey[- ]value\b`)},
}

var (
	codeFencePattern         = regexp.MustCompile("(?s)```.*?```")
	inlineCodePattern        = regexp.MustCompile("`[^`\n]+`")
	codeCommentPattern       = regexp.MustCompile(`#[^\n]*`)
	codeStringLiteralPattern = regexp.MustCompile(`"[^"\n]*"|'[^'\n]*'`)
)

func replyCodeSegments(visible string) string {
	parts := codeFencePattern.FindAllString(visible, -1)
	parts = append(parts, inlineCodePattern.FindAllString(codeFencePattern.ReplaceAllString(visible, " "), -1)...)
	for i, part := range parts {
		part = strings.Trim(part, "`")
		// Drop a fence's language tag line.
		if nl := strings.Index(part, "\n"); nl >= 0 && !strings.ContainsAny(part[:nl], " =()") {
			part = part[nl+1:]
		}
		// Comments and string literals are English, not Python: "# area and perimeter"
		// must not count as the week 5 `and` operator.
		part = codeStringLiteralPattern.ReplaceAllString(part, `""`)
		parts[i] = codeCommentPattern.ReplaceAllString(part, "")
	}
	return strings.Join(parts, "\n")
}

// outOfScopeConcepts lists concepts in a reply that are first taught after currentWeek
// (or never), as "name (week N)". The tutor checks the practice problems it writes with it.
func outOfScopeConcepts(currentWeek int, visible string) []string {
	if currentWeek < 1 {
		return nil
	}
	code := replyCodeSegments(visible)
	var found []string
	for _, d := range scopeDetectors {
		if d.week <= currentWeek {
			continue
		}
		if matchesAny(d.prose, visible) || (code != "" && matchesAny(d.code, code)) {
			if d.week == neverInScopeWeek {
				found = append(found, d.name+" (never in scope)")
			} else {
				found = append(found, fmt.Sprintf("%s (week %d)", d.name, d.week))
			}
		}
	}
	return found
}

func matchesAny(patterns []*regexp.Regexp, text string) bool {
	for _, p := range patterns {
		if p.MatchString(text) {
			return true
		}
	}
	return false
}

func learnedConceptsSummary(currentWeek int) string {
	var parts []string
	for week := 1; week <= currentWeek && week <= lastSyllabusWeek; week++ {
		parts = append(parts, fmt.Sprintf("week %d: %s", week, strings.Join(conceptsIntroducedByWeek[week], ", ")))
	}
	return strings.Join(parts, "; ")
}
