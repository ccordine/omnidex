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

func TestScrumPlayQueueSummary(t *testing.T) {
	board := ScrumBoard{
		Cards: []ScrumCard{
			{ID: "run", PlayState: scrumPlayRunning},
			{ID: "q2", PlayState: scrumPlayQueued, QueueOrder: 2},
			{ID: "q1", PlayState: scrumPlayQueued, QueueOrder: 1},
		},
	}
	summary := scrumPlayQueueSummary(board)
	if summary["running_card_id"] != "run" {
		t.Fatalf("running=%#v", summary["running_card_id"])
	}
	queued := summary["queued_card_ids"].([]string)
	if len(queued) != 2 || queued[0] != "q1" || queued[1] != "q2" {
		t.Fatalf("queued order=%#v", queued)
	}
}
