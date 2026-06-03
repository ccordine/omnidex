package api

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestResolveScrumPlayOutcomeForCardUsesChannelSuccess(t *testing.T) {
	s := &Server{}
	job := model.JobDetails{
		Job: model.Job{
			Status:   model.JobStatusCompleted,
			Metadata: json.RawMessage(`{"source":"omni-scrum","execution_agent":"cursor","scrum_raw_play":true}`),
		},
		Steps: []model.Step{{
			Output: `{"agent":"cursor","type":"started","message":"Cursor external implementation session started"}
{"agent":"cursor","type":"completed","message":"Cursor external implementation session completed"}`,
		}},
	}
	card := ScrumCard{
		Column:    "in_progress",
		PlayState: scrumPlayRunning,
		Chat: []ScrumChatMessage{{
			Role:    "assistant",
			Content: "Implemented the fix.\nSCRUM_STATUS: success",
		}},
	}
	outcome, _ := s.resolveScrumPlayOutcomeForCard(t.Context(), job, card)
	if outcome != ScrumOutcomeSuccess {
		t.Fatalf("outcome=%q want success when SCRUM_STATUS is in synced channel", outcome)
	}
	transition := scrumColumnForOutcome(outcome)
	if transition.Column != "review" {
		t.Fatalf("transition=%+v want review", transition)
	}
}

func TestReconcileTerminalJobInProgressColumn(t *testing.T) {
	jobID := "99"
	card := ScrumCard{
		ID:        "c1",
		Column:    "in_progress",
		PlayState: "",
		JobID:     jobID,
	}
	if !scrumCardNeedsTerminalJobReconcile(card) {
		t.Fatal("expected in_progress card with job id to need reconcile")
	}
}
