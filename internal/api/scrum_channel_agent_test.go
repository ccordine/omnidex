package api

import (
	"encoding/json"
	"testing"
)

func TestScrumChannelPlayColumn(t *testing.T) {
	tests := []struct {
		current       string
		channelOrigin bool
		want          string
	}{
		{"review", true, "review"},
		{"review", false, "in_progress"},
		{"assigned", true, "in_progress"},
		{"ready", true, "in_progress"},
		{"in_progress", true, "in_progress"},
	}
	for _, tc := range tests {
		got := scrumChannelPlayColumn(tc.current, tc.channelOrigin)
		if got != tc.want {
			t.Fatalf("scrumChannelPlayColumn(%q, %v)=%q want %q", tc.current, tc.channelOrigin, got, tc.want)
		}
	}
}

func TestScrumChannelJobMetadata(t *testing.T) {
	raw := scrumChannelJobMetadata([]byte(`{"source":"omni-scrum"}`), "review")
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	if meta["scrum_channel_origin"] != true {
		t.Fatalf("metadata=%v", meta)
	}
	if meta["scrum_return_column"] != "review" {
		t.Fatalf("return column=%v", meta["scrum_return_column"])
	}
}

func TestApplyScrumReturnColumnReviewOnly(t *testing.T) {
	meta := json.RawMessage(`{"scrum_return_column":"review"}`)
	transition := applyScrumReturnColumn(scrumColumnForOutcome(ScrumOutcomeInProgress), ScrumOutcomeSuccess, meta)
	if transition.Column != "review" {
		t.Fatalf("success+review return=%q", transition.Column)
	}

	assignedMeta := json.RawMessage(`{"scrum_return_column":"assigned"}`)
	transition = applyScrumReturnColumn(scrumColumnForOutcome(ScrumOutcomeSuccess), ScrumOutcomeSuccess, assignedMeta)
	if transition.Column != "review" {
		t.Fatalf("assigned origin should keep default review, got %q", transition.Column)
	}
}
