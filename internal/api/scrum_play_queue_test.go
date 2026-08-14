package api

import "testing"

func TestSortCardsForColumnAssignedQueueOrder(t *testing.T) {
	cards := []ScrumCard{
		{ID: "a", Column: "assigned", PlayState: scrumPlayQueued, QueueOrder: 2, UpdatedAt: "2026-05-29T12:00:00Z"},
		{ID: "b", Column: "assigned", UpdatedAt: "2026-05-29T12:00:01Z"},
		{ID: "c", Column: "assigned", PlayState: scrumPlayQueued, QueueOrder: 1, UpdatedAt: "2026-05-29T12:00:02Z"},
	}
	sortCardsForColumn("assigned", cards)
	if cards[0].ID != "b" {
		t.Fatalf("expected non-queued card first, got %s", cards[0].ID)
	}
	if cards[1].ID != "c" || cards[2].ID != "a" {
		t.Fatalf("expected queued cards ordered by queue_order, got %#v", cards[1:])
	}
}

func TestScrumManagerAutoAdvance(t *testing.T) {
	if !scrumManagerAutoAdvance(ScrumOutcomeSuccess) {
		t.Fatal("success should auto-advance")
	}
	if !scrumManagerAutoAdvance(ScrumOutcomeFailed) {
		t.Fatal("failed should auto-advance")
	}
	if scrumManagerAutoAdvance(ScrumOutcomePaused) {
		t.Fatal("paused should not auto-advance")
	}
}
