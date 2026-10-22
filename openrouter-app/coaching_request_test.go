package main

import "testing"

func TestIsCoachingRequest(t *testing.T) {
	yes := []string{
		"switch to coach mode",
		"Switch to coach mode",
		"coaching",
		"Could I get some coaching on how I did?",
		"give me feedback on my assessment",
		"Thanks. Could I get some coaching on how I did? I'd especially like to understand why the Bug part was rated lower than the others.",
	}
	for _, msg := range yes {
		if !isCoachingRequest(msg) {
			t.Errorf("isCoachingRequest(%q) = false, want true", msg)
		}
	}
	no := []string{
		"```python\nprint(\"Thanks for the coaching session!\")\n```",
		"name = input(\"Client name: \")\nprint(\"Welcome to the coaching program, \" + name)",
		"print(\"Your coaching plan is ready\")",
		"I'd sort the clients by who needs coaching most, and then I'd compare each client's sessions against the plan they agreed to, one at a time, so that nobody is missed.",
		"I would check the schedule.",
	}
	for _, msg := range no {
		if isCoachingRequest(msg) {
			t.Errorf("isCoachingRequest(%q) = true, want false", msg)
		}
	}
}

func TestMidInterviewCoachingRequestIsStricter(t *testing.T) {
	if !isMidInterviewCoachingRequest("switch to coach mode") || !isMidInterviewCoachingRequest("Could I get some coaching?") {
		t.Error("a short request during the interview should count")
	}
	// Fine as a request after the results, but too long to be taken for one mid-interview.
	long := "Could I get some coaching on how I am doing so far please?"
	if !isCoachingRequest(long) {
		t.Fatal("test message should be a request after the results")
	}
	if isMidInterviewCoachingRequest(long) {
		t.Errorf("isMidInterviewCoachingRequest(%q) = true, want false", long)
	}
	for _, answer := range []string{
		"I would track coaching sessions per client and total the hours.",
		"I'd list coaching sessions per client.",
		"Coaching sessions go in a list.",
		"Each coaching client has a plan.",
	} {
		if isMidInterviewCoachingRequest(answer) {
			t.Errorf("isMidInterviewCoachingRequest(%q) = true: a short answer that mentions coaching was taken for a request", answer)
		}
	}
}
