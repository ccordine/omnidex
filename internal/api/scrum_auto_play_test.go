package api

import "testing"

func TestNextAutoPlayThroughScrumCardTopToBottom(t *testing.T) {
	s := &Server{}
	board := ScrumBoard{
		Cards: []ScrumCard{
			{ID: "assigned-top", Column: "assigned", BoardOrder: 1, UpdatedAt: "2026-05-29T12:00:00Z"},
			{ID: "backlog-top", Column: "backlog", BoardOrder: 1, UpdatedAt: "2026-05-29T12:00:00Z"},
			{ID: "ready-top", Column: "ready", BoardOrder: 1, UpdatedAt: "2026-05-29T12:00:00Z"},
			{ID: "backlog-second", Column: "backlog", BoardOrder: 2, UpdatedAt: "2026-05-29T12:00:01Z"},
		},
	}
	got := s.nextAutoPlayThroughScrumCard(board)
	if got == nil || got.ID != "backlog-top" {
		t.Fatalf("expected top backlog card, got %#v", got)
	}

	board.Cards = []ScrumCard{
		{ID: "queued", Column: "assigned", PlayState: scrumPlayQueued, QueueOrder: 1, BoardOrder: 5},
		{ID: "backlog-top", Column: "backlog", BoardOrder: 1},
	}
	got = s.nextAutoPlayThroughScrumCard(board)
	if got == nil || got.ID != "queued" {
		t.Fatalf("expected queued card first, got %#v", got)
	}

	board.Cards = []ScrumCard{
		{ID: "ready-b", Column: "ready", BoardOrder: 2},
		{ID: "ready-a", Column: "ready", BoardOrder: 1},
	}
	got = s.nextAutoPlayThroughScrumCard(board)
	if got == nil || got.ID != "ready-a" {
		t.Fatalf("expected ready-a by board_order, got %#v", got)
	}
}

func TestScrumAutoPlayThroughComplete(t *testing.T) {
	board := ScrumBoard{
		Cards: []ScrumCard{
			{ID: "a", Column: "review"},
			{ID: "b", Column: "done"},
		},
	}
	if !scrumAutoPlayThroughComplete(board) {
		t.Fatal("expected complete when all cards are review/done")
	}
	board.Cards = append(board.Cards, ScrumCard{ID: "c", Column: "assigned"})
	if scrumAutoPlayThroughComplete(board) {
		t.Fatal("expected incomplete when assigned cards remain")
	}
}

func TestLoadScrumAutoPlayThrough(t *testing.T) {
	settings := []byte(`{"scrum_auto_play_through":true}`)
	if !loadScrumAutoPlayThrough(settings) {
		t.Fatal("expected true")
	}
	if loadScrumAutoPlayThrough([]byte(`{"scrum_auto_play_through":false}`)) {
		t.Fatal("expected false")
	}
	if loadScrumAutoPlayThrough(nil) {
		t.Fatal("expected false for empty settings")
	}
}
