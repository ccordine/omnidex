package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestAppendPlanningDraftsDedupesPendingTitles(t *testing.T) {
	queue := []ProjectPlanningStoredDraft{
		{ID: "draft_1", Title: "Add auth", Status: planningDraftStatusPending, CreatedAt: "2026-01-01T00:00:00Z"},
	}
	queue = appendPlanningDrafts(queue, []ProjectPlanningCardDraft{
		{Title: "Add auth", Column: "backlog"},
		{Title: "Session middleware", Column: "backlog", Checklist: []string{"cookie config"}},
	}, "research", "batch_1")
	if len(queue) != 2 {
		t.Fatalf("queue len=%d want 2", len(queue))
	}
	if queue[1].Title != "Session middleware" || queue[1].BatchID != "batch_1" {
		t.Fatalf("second draft=%+v", queue[1])
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

func TestLoadPlanningDraftQueueFromSettings(t *testing.T) {
	settings := json.RawMessage(`{"planning_draft_queue":[{"id":"draft_1","title":"Card","status":"pending","created_at":"2026-01-01T00:00:00Z"}]}`)
	queue := loadPlanningDraftQueue(settings)
	if len(queue) != 1 || queue[0].Title != "Card" {
		t.Fatalf("queue=%+v", queue)
	}
}

func TestFormatPlanningDraftDescription(t *testing.T) {
	text := formatPlanningDraftDescription(ProjectPlanningCardDraft{
		Description: "Implement sessions",
		Checklist:   []string{"middleware", "tests"},
	})
	if !strings.Contains(text, "Implement sessions") || !strings.Contains(text, "Checklist:") {
		t.Fatalf("text=%q", text)
	}
}

func TestTrimPlanningDraftQueueDropsDismissedFirst(t *testing.T) {
	queue := make([]ProjectPlanningStoredDraft, 0, planningDraftQueueMax+1)
	for i := 0; i < planningDraftQueueMax; i++ {
		queue = append(queue, ProjectPlanningStoredDraft{
			ID:        fmt.Sprintf("pending_%d", i),
			Title:     "Pending",
			Status:    planningDraftStatusPending,
			CreatedAt: "2026-01-01T00:00:00Z",
		})
	}
	queue = append(queue, ProjectPlanningStoredDraft{
		ID:        "dismissed_1",
		Title:     "Old dismissed",
		Status:    planningDraftStatusDismissed,
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	trimmed := trimPlanningDraftQueue(queue)
	if len(trimmed) != planningDraftQueueMax {
		t.Fatalf("len=%d want %d", len(trimmed), planningDraftQueueMax)
	}
	for _, draft := range trimmed {
		if draft.ID == "dismissed_1" {
			t.Fatal("dismissed draft should have been dropped")
		}
	}
}
