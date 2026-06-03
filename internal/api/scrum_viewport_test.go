package api

import (
	"net/http"
	"testing"
)

func TestScrumBoardColumnViewportReturnsOnlyRequestedColumn(t *testing.T) {
	board := ScrumBoard{
		Columns: []string{"backlog", "assigned", "done"},
		Cards: []ScrumCard{
			{ID: "a", Column: "backlog", BoardOrder: 1},
			{ID: "b", Column: "assigned", BoardOrder: 2},
			{ID: "c", Column: "assigned", BoardOrder: 1},
			{ID: "d", Column: "done", BoardOrder: 1},
		},
	}

	viewport := scrumBoardColumnViewport(board, "assigned")
	if len(viewport.Columns) != 1 || viewport.Columns[0] != "assigned" {
		t.Fatalf("viewport.Columns=%v want [assigned]", viewport.Columns)
	}
	if len(viewport.Cards) != 2 {
		t.Fatalf("len(viewport.Cards)=%d want 2", len(viewport.Cards))
	}
	if viewport.Cards[0].ID != "c" || viewport.Cards[1].ID != "b" {
		t.Fatalf("viewport card order=%v want c,b", []string{viewport.Cards[0].ID, viewport.Cards[1].ID})
	}
}

func TestScrumViewportColumnDefaultsToAssigned(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/v1/scrum", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := scrumViewportColumn(req, []string{"backlog", "assigned"}); got != "assigned" {
		t.Fatalf("scrumViewportColumn()=%q want assigned", got)
	}
}
