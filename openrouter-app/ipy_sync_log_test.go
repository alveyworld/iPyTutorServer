package main

import "testing"

func TestCollectIPyIntervuSyncPortionsComplete(t *testing.T) {
	assistant := "Thanks.\n\n```_ipyintervu\n{\"bugAssessmentPhase\": \"in_progress\"}\n```"
	complete, partial := collectIPyIntervuSyncPortions(assistant)
	if len(complete) != 1 {
		t.Fatalf("complete len = %d, want 1", len(complete))
	}
	if complete[0] != `{"bugAssessmentPhase": "in_progress"}` {
		t.Fatalf("complete[0] = %q", complete[0])
	}
	if partial != "" {
		t.Fatalf("partial = %q, want empty", partial)
	}
}

func TestCollectIPyIntervuSyncPortionsPartial(t *testing.T) {
	assistant := "Please choose one concept.\n```_ipyintervu\n{\"conceptualAssessmentPhase\":"
	complete, partial := collectIPyIntervuSyncPortions(assistant)
	if len(complete) != 0 {
		t.Fatalf("complete len = %d, want 0", len(complete))
	}
	if partial == "" {
		t.Fatal("expected partial sync body")
	}
}

func TestCollectIPyIntervuSyncPortionsMultipleComplete(t *testing.T) {
	assistant := "A\n\n```_ipyintervu\n{\"codeAssessmentPhase\": \"in_progress\"}\n```\n\nB\n\n```_ipyintervu\n{\"codeAssessmentPhase\": \"complete\", \"codeAssessmentBucket\": \"Competent\"}\n```"
	complete, partial := collectIPyIntervuSyncPortions(assistant)
	if len(complete) != 2 {
		t.Fatalf("complete len = %d, want 2", len(complete))
	}
	if partial != "" {
		t.Fatalf("partial = %q, want empty", partial)
	}
}
