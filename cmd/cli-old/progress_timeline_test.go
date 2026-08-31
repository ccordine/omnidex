package main

import "testing"

func TestParseStepEventPayload(t *testing.T) {
	raw := "time=2026-02-15T00:00:00Z event=coding_phase_changed phase=assembling detail=compile"
	payload := parseStepEventPayload(raw)
	if payload.Time != "2026-02-15T00:00:00Z" {
		t.Fatalf("unexpected time: %q", payload.Time)
	}
	if payload.EventType != "coding_phase_changed" {
		t.Fatalf("unexpected event type: %q", payload.EventType)
	}
	if payload.Message == "" {
		t.Fatal("expected message content")
	}
}

func TestSummarizeStepEvent(t *testing.T) {
	got := summarizeStepEvent(stepEventPayload{
		EventType: "coding_stage_started",
		Message:   "attempt=1 generated_blocks=4",
	})
	if got != "Running isolated staged validation" {
		t.Fatalf("unexpected summary: %q", got)
	}

	waiting := summarizeStepEvent(stepEventPayload{
		EventType: "objective_waiting_input",
		Message:   "reason=missing_authority",
	})
	if waiting == "" {
		t.Fatalf("expected waiting summary")
	}
}

func TestSummarizeProgressStream(t *testing.T) {
	kind, summary := summarizeProgressStream("stdout", "running test: go test ./...", 200)
	if kind != "Run" {
		t.Fatalf("kind=%q", kind)
	}
	if summary == "" {
		t.Fatal("expected summary")
	}

	kind, summary = summarizeProgressStream("stderr", "compiler exited with status 1", 200)
	if kind == "" || summary == "" {
		t.Fatalf("expected non-empty stderr summary")
	}
}
