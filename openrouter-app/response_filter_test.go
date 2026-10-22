package main

import "testing"

func TestStripIPyIntervuTail(t *testing.T) {
	raw := "Hi, I'm Alex at ChemSolve Labs.\n\n```_ipyintervu\n{\n  \"businessDomain\": {\"companyName\": \"ChemSolve Labs\", \"domain\": \"chemistry lab simulation software\"}\n}\n```"
	got := stripIPyIntervuTail(raw)
	want := "Hi, I'm Alex at ChemSolve Labs."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestClientVisibleAssistantContentHoldsPartialFence(t *testing.T) {
	raw := "Hello\n\n```_ipyintervu\n{\"businessDomain\":"
	got := clientVisibleAssistantContent(raw)
	want := "Hello"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestClientVisibleAssistantContentHoldsPartialIPyFence(t *testing.T) {
	raw := "Understood.\n\nI think we have a solid picture.\n\n```_ipy"
	got := clientVisibleAssistantContent(raw)
	want := "Understood.\n\nI think we have a solid picture."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestClientVisibleAssistantContentHoldsBareUnderscoreFence(t *testing.T) {
	raw := "Please choose one of these key concepts for us to assess today.\n```_"
	got := clientVisibleAssistantContent(raw)
	want := "Please choose one of these key concepts for us to assess today."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestClientVisibleAssistantContentHoldsBareOpeningFence(t *testing.T) {
	raw := "Thanks - I have your major as math.\n\n```"
	got := clientVisibleAssistantContent(raw)
	want := "Thanks - I have your major as math."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestClientVisibleAssistantContentPreservesPythonFence(t *testing.T) {
	raw := "Try this:\n\n```python\nprint('hi')\n```"
	got := clientVisibleAssistantContent(raw)
	if got != raw {
		t.Fatalf("got %q, want python block preserved", got)
	}
}

func TestStripIPyIntervuTailShortFenceTag(t *testing.T) {
	raw := "Summary text.\n\n```_ipy\n{\"conceptualAssessmentBucket\": \"Competent\"}\n```"
	got := stripIPyIntervuTail(raw)
	want := "Summary text."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestClientVisibleAssistantContentStripsStrayFenceBackticks(t *testing.T) {
	raw := "What would you identify as the input?\n\n``"
	got := clientVisibleAssistantContent(raw)
	want := "What would you identify as the input?"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
