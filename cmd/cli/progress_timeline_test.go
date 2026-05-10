package main

import "testing"

func TestParseStepEventPayload(t *testing.T) {
	raw := "time=2026-02-15T00:00:00Z event=web_search_begin mode=auto time_sensitive=true"
	payload := parseStepEventPayload(raw)
	if payload.Time != "2026-02-15T00:00:00Z" {
		t.Fatalf("unexpected time: %q", payload.Time)
	}
	if payload.EventType != "web_search_begin" {
		t.Fatalf("unexpected event type: %q", payload.EventType)
	}
	if payload.Message == "" {
		t.Fatal("expected message content")
	}
}

func TestSummarizeStepEvent(t *testing.T) {
	got := summarizeStepEvent(stepEventPayload{
		EventType: "tooling_begin",
		Message:   "autonomy=on",
	})
	if got != "Inspecting environment and required tools" {
		t.Fatalf("unexpected summary: %q", got)
	}

	waiting := summarizeStepEvent(stepEventPayload{
		EventType: "tooling_waiting_input",
		Message:   "missing_tools=2",
	})
	if waiting == "" {
		t.Fatalf("expected waiting summary")
	}
}

func TestSummarizeProgressStream(t *testing.T) {
	kind, summary := summarizeProgressStream("stdout", "web search query: best fedora package manager tips", 200)
	if kind != "Explore" {
		t.Fatalf("kind=%q", kind)
	}
	if summary == "" {
		t.Fatal("expected summary")
	}

	kind, summary = summarizeProgressStream("stderr", "tool check: npm missing", 200)
	if kind == "" || summary == "" {
		t.Fatalf("expected non-empty stderr summary")
	}
}
