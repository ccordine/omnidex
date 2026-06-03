package api

import (
	"encoding/json"
	"os"
	"strings"
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

func TestResolveScrumPlayOutcomeCompletedWithoutStatusMovesToSuccess(t *testing.T) {
	s := &Server{}
	job := model.JobDetails{
		Job: model.Job{
			Status:   model.JobStatusCompleted,
			Metadata: json.RawMessage(`{"source":"omni-scrum","execution_agent":"cursor","scrum_raw_play":true}`),
		},
		Steps: []model.Step{{
			Output: `{"agent":"cursor","type":"completed","message":"Cursor external implementation session completed"}`,
		}},
	}
	outcome, note := s.resolveScrumPlayOutcomeForCard(t.Context(), job, ScrumCard{})
	if outcome != ScrumOutcomeSuccess || note != "" {
		t.Fatalf("outcome=%q note=%q want success without scan note", outcome, note)
	}
	transition := scrumColumnForOutcome(outcome)
	if transition.Column != "review" || transition.PlayState != "" {
		t.Fatalf("transition=%+v want review", transition)
	}
}

func TestResolveScrumPlayOutcomeCompletedInProgressStatusIsProgrammatic(t *testing.T) {
	s := &Server{}
	job := model.JobDetails{
		Job: model.Job{
			Status:   model.JobStatusCompleted,
			Metadata: json.RawMessage(`{"source":"omni-scrum","execution_agent":"cursor","scrum_raw_play":true}`),
		},
		Steps: []model.Step{{Output: "SCRUM_STATUS: in_progress"}},
	}
	outcome, _ := s.resolveScrumPlayOutcomeForCard(t.Context(), job, ScrumCard{})
	if outcome != ScrumOutcomeInProgress {
		t.Fatalf("outcome=%q want in_progress", outcome)
	}
}

func TestResolveScrumPlayOutcomeDoesNotCallClassifier(t *testing.T) {
	source, err := os.ReadFile("scrum_play_outcome.go")
	if err != nil {
		t.Fatalf("read scrum_play_outcome.go: %v", err)
	}
	if strings.Contains(string(source), "classifyScrumAgentOutcome") {
		t.Fatal("scrum play outcome must be programmatic and must not call an LLM classifier")
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
