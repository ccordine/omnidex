package api

import (
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestNewProjectPlanningDraftsCreatesTypedBatch(t *testing.T) {
	drafts, batchID, err := newProjectPlanningDrafts(42, []ProjectPlanningCardDraft{
		{Title: "Add auth", Description: "Use sessions", Column: "backlog", Checklist: []string{"Add tests"}},
	}, "research")
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) != 1 || drafts[0].ProjectID != 42 || drafts[0].Status != planningDraftStatusPending {
		t.Fatalf("drafts=%+v", drafts)
	}
	if drafts[0].ID == "" || batchID == "" || drafts[0].BatchID != batchID {
		t.Fatalf("draft ID or batch ID missing: %+v batch=%q", drafts[0], batchID)
	}
}

func TestNewProjectPlanningDraftsRejectsDuplicateTitles(t *testing.T) {
	_, _, err := newProjectPlanningDrafts(42, []ProjectPlanningCardDraft{
		{Title: "Add auth", Column: "backlog"},
		{Title: " add AUTH ", Column: "backlog"},
	}, "cards")
	if err == nil {
		t.Fatal("duplicate generated titles must fail")
	}
}

func TestPendingPlanningDrafts(t *testing.T) {
	queue := []ProjectPlanningStoredDraft{
		{ID: "a", Title: "A", Status: planningDraftStatusPending},
		{ID: "b", Title: "B", Status: planningDraftStatusAdded},
		{ID: "c", Title: "C", Status: planningDraftStatusDismissed},
	}
	pending := pendingPlanningDrafts(queue)
	if len(pending) != 1 || pending[0].ID != "a" {
		t.Fatalf("pending=%+v", pending)
	}
}

func TestProjectPlanningDraftsToAPIUsesServerTimestamps(t *testing.T) {
	created := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	added := created.Add(time.Minute)
	converted := projectPlanningDraftsToAPI([]model.ProjectPlanningDraft{{
		ID: "draft_1", Title: "Card", Column: "backlog", Status: "added", CreatedAt: created, AddedAt: &added,
	}})
	if len(converted) != 1 || converted[0].CreatedAt != created.Format(time.RFC3339Nano) || converted[0].AddedAt != added.Format(time.RFC3339Nano) {
		t.Fatalf("converted=%+v", converted)
	}
}
