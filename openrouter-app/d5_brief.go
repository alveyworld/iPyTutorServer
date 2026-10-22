package main

import (
	"regexp"
	"strings"
)

// d5Brief is the Evaluator's labelled-line output (design §5), parsed tolerantly.
type d5Brief struct {
	Mode          string
	AnswerIndex   int
	Quality       string
	Clarification bool
	Evidence      []string
	Covered       []string
	Gaps          []string
	Next          string
	Fallback      string
	Avoid         []string
	Recommend     string // "continue" or "close"
	Levels        []gradeLabel
	Issues        []string
}

var labelledLinePrefix = regexp.MustCompile(`^[\s>*_-]*`)

// splitLabelledLine splits "LABEL: value" (allowing markdown bold, bullets and "- "
// prefixes) into an upper-case label and its value.
func splitLabelledLine(line string) (string, string, bool) {
	line = labelledLinePrefix.ReplaceAllString(line, "")
	idx := strings.Index(line, ":")
	if idx <= 0 {
		return "", "", false
	}
	label := strings.ToUpper(strings.Trim(strings.TrimSpace(line[:idx]), "*_`"))
	if strings.ContainsAny(label, " \t") && label != "AI USE" {
		return "", "", false
	}
	value := strings.TrimSpace(strings.Trim(strings.TrimSpace(line[idx+1:]), "*_`"))
	return label, value, true
}

func splitList(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ";") {
		item = strings.TrimSpace(item)
		if item == "" || strings.EqualFold(item, "none") || strings.EqualFold(item, "n/a") {
			continue
		}
		out = append(out, item)
	}
	return out
}

// firstWord returns the first word of an enumerated value, so "close (enough evidence)"
// reads as "close".
func firstWord(value string) string {
	fields := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return r == ' ' || r == '(' || r == ',' || r == '.' || r == '|'
	})
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// parseLevels reads "dim=level; dim=level" for the given mode, keeping only the mode's
// dimensions and valid levels.
func parseLevels(value, mode string, answerIndex int) []gradeLabel {
	var out []gradeLabel
	for _, pair := range strings.FieldsFunc(value, func(r rune) bool { return r == ';' || r == ',' }) {
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			k, v, ok = strings.Cut(pair, ":")
		}
		if !ok {
			continue
		}
		dim := strings.ToLower(strings.TrimSpace(strings.Trim(strings.TrimSpace(k), "*`")))
		dim = strings.NewReplacer(" ", "_", "-", "_").Replace(dim)
		level := normalizeLevel(v)
		if level == "" || !dimensionInMode(mode, dim) {
			continue
		}
		out = append(out, gradeLabel{Dimension: dim, Level: level, AnswerIndex: answerIndex})
	}
	return out
}

// parseBrief parses an Evaluator brief. ok is false when the brief has neither NEXT nor
// FALLBACK, in which case the caller keeps the previous brief.
func parseBrief(text, mode string, answerIndex int) (d5Brief, bool) {
	b := d5Brief{Mode: mode, AnswerIndex: answerIndex, Recommend: "continue"}
	for _, line := range strings.Split(text, "\n") {
		label, value, ok := splitLabelledLine(line)
		if !ok {
			continue
		}
		switch label {
		case "QUALITY":
			switch q := firstWord(value); q {
			case "vague", "partial", "solid", "strong":
				b.Quality = q
			}
		case "CLARIFICATION":
			b.Clarification = firstWord(value) == "yes"
		case "EVIDENCE":
			b.Evidence = splitList(value)
		case "COVERED":
			b.Covered = splitList(value)
		case "GAPS":
			b.Gaps = splitList(value)
		case "NEXT":
			b.Next = value
		case "FALLBACK":
			b.Fallback = value
		case "AVOID":
			b.Avoid = splitList(value)
		case "RECOMMEND":
			if firstWord(value) == "close" {
				b.Recommend = "close"
			}
		case "LEVELS":
			b.Levels = parseLevels(value, mode, answerIndex)
		case "ISSUES":
			b.Issues = splitList(value)
		}
	}
	return b, b.Next != "" || b.Fallback != ""
}

// parseTargetLevel reads a levels-only reply: one level word, labelled on the targeted
// dimension. A "dimension=level" reply is accepted too.
func parseTargetLevel(text, mode, target string, answerIndex int) []gradeLabel {
	if target == dimCorrectness {
		switch firstWord(strings.Trim(strings.TrimSpace(text), "*`\"")) {
		case "works", "working", "works.", "correct", "yes", "pass":
			return []gradeLabel{{Dimension: dimCorrectness, Level: levelExceptional, AnswerIndex: answerIndex}}
		case "broken", "fails", "incorrect", "no", "fail", "wrong":
			return []gradeLabel{{Dimension: dimCorrectness, Level: levelNotReady, AnswerIndex: answerIndex}}
		}
	}
	if labels := parseLevelsLine(text, mode, answerIndex); len(labels) > 0 {
		return labels
	}
	for _, line := range strings.Split(text, "\n") {
		level := normalizeLevel(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "LEVEL:")))
		if level != "" && dimensionInMode(mode, target) {
			return []gradeLabel{{Dimension: target, Level: level, AnswerIndex: answerIndex}}
		}
	}
	return nil
}

// parseLevelsLine finds the LEVELS line in a levels-only reply. The model often drops the
// "LEVELS:" prefix and answers just "conceptual=competent", so a bare pairs line counts too.
func parseLevelsLine(text, mode string, answerIndex int) []gradeLabel {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if label, value, ok := splitLabelledLine(line); ok && label == "LEVELS" {
			return parseLevels(value, mode, answerIndex)
		}
	}
	for _, line := range lines {
		if labels := parseLevels(strings.Trim(strings.TrimSpace(line), "`*"), mode, answerIndex); len(labels) > 0 {
			return labels
		}
	}
	return nil
}

var defectLinePattern = regexp.MustCompile(`(?im)^[ \t>*_-]*DEFECT[*_]*[ \t]*:[ \t]*(.*)$`)

// splitDefectLine removes the hidden "DEFECT: …" line from a Bug opening, wherever it
// is (the prompt puts it first so the model plans the bug before the code), and returns
// the opening without it plus the defect ("" when the line is missing).
func splitDefectLine(opening string) (string, string) {
	loc := defectLinePattern.FindStringSubmatchIndex(opening)
	if loc == nil {
		return strings.TrimSpace(opening), ""
	}
	defect := strings.TrimSpace(strings.Trim(opening[loc[2]:loc[3]], "*_`"))
	rest := strings.TrimSpace(opening[:loc[0]]) + "\n" + strings.TrimSpace(opening[loc[1]:])
	return strings.TrimSpace(rest), defect
}

// defectSelfAdmitPattern marks a DEFECT line where the model admits the snippet has no
// real bug or argues with itself ("…which works—actually the bug is…").
var defectSelfAdmitPattern = regexp.MustCompile(`(?i)\bactually\b|\bwait\b|no (?:real )?defect|no bug|nothing (?:is )?wrong|is (?:actually )?correct|works (?:fine|correctly)|that's fine|\bhmm\b|let me|instead,? the|the real (?:bug|defect|issue)|but here`)

// d5DefectLooksBad reports a missing, self-admitting or rambling DEFECT line.
func d5DefectLooksBad(defect string) bool {
	return defect == "" || defectSelfAdmitPattern.MatchString(defect) || len(defect) > 300
}

// parseCompanyLine splits the first opening's "COMPANY: name | description" line from
// the rest of the reply. ok is false when the line is missing.
func parseCompanyLine(reply string) (name, domain, rest string, ok bool) {
	lines := strings.Split(strings.TrimSpace(reply), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		label, value, found := splitLabelledLine(line)
		if !found || label != "COMPANY" {
			return "", "", strings.TrimSpace(reply), false
		}
		name, domain, _ = strings.Cut(value, "|")
		if !strings.Contains(value, "|") {
			name, domain, _ = strings.Cut(value, " - ")
		}
		name = strings.TrimSpace(strings.Trim(name, "*`\""))
		domain = strings.TrimSpace(strings.TrimRight(strings.Trim(domain, "*`\""), "."))
		rest = strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
		return name, domain, rest, name != ""
	}
	return "", "", "", false
}
