package queue

import (
	"fmt"
	"testing"
)

func TestScrumCardPageUsesColumnScopedDatabaseBounds(t *testing.T) {
	repository, _, ctx := replanTestRepository(t)
	var projectID int64
	if err := repository.pool.QueryRow(ctx, `
		INSERT INTO projects(location,name) VALUES('/tmp/scrum-page','Scrum page') RETURNING id
	`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 5; index++ {
		if _, err := repository.pool.Exec(ctx, `
			INSERT INTO scrum_cards(id,project_id,title,column_name,board_order)
			VALUES($1,$2,$3,'assigned',$4)
		`, fmt.Sprintf("card-%d", index), projectID, fmt.Sprintf("Card %d", index), index); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repository.pool.Exec(ctx, `
		INSERT INTO scrum_cards(id,project_id,title,column_name,board_order)
		VALUES('other-column',$1,'Other','done',0)
	`, projectID); err != nil {
		t.Fatal(err)
	}
	first, err := repository.ListScrumCardPage(ctx, projectID, ScrumCardPageRequest{Column: "assigned", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.ListScrumCardPage(ctx, projectID, ScrumCardPageRequest{Column: "assigned", Limit: 2, Offset: 2})
	if err != nil {
		t.Fatal(err)
	}
	last, err := repository.ListScrumCardPage(ctx, projectID, ScrumCardPageRequest{Column: "assigned", Limit: 2, Offset: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || !first.HasMore || first.Items[0].ID != "card-0" ||
		len(second.Items) != 2 || !second.HasMore || second.Items[0].ID != "card-2" ||
		len(last.Items) != 1 || last.HasMore || last.Items[0].ID != "card-4" {
		t.Fatalf("unexpected Scrum pages first=%+v second=%+v last=%+v", first, second, last)
	}
	counts, err := repository.CountScrumCardsByColumn(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if counts["assigned"] != 5 || counts["done"] != 1 {
		t.Fatalf("column counts=%v", counts)
	}
}

func TestScrumCardPageRejectsInvalidBounds(t *testing.T) {
	repository, _, ctx := replanTestRepository(t)
	if _, err := repository.ListScrumCardPage(ctx, 1, ScrumCardPageRequest{Limit: 0}); err == nil {
		t.Fatal("expected zero limit to fail")
	}
	if _, err := repository.ListScrumCardPage(ctx, 1, ScrumCardPageRequest{Limit: 1, Offset: -1}); err == nil {
		t.Fatal("expected negative offset to fail")
	}
}
