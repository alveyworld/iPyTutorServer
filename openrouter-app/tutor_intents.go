package main

import (
	"regexp"
	"strconv"
	"strings"
)

type intentKind string

const (
	// intentNone is anything else: an answer to a practice problem or a question.
	intentNone       intentKind = ""
	intentLesson     intentKind = "lesson"
	intentNextLesson intentKind = "next_lesson"
	intentPractice   intentKind = "practice"
	intentAnother    intentKind = "another"
	intentHint       intentKind = "hint"
	intentSolution   intentKind = "solution"
	intentGuide      intentKind = "guide"
	intentOverview   intentKind = "overview"
	intentWeek       intentKind = "week"
	intentWeekMenu   intentKind = "weeks"
	intentSetMajor   intentKind = "major"
	intentHelp       intentKind = "help"
)

const (
	difficultyStandard = ""
	difficultyEasier   = "easier"
	difficultyHarder   = "harder"
)

type intent struct {
	Kind intentKind
	// Practice is the practice format for intentPractice; "" means the last one used.
	Practice   string
	Difficulty string
	// Week is the week to study, for intentWeek.
	Week int
	// Lesson is a lesson number (from 1) the student asked for, with intentWeek or
	// intentLesson.
	Lesson int
	// Teach starts the week's first lesson after switching weeks ("teach me for loops").
	Teach bool
	// Topic marks a week chosen by naming its topic ("study lists") rather than its number.
	Topic bool
	// MoveOn marks a bare "next" or "continue", whose meaning depends on the activity.
	MoveOn bool
}

// parseAction reads the action a UI button sends ("practice:code", "week:5", "lesson:3").
func parseAction(action string) (intent, bool) {
	name, arg, _ := strings.Cut(strings.TrimSpace(action), ":")
	switch intentKind(name) {
	case intentNextLesson, intentHint, intentSolution, intentGuide, intentOverview, intentWeekMenu, intentHelp:
		return intent{Kind: intentKind(name)}, true
	case intentLesson:
		if arg == "" {
			return intent{Kind: intentLesson}, true
		}
		if n, err := strconv.Atoi(arg); err == nil && n > 0 {
			return intent{Kind: intentLesson, Lesson: n}, true
		}
	case intentAnother:
		return intent{Kind: intentAnother, Difficulty: difficultyWord(arg)}, true
	case intentPractice:
		switch arg {
		case "", kindConcept, kindCode, kindBug:
			return intent{Kind: intentPractice, Practice: arg}, true
		}
	case intentWeek:
		if n, err := strconv.Atoi(arg); err == nil && weekInfo(n) != nil {
			return intent{Kind: intentWeek, Week: n}, true
		}
	}
	return intent{}, false
}

// Typed commands are only recognized in short messages, so an answer that happens to
// contain "hint" or "solution" is still treated as an answer.
const maxCommandWords = 8

var (
	commandTrim       = regexp.MustCompile(`^[\s"'*_]+|[\s"'*_.!?]+$`)
	politePrefix      = regexp.MustCompile(`^(?:(?:ok(?:ay)?|hey|hi|please|so|um|alright|great|thanks|thank you)[,!. ]+)+`)
	politeSuffix      = regexp.MustCompile(`[,. ]+(?:please|thanks|thank you|now)$`)
	requestPrefix     = regexp.MustCompile(`^(?:(?:can|could|may) (?:i|you|we) (?:get|have|see|do|try|give me)|give me|show me|i(?:'d| would) like|i want|i need|let'?s (?:do|try|see|get)?|let me (?:see|try|have)|can you|could you|time for|start|begin|do|try)\s+`)
	weekCommand       = regexp.MustCompile(`^(?:(?:switch|change|move|go|jump|skip) (?:to |over to |ahead to |back to )?|study |studying |start |open |back to )?week (\d{1,2})(?:,? (?:and )?(?:lesson|l) ?(\d{1,2}))?$`)
	lessonOfWeek      = regexp.MustCompile(`^(?:(?:re)?teach (?:me )?|study |open |go to )?lesson (\d{1,2}) (?:of|in|from|for) week (\d{1,2})$`)
	lessonNumber      = regexp.MustCompile(`^(?:(?:re)?teach (?:me )?|repeat |redo |go back to |back to |go to |open |study |skip to |jump to )?lesson (\d{1,2})$`)
	topicCommand      = regexp.MustCompile(`^(?:to )?(study|learn|learn about|teach me|teach me about|i'?m studying|i'?m learning|switch to|go to|move on to|open|let'?s study|let'?s learn)\s+(?:about\s+)?(?:the\s+)?(.+)$`)
	majorCommand      = regexp.MustCompile(`^(?:my major is|i'?m majoring in|i am majoring in|i major in|major:?|set my major to|change my major to)\s+\S|^i(?:'m| am) an? .+ (?:major|student)$`)
	anotherCommand    = regexp.MustCompile(`^(?:another(?: one)?|one more|more|again|next (?:one|problem|question)|(?:a )?new (?:one|problem|question)|(?:an? )?(easier|harder|simpler|tougher|more difficult|more challenging) (?:one|problem|question))$`)
	practiceCommand   = regexp.MustCompile(`^((?:a|an|one|another|some|more|new|next|the next|one more)\s+)?(?:(easier|harder|simpler|tougher|more difficult|more challenging)\s+)?(concept(?:ual)?(?: question)?|decomposition(?: problem| question| practice| exercise)?|code(?: problem)?|coding(?: problem| question| exercise| challenge)?|programming (?:problem|exercise)|bug(?: hunt| hunting| problem)?|debugging(?: problem| practice| exercise)?|practice(?: problem| question)?|problem|exercise|quiz(?: me)?)s?$`)
	hintCommand       = regexp.MustCompile(`^(?:a |another |one more |small |little )*hints?$|^(?:help|i'?m stuck|stuck|nudge)$`)
	solutionCommand   = regexp.MustCompile(`^(?:the |a )?(?:solution|answer|worked solution|full solution|model answer)$|^(?:i give up|give up|reveal(?: it| the answer| the solution)?|what'?s the answer)$`)
	lessonCommand     = regexp.MustCompile(`^(?:teach me|lessons?|teach|start (?:the )?lessons?|a lesson|this week'?s lessons?|continue lessons?|continue (?:the )?lesson)$`)
	nextLessonCommand = regexp.MustCompile(`^(?:the )?next lesson$`)
	// moveOnCommand is "next" or "continue": the next lesson during lessons, otherwise
	// another problem of the same kind.
	moveOnCommand   = regexp.MustCompile(`^(?:next|continue|keep going|go on)$`)
	guideCommand    = regexp.MustCompile(`^(?:the )?interview(?: guide| tips| prep| help| format| info)?$|^(?:how does the interview work|what'?s (?:on|in) the interview|interview tips)$`)
	overviewCommand = regexp.MustCompile(`^(?:the |this week'?s |week |weekly )?(?:plan|study plan|overview|outline|lesson list|list of lessons|topics)$|^what'?s (?:in|on) this week$`)
	weekMenuCommand = regexp.MustCompile(`^(?:change|switch|pick|choose|another|a different|other) week$|^(?:weeks|week list|list weeks|all weeks)$`)
	helpCommand     = regexp.MustCompile(`^(?:help me|what can you do|commands|options|menu|how does this work|how do i use this)$`)
)

// commandText normalizes a short message for command matching, or returns "" for a
// message that is too long or contains code to be a command.
func commandText(msg string) string {
	if strings.Contains(msg, "```") || strings.Contains(msg, "\n") {
		return ""
	}
	s := strings.ToLower(strings.TrimSpace(msg))
	s = strings.NewReplacer("’", "'", "‘", "'").Replace(s)
	s = commandTrim.ReplaceAllString(s, "")
	if len(strings.Fields(s)) > maxCommandWords {
		return ""
	}
	s = politePrefix.ReplaceAllString(s, "")
	s = politeSuffix.ReplaceAllString(s, "")
	s = requestPrefix.ReplaceAllString(s, "")
	return strings.Join(strings.Fields(s), " ")
}

// detectIntent recognizes a typed command. practicing reports an open practice problem,
// where "help" asks for a hint rather than for the list of commands.
func detectIntent(msg string, practicing bool) intent {
	s := commandText(msg)
	if s == "" {
		return intent{}
	}
	if m := weekCommand.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		lesson, _ := strconv.Atoi(m[2])
		if weekInfo(n) != nil {
			return intent{Kind: intentWeek, Week: n, Lesson: lesson}
		}
	}
	if m := lessonOfWeek.FindStringSubmatch(s); m != nil {
		lesson, _ := strconv.Atoi(m[1])
		n, _ := strconv.Atoi(m[2])
		if weekInfo(n) != nil {
			return intent{Kind: intentWeek, Week: n, Lesson: lesson}
		}
	}
	if m := lessonNumber.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		return intent{Kind: intentLesson, Lesson: n}
	}
	if majorCommand.MatchString(s) {
		return intent{Kind: intentSetMajor}
	}
	switch {
	case hintCommand.MatchString(s):
		if s == "help" && !practicing {
			return intent{Kind: intentHelp}
		}
		return intent{Kind: intentHint}
	case solutionCommand.MatchString(s):
		return intent{Kind: intentSolution}
	case nextLessonCommand.MatchString(s):
		return intent{Kind: intentNextLesson}
	case lessonCommand.MatchString(s):
		return intent{Kind: intentLesson}
	case guideCommand.MatchString(s):
		return intent{Kind: intentGuide}
	case overviewCommand.MatchString(s):
		return intent{Kind: intentOverview}
	case weekMenuCommand.MatchString(s):
		return intent{Kind: intentWeekMenu}
	case helpCommand.MatchString(s):
		return intent{Kind: intentHelp}
	}
	if moveOnCommand.MatchString(s) {
		return intent{Kind: intentNextLesson, MoveOn: true}
	}
	if m := anotherCommand.FindStringSubmatch(s); m != nil {
		return intent{Kind: intentAnother, Difficulty: difficultyWord(m[1])}
	}
	if m := practiceCommand.FindStringSubmatch(s); m != nil {
		return practiceIntent(m[1], difficultyWord(m[2]), m[3])
	}
	if m := topicCommand.FindStringSubmatch(s); m != nil {
		if sel, ok := matchWeekSelection(m[2]); ok {
			return intent{Kind: intentWeek, Week: sel.CurrentWeekNumber, Topic: true, Teach: strings.HasPrefix(m[1], "teach")}
		}
	}
	return intent{}
}

func difficultyWord(word string) string {
	switch word {
	case "easier", "simpler":
		return difficultyEasier
	case "harder", "tougher", "more difficult", "more challenging":
		return difficultyHarder
	}
	return difficultyStandard
}

func practiceIntent(prefix, difficulty, format string) intent {
	kind := ""
	switch {
	case strings.HasPrefix(format, "concept"), strings.HasPrefix(format, "decomposition"):
		kind = kindConcept
	case strings.HasPrefix(format, "cod"), strings.HasPrefix(format, "programming"):
		kind = kindCode
	case strings.HasPrefix(format, "bug"), strings.HasPrefix(format, "debug"):
		kind = kindBug
	}
	// "another problem" or "a harder exercise" repeats the last format.
	switch strings.TrimSpace(prefix) {
	case "another", "more", "new", "next", "the next", "one more":
		if kind == "" {
			return intent{Kind: intentAnother, Difficulty: difficulty}
		}
	}
	return intent{Kind: intentPractice, Practice: kind, Difficulty: difficulty}
}
