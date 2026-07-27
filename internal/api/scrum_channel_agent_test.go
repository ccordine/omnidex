package api

import (
	"encoding/json"
	"testing"
)

func TestMoveScrumCardToInProgress(t *testing.T) {
	card := moveScrumCardToInProgress(ScrumCard{Column: "backlog", PlayState: scrumPlayQueued, QueueOrder: 3})
	if card.Column != "in_progress" || card.PlayState != "" || card.QueueOrder != 0 {
		t.Fatalf("card=%+v", card)
	}
}

func TestScrumChannelJobMetadata(t *testing.T) {
	raw, err := scrumChannelJobMetadata([]byte(`{"source":"omni-scrum"}`), "review", "Fix routing only")
	if err != nil {
		t.Fatalf("scrumChannelJobMetadata: %v", err)
	}
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
	directives, ok := meta["v3_authority_directives"].([]any)
	if !ok || len(directives) != 1 || directives[0] != "Fix routing only" {
		t.Fatalf("authority directives=%#v", meta["v3_authority_directives"])
	}
	if _, exists := meta["scrum_current_user_instruction"]; exists {
		t.Fatalf("removed current-instruction compatibility key survived: %v", meta)
	}
}

func TestScrumChannelJobMetadataRejectsInvalidJSON(t *testing.T) {
	if _, err := scrumChannelJobMetadata([]byte(`{"source":`), "review", "Fix routing"); err == nil {
		t.Fatal("invalid job metadata must fail instead of returning the original payload")
	}
}

func TestScrumChannelJobMetadataRejectsMissingCurrentDirective(t *testing.T) {
	if _, err := scrumChannelJobMetadata([]byte(`{"source":"omni-scrum"}`), "review", "  "); err == nil {
		t.Fatal("channel metadata must not synthesize an instruction from history")
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
