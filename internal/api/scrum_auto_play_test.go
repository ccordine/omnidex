package api

import "testing"

func TestNextAutoPlayThroughScrumCardDefaultsToAssigned(t *testing.T) {
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
	if got == nil || got.ID != "assigned-top" {
		t.Fatalf("expected assigned card by default, got %#v", got)
	}

	board.Cards = []ScrumCard{
		{ID: "queued", Column: "assigned", PlayState: scrumPlayQueued, QueueOrder: 1, BoardOrder: 5},
		{ID: "backlog-top", Column: "backlog", BoardOrder: 1},
	}
	got = s.nextAutoPlayThroughScrumCard(board)
	if got == nil || got.ID != "queued" {
		t.Fatalf("expected queued card first, got %#v", got)
	}
}

func TestNextAutoWorkScrumCardUsesConfiguredColumns(t *testing.T) {
	s := &Server{}
	board := ScrumBoard{Cards: []ScrumCard{
		{ID: "ready-b", Column: "ready", BoardOrder: 2},
		{ID: "ready-a", Column: "ready", BoardOrder: 1},
		{ID: "assigned-a", Column: "assigned", BoardOrder: 1},
	}}
	got := s.nextAutoWorkScrumCard(board, ScrumAutoWorkConfig{Enabled: true, SourceColumns: []string{"ready", "assigned"}})
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

func TestLoadScrumAutoWorkConfig(t *testing.T) {
	cfg := loadScrumAutoWorkConfig([]byte(`{"scrum_auto_work":{"enabled":true,"source_columns":["ready","assigned","review","ready"]}}`))
	if !cfg.Enabled {
		t.Fatal("expected enabled")
	}
	if len(cfg.SourceColumns) != 2 || cfg.SourceColumns[0] != "ready" || cfg.SourceColumns[1] != "assigned" {
		t.Fatalf("unexpected source columns: %#v", cfg.SourceColumns)
	}
	cfg = loadScrumAutoWorkConfig([]byte(`{"scrum_auto_play_through":true}`))
	if !cfg.Enabled || len(cfg.SourceColumns) != 1 || cfg.SourceColumns[0] != "assigned" {
		t.Fatalf("expected legacy enabled with assigned default, got %#v", cfg)
	}
}
