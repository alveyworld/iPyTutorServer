package main

import (
	"encoding/json"
	"regexp"
	"strings"
)

var ipyintervuFenceStripPattern = regexp.MustCompile("(?is)```(?:json)?\\s*_ipy(?:intervu)?\\s*\\n.*?\\n```")

func stripTrailingPartialIPyFence(content string) string {
	content = stripStraySyncFenceTail(content)
	if strings.Count(content, "```") == 0 {
		return content
	}
	if strings.Count(content, "```")%2 == 0 {
		return content
	}
	open := strings.LastIndex(content, "```")
	after := strings.TrimSpace(content[open+3:])
	if after == "" ||
		strings.HasPrefix(after, "json") ||
		strings.HasPrefix(after, "_") ||
		strings.HasPrefix(strings.ToLower(after), "_ipy") {
		return strings.TrimRight(content[:open], " \t\n\r")
	}
	return content
}

func stripStraySyncFenceTail(content string) string {
	content = strings.TrimRight(content, " \t\n\r")
	for {
		i := len(content) - 1
		for i >= 0 && content[i] == '`' {
			i--
		}
		trailing := content[i+1:]
		if trailing == "" || len(trailing) >= 3 {
			break
		}
		content = strings.TrimRight(content[:i+1], " \t\n\r")
	}
	return content
}

// stripIPyIntervuTail removes complete _ipyintervu sync blocks from assistant text.
func stripIPyIntervuTail(content string) string {
	s := ipyintervuFenceStripPattern.ReplaceAllString(content, "")
	return strings.TrimRight(s, " \t\n\r")
}

// clientVisibleAssistantContent is safe to display (complete blocks removed, partial fence held back).
func clientVisibleAssistantContent(accumulated string) string {
	return stripTrailingPartialIPyFence(stripIPyIntervuTail(accumulated))
}

func replaceAssistantContent(body []byte, content string) ([]byte, error) {
	var completion openRouterCompletion
	if err := json.Unmarshal(body, &completion); err != nil {
		return body, err
	}
	if len(completion.Choices) == 0 {
		return body, nil
	}
	completion.Choices[0].Message.Content = content
	return json.Marshal(completion)
}
