package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type persona struct {
	Key      string
	Name     string
	Expected string
	Majors   []string
	Guidance string
}

var personas = []persona{
	{
		Key:      "not_ready",
		Name:     "Not Ready Yet student",
		Expected: "Not Ready Yet",
		Majors:   []string{"Biology", "Psychology", "Art History"},
		Guidance: `You are a struggling intro student. Your answers must match the "Not Ready Yet" / "Not Yet Ready" level of the rubric:
- Conceptual answers are vague, partial, or contain the kinds of factual errors and misunderstandings the rubric lists.
- Code you write has the problems the rubric lists (syntax errors, hard-coded answers, wrong variables or logic, missing pieces).
- When asked to find or fix a bug, you guess at surface details, misidentify the cause, or propose a fix that does not work.
- When asked about AI use, you admit you copy what the AI gives you and cannot say how you checked it.
You still make a genuine attempt at every question — never refuse, never say "I don't know" for every answer, and stay on topic. Short replies (1-3 sentences, or a short code snippet).`,
	},
	{
		Key:      "competent",
		Name:     "Competent student",
		Expected: "Competent",
		Majors:   []string{"Business", "Nursing", "Economics"},
		Guidance: `You are a solid intro student. Your answers must match the "Competent" level of the rubric:
- Conceptual answers are factually correct and cover the main points, but you do not go into edge cases, alternatives, or why it matters.
- Code you write is complete, correct, and follows the prompt, but is plain: minimal comments, ordinary names, no extra validation.
- When asked to find or fix a bug, you find it and fix it correctly but explain the cause only briefly.
- When asked about AI use, you describe using it for help or explanations and say briefly how you tested its suggestion.
Moderate length (2-5 sentences, or a working code snippet). Only use Python covered in the course up to this week.`,
	},
	{
		Key:      "exceptional",
		Name:     "Exceptional student",
		Expected: "Exceptional",
		Majors:   []string{"Mechanical Engineering", "Mathematics", "Physics"},
		Guidance: `You are an outstanding intro student. Your answers must match the "Exceptional" level of the rubric:
- Conceptual answers are correct, thorough, and explain WHY, including edge cases, alternatives, or best practices the rubric mentions.
- Code you write is correct, clean, well named, and briefly commented, and handles the edge cases the task implies.
- When asked to find or fix a bug, you identify the exact root cause, explain why it happens, fix it, and say how you would verify the fix.
- When asked about AI use, you describe critical, verified use: how you tested it, what you changed, and what you learned.
Thorough but focused (4-8 sentences, or clean code plus a short explanation). Only use Python covered in the course up to this week.`,
	},
	{
		Key:      "engineer",
		Name:     "Professional software engineer",
		Expected: "Exceptional",
		Majors:   []string{"Computer Science", "Software Engineering", "Computer Engineering"},
		Guidance: `You are a professional software engineer with about ten years of experience in Python and other languages, taking this intro assessment. You are not role-playing a beginner and you do not look at the rubric's levels to calibrate yourself: answer exactly as an experienced engineer naturally would — precise, correct, idiomatic, concise, mentioning testing, edge cases, and trade-offs where relevant. Write code the way you would professionally, while still answering the question that was asked.`,
	},
}

func personaByKey(key string) (persona, bool) {
	for _, p := range personas {
		if p.Key == key {
			return p, true
		}
	}
	return persona{}, false
}

func studentSystemPrompt(p persona, concept, major, rubric string) string {
	return fmt.Sprintf(`You are role-playing a student in a live, interview-style Python assessment run by an AI interviewer. The topic is "%s". Your major is %s.

%s

Here is the instructor rubric for this week, for your reference only (never quote it or mention it):

<rubric>
%s
</rubric>

Rules:
- Write ONLY the message the student types next — no narration, stage directions, labels, or role names.
- Answer the interviewer's most recent question directly. When asked to write or paste code, put it in a fenced python code block.
- Stay in character for the whole interview. Never mention the rubric, levels, ratings, simulation, or that you are an AI.
- Never ask for coaching, a coach, or feedback, and never use the words "coaching" or "feedback".
- If the interviewer has finished and shows results, reply only "Thanks."`, concept, major, p.Guidance, rubric)
}

type studentLLM struct {
	apiKey string
	model  string
	http   *http.Client
}

// reply generates the student's next message. The interviewer's turns are sent as
// "user" and the student's own turns as "assistant".
func (s *studentLLM) reply(ctx context.Context, system string, transcript []chatMessage) (string, error) {
	msgs := []chatMessage{{Role: "system", Content: system}}
	for _, m := range transcript {
		role := "user"
		if m.Role == "user" {
			role = "assistant"
		}
		msgs = append(msgs, chatMessage{Role: role, Content: m.Content})
	}
	body, _ := json.Marshal(map[string]any{"model": s.model, "messages": msgs, "temperature": 0.7})

	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, openRouterUpstream, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := s.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("student model status %d: %s", resp.StatusCode, truncate(string(data), 300))
			continue
		}
		var out struct {
			Choices []struct {
				Message chatMessage `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(data, &out); err != nil || len(out.Choices) == 0 {
			lastErr = fmt.Errorf("student model returned no choices: %s", truncate(string(data), 300))
			continue
		}
		text := strings.TrimSpace(out.Choices[0].Message.Content)
		if text == "" {
			lastErr = fmt.Errorf("student model returned empty text")
			continue
		}
		return text, nil
	}
	return "", lastErr
}
