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

func TestScrumManagerAutoAdvance(t *testing.T) {
	if !scrumManagerAutoAdvance(ScrumOutcomeSuccess) {
		t.Fatal("success should auto-advance")
	}
	if !scrumManagerAutoAdvance(ScrumOutcomeBlocked) {
		t.Fatal("blocked should auto-advance")
	}
	if scrumManagerAutoAdvance(ScrumOutcomePaused) {
		t.Fatal("paused should not auto-advance")
	}
}

func TestNextAutoPlayScrumCardPriority(t *testing.T) {
	s := &Server{}
	board := ScrumBoard{
		Cards: []ScrumCard{
			{ID: "queued", Column: "assigned", PlayState: scrumPlayQueued, QueueOrder: 1},
			{ID: "paused", Column: "assigned", PlayState: scrumPlayPaused, UpdatedAt: "2026-05-29T10:00:00Z"},
			{ID: "idle-assigned", Column: "assigned", UpdatedAt: "2026-05-29T12:00:00Z"},
			{ID: "idle-progress", Column: "in_progress", UpdatedAt: "2026-05-29T11:00:00Z"},
		},
	}
	if got := s.nextAutoPlayScrumCard(board); got == nil || got.ID != "queued" {
		t.Fatalf("expected queued card, got %#v", got)
	}

	board.Cards = board.Cards[1:]
	if got := s.nextAutoPlayScrumCard(board); got == nil || got.ID != "paused" {
		t.Fatalf("expected paused card, got %#v", got)
	}

	board.Cards = board.Cards[1:]
	if got := s.nextAutoPlayScrumCard(board); got == nil || got.ID != "idle-progress" {
		t.Fatalf("expected idle in_progress card, got %#v", got)
	}

	board.Cards = board.Cards[:1]
	if got := s.nextAutoPlayScrumCard(board); got == nil || got.ID != "idle-assigned" {
		t.Fatalf("expected idle assigned card, got %#v", got)
	}
}

func TestNextPausedScrumCardFIFO(t *testing.T) {
	s := &Server{}
	board := ScrumBoard{
		Cards: []ScrumCard{
			{ID: "newer", Column: "assigned", PlayState: scrumPlayPaused, UpdatedAt: "2026-05-29T12:00:00Z"},
			{ID: "older", Column: "assigned", PlayState: scrumPlayPaused, UpdatedAt: "2026-05-29T10:00:00Z"},
		},
	}
	got := s.nextPausedScrumCard(board)
	if got == nil || got.ID != "older" {
		t.Fatalf("expected oldest paused card first, got %#v", got)
	}
}
