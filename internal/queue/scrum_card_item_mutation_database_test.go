package queue

import (
	"encoding/json"
	"testing"
)

func TestPostgresChecklistMutationRefreshesCompletionMetricsAtomically(t *testing.T) {
	repository, _, ctx := scrumChannelOperationTestRepository(t)
	project, err := repository.CreateProject(ctx, "Checklist metrics", "/tmp/checklist-metrics", "")
	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(
		ctx, project.ID, "checklist-metrics-card", "Checklist metrics", "", "review", nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	card, err = repository.MutateScrumCardItem(ctx, ScrumCardItemMutation{
		ProjectID: project.ID, CardID: card.ID, ExpectedUpdatedAt: card.UpdatedAt,
		Collection: ScrumCardChecklist, Action: ScrumCardItemAdd, Text: "Required exact check",
	})
	if err != nil {
		t.Fatal(err)
	}
	var metrics ScrumFlowMetrics
	if err := json.Unmarshal(card.FlowMetrics, &metrics); err != nil {
		t.Fatal(err)
	}
	if metrics.IncompleteScore != 15 || len(metrics.Signals) != 1 ||
		metrics.Signals[0] != "checklist still incomplete in review/done" {
		t.Fatalf("incomplete checklist metrics=%+v", metrics)
	}
	items, err := decodeCanonicalScrumCardItems(card.Checklist)
	if err != nil || len(items) != 1 {
		t.Fatalf("checklist=%+v error=%v", items, err)
	}
	done := true
	card, err = repository.MutateScrumCardItem(ctx, ScrumCardItemMutation{
		ProjectID: project.ID, CardID: card.ID, ExpectedUpdatedAt: card.UpdatedAt,
		Collection: ScrumCardChecklist, Action: ScrumCardItemToggle, ItemID: items[0].ID, Done: &done,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(card.FlowMetrics, &metrics); err != nil {
		t.Fatal(err)
	}
	if metrics.IncompleteScore != 0 || len(metrics.Signals) != 0 {
		t.Fatalf("completed checklist metrics=%+v", metrics)
	}
}

func TestPostgresChecklistMetricsRefreshHasNoLiveFlowLedgerDependency(t *testing.T) {
	repository, pool, ctx := scrumChannelOperationTestRepository(t)
	project, err := repository.CreateProject(ctx, "Bounded checklist metrics", "/tmp/bounded-checklist-metrics", "")
	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(
		ctx, project.ID, "bounded-checklist-card", "Bounded checklist", "", "review", nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	var liveRelation bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('scrum_flow_events') IS NOT NULL`).Scan(&liveRelation); err != nil {
		t.Fatal(err)
	}
	if liveRelation {
		t.Fatal("retired live flow ledger still exists")
	}
	updated, err := repository.MutateScrumCardItem(ctx, ScrumCardItemMutation{
		ProjectID: project.ID, CardID: card.ID, ExpectedUpdatedAt: card.UpdatedAt,
		Collection: ScrumCardChecklist, Action: ScrumCardItemAdd, Text: "Bounded exact check",
	})
	if err != nil {
		t.Fatalf("checklist refresh failed without retired flow ledger: %v", err)
	}
	var metrics ScrumFlowMetrics
	if err := json.Unmarshal(updated.FlowMetrics, &metrics); err != nil {
		t.Fatal(err)
	}
	if metrics.IncompleteScore != 15 || metrics.RegressionCount != 0 {
		t.Fatalf("bounded checklist refresh reaggregated historical events: %+v", metrics)
	}
}
