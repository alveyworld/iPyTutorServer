package main

import (
	"log"
	"regexp"
	"strings"
)

var (
	replyOpenTag  = regexp.MustCompile(`(?i)<\s*reply\s*>`)
	replyCloseTag = regexp.MustCompile(`(?i)<\s*/\s*reply\s*>`)
)

// studentReply returns the text between the <reply> tags that live replies are asked to
// use (replyFormat), dropping any notes the model wrote around them. It takes the last
// opening tag, since notes may mention the tag, and the text after it up to the closing
// tag, or to the end when the reply was cut off. tagged is false, and the whole reply is
// returned, when there is no tagged text.
func studentReply(raw string) (text string, tagged bool) {
	opens := replyOpenTag.FindAllStringIndex(raw, -1)
	if len(opens) > 0 {
		body := raw[opens[len(opens)-1][1]:]
		if end := replyCloseTag.FindStringIndex(body); end != nil {
			body = body[:end[0]]
		}
		if body = strings.TrimSpace(body); body != "" {
			return body, true
		}
	}
	untagged := replyCloseTag.ReplaceAllString(replyOpenTag.ReplaceAllString(raw, ""), "")
	return strings.TrimSpace(untagged), false
}

// trimToFirstHeading drops anything before a lesson's first Markdown heading: a lesson
// starts with its "## Title", so text before it is the model's notes.
func trimToFirstHeading(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if headingLevel(strings.TrimSpace(line)) > 0 {
			return strings.TrimSpace(strings.Join(lines[i:], "\n"))
		}
	}
	return text
}

// cleanReply returns the part of a live reply the student sees. A reply without the tags
// is logged, and a lesson's is cut to start at its first heading.
func cleanReply(sessionID string, spec callSpec, raw string) string {
	text, tagged := studentReply(raw)
	if !tagged && text != "" {
		log.Printf("[tutor] reply_untagged call=%s session=%s", spec.Name, shortID(sessionID))
		if spec.Name == callLesson.Name {
			text = trimToFirstHeading(text)
		}
	}
	return text
}
