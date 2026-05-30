package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

func TestIsScrumRegressionToAssigned(t *testing.T) {
	if !isScrumRegressionToAssigned("review", "assigned") {
		t.Fatal("review -> assigned should be regression")
	}
	if isScrumRegressionToAssigned("ready", "assigned") {
		t.Fatal("ready -> assigned should not count")
	}
}

func TestComputeScrumFlowMetricsIncomplete(t *testing.T) {
	card := ScrumCard{
		ID:     "c1",
		Column: "assigned",
		Chat: []ScrumChatMessage{
			{Role: "user", Content: "still working"},
		},
		Checklist: []ScrumChecklistItem{{ID: "1", Text: "task", Done: false}},
	}
	events := []queue.ScrumFlowEvent{
		{EventType: scrumFlowEventColumnMove, FromColumn: "review", ToColumn: "assigned", CreatedAt: time.Now()},
		{EventType: scrumFlowEventColumnMove, FromColumn: "in_progress", ToColumn: "assigned", CreatedAt: time.Now()},
		{EventType: scrumFlowEventPlayStarted, CreatedAt: time.Now()},
		{EventType: scrumFlowEventPlayStarted, CreatedAt: time.Now()},
		{EventType: scrumFlowEventPlayStarted, CreatedAt: time.Now()},
		{
			EventType: scrumFlowEventPlayFinished,
			Payload:   []byte(`{"outcome":"failed"}`),
			CreatedAt: time.Now(),
		},
	}
	metrics := computeScrumFlowMetrics(card, events)
	if metrics.AssignedReturns < 2 {
		t.Fatalf("assigned_returns=%d", metrics.AssignedReturns)
	}
	if metrics.CompletionStatus != "likely_incomplete" {
		t.Fatalf("status=%q score=%d", metrics.CompletionStatus, metrics.IncompleteScore)
	}
	if len(metrics.Signals) == 0 {
		t.Fatal("expected signals")
	}
}

func TestSummarizeScrumFlowMetrics(t *testing.T) {
	cards := []ScrumCard{
		{FlowMetrics: mustJSON(ScrumFlowMetrics{CompletionStatus: "likely_incomplete", AssignedReturns: 1})},
		{FlowMetrics: mustJSON(ScrumFlowMetrics{CompletionStatus: "likely_complete"})},
	}
	summary := summarizeScrumFlowMetrics(cards)
	if summary.LikelyIncomplete != 1 || summary.LikelyComplete != 1 {
		t.Fatalf("summary=%+v", summary)
	}
}

func mustJSON(v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}
