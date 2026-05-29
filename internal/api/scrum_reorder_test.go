package api

import "testing"

func TestPlaceScrumCardMovesAcrossColumns(t *testing.T) {
	board := ScrumBoard{
		Columns: []string{"backlog", "ready", "assigned"},
		Cards: []ScrumCard{
			{ID: "a", Title: "A", Column: "backlog", BoardOrder: 0, UpdatedAt: "2026-05-29T12:00:00Z"},
			{ID: "b", Title: "B", Column: "backlog", BoardOrder: 1, UpdatedAt: "2026-05-29T11:00:00Z"},
			{ID: "c", Title: "C", Column: "ready", BoardOrder: 0, UpdatedAt: "2026-05-29T10:00:00Z"},
		},
	}

	updated, changed, err := placeScrumCard(&board, "b", "ready", "c")
	if err != nil {
		t.Fatalf("placeScrumCard: %v", err)
	}
	if updated.Column != "ready" {
		t.Fatalf("column=%q want ready", updated.Column)
	}
	if updated.BoardOrder != 0 {
		t.Fatalf("board_order=%d want 0", updated.BoardOrder)
	}
	if len(changed) < 2 {
		t.Fatalf("expected at least 2 changed cards, got %d", len(changed))
	}
	ready := columnCards(&board, "ready", "")
	if len(ready) != 2 || ready[0].ID != "b" || ready[1].ID != "c" {
		t.Fatalf("ready order=%#v", ready)
	}
}

func TestPlaceScrumCardReordersWithinColumn(t *testing.T) {
	board := ScrumBoard{
		Columns: []string{"backlog"},
		Cards: []ScrumCard{
			{ID: "a", Title: "A", Column: "backlog", BoardOrder: 0, UpdatedAt: "2026-05-29T12:00:00Z"},
			{ID: "b", Title: "B", Column: "backlog", BoardOrder: 1, UpdatedAt: "2026-05-29T11:00:00Z"},
			{ID: "c", Title: "C", Column: "backlog", BoardOrder: 2, UpdatedAt: "2026-05-29T10:00:00Z"},
		},
	}

	updated, _, err := placeScrumCard(&board, "c", "backlog", "a")
	if err != nil {
		t.Fatalf("placeScrumCard: %v", err)
	}
	if updated.BoardOrder != 0 {
		t.Fatalf("board_order=%d want 0", updated.BoardOrder)
	}
	backlog := columnCards(&board, "backlog", "")
	if len(backlog) != 3 || backlog[0].ID != "c" || backlog[1].ID != "a" || backlog[2].ID != "b" {
		t.Fatalf("backlog order=%#v", backlog)
	}
}
