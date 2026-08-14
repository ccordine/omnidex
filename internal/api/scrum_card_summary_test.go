package api

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestDBScrumCardSummaryMapsExactBoardScalars(t *testing.T) {
	stored := queue.DBScrumCardSummary{
		ID: "card-1", Title: "Summary", Description: "bounded", Column: "review",
		ChecklistDone: 1, ChecklistTotal: 2, RefFileCount: 3, ChatCount: 4,
		TestCriteriaDone: 6, TestCriteriaTotal: 7, HasCardTicket: true,
		Tags: json.RawMessage(`["one","two"]`), FlowMetrics: json.RawMessage(`{"completion_status":"uncertain"}`),
		JobID: "42", PlayState: "running", QueueOrder: 8, BoardOrder: 9,
	}
	card, err := dbScrumCardSummaryToAPI(stored)
	if err != nil {
		t.Fatal(err)
	}
	if !card.Summary || card.ChatCount != 4 || card.ChecklistDone != 1 || card.ChecklistTotal != 2 ||
		card.TestCriteriaDone != 6 || card.TestCriteriaTotal != 7 || len(card.Chat) != 0 ||
		len(card.Tags) != 2 {
		t.Fatalf("mapped summary=%+v", card)
	}
}

func TestDBScrumCardSummaryRejectsMalformedRetainedObjects(t *testing.T) {
	for name, mutate := range map[string]func(*queue.DBScrumCardSummary){
		"tags":         func(card *queue.DBScrumCardSummary) { card.Tags = json.RawMessage(`{}`) },
		"flow_metrics": func(card *queue.DBScrumCardSummary) { card.FlowMetrics = json.RawMessage(`[]`) },
	} {
		t.Run(name, func(t *testing.T) {
			stored := queue.DBScrumCardSummary{ID: "bad", Tags: json.RawMessage(`[]`), FlowMetrics: json.RawMessage(`{}`)}
			mutate(&stored)
			if _, err := dbScrumCardSummaryToAPI(stored); err == nil {
				t.Fatal("expected malformed retained summary JSON to fail")
			}
		})
	}
}

func TestScrumCardListCallersUseOnlySummaryDecoder(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"scrum_card_pages.go", "scrum_flow_metrics_service.go"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		for _, forbidden := range []string{"dbScrumCardToAPI(", "scrumCardBoardSummary("} {
			if strings.Contains(source, forbidden) {
				t.Errorf("full-card list decoder %q remains in %s", forbidden, path)
			}
		}
		if !strings.Contains(source, "dbScrumCardSummaryToAPI(") {
			t.Errorf("summary list decoder is missing from %s", path)
		}
	}
}

func TestScrumCardSummaryReadsDoNotRecomputePersistedFlowMetrics(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"scrum_card_pages.go", "scrum_flow_metrics_service.go", "scrum_service.go", "scrum_realtime.go"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "refreshScrumFlowMetricsForBoard") {
			t.Errorf("read-time flow-metric recomputation remains in %s", path)
		}
	}
}
