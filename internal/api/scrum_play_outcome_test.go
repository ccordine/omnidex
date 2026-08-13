package api

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestResolveScrumPlayOutcomeForCardIgnoresChannelStatusText(t *testing.T) {
	s := &Server{}
	job := model.JobDetails{
		Job: model.Job{
			Status:   model.JobStatusCompleted,
			Metadata: json.RawMessage(`{"source":"omni-scrum","agent_config":{"agent_system":"cursor"},"scrum_raw_play":true}`),
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
	outcome, err := s.resolveScrumPlayOutcomeForCard(t.Context(), job, card)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != ScrumOutcomeSuccess {
		t.Fatalf("outcome=%q want typed completed lifecycle", outcome)
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
			Metadata: json.RawMessage(`{"source":"omni-scrum","agent_config":{"agent_system":"cursor"},"scrum_raw_play":true}`),
		},
		Steps: []model.Step{{
			Output: `{"agent":"cursor","type":"completed","message":"Cursor external implementation session completed"}`,
		}},
	}
	outcome, err := s.resolveScrumPlayOutcomeForCard(t.Context(), job, ScrumCard{})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != ScrumOutcomeSuccess {
		t.Fatalf("outcome=%q want success", outcome)
	}
	transition := scrumColumnForOutcome(outcome)
	if transition.Column != "review" || transition.PlayState != "" {
		t.Fatalf("transition=%+v want review", transition)
	}
}

func TestResolveScrumPlayOutcomeCompletedIgnoresInProgressProse(t *testing.T) {
	s := &Server{}
	job := model.JobDetails{
		Job: model.Job{
			Status:   model.JobStatusCompleted,
			Metadata: json.RawMessage(`{"source":"omni-scrum","agent_config":{"agent_system":"cursor"},"scrum_raw_play":true}`),
		},
		Steps: []model.Step{{Output: "SCRUM_STATUS: in_progress"}},
	}
	outcome, err := s.resolveScrumPlayOutcomeForCard(t.Context(), job, ScrumCard{})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != ScrumOutcomeSuccess {
		t.Fatalf("outcome=%q want typed completed lifecycle", outcome)
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
