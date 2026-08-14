package queue

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestMoveScrumCardReordersColumnsInsideDatabase(t *testing.T) {
	repository, _, ctx := scrumChannelOperationTestRepository(t)
	var projectID int64
	if err := repository.pool.QueryRow(ctx, `
		INSERT INTO projects(location,name) VALUES('/tmp/scrum-move','Scrum move') RETURNING id
	`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	for _, seed := range []struct {
		id, column string
		order      int
	}{{"source-a", "backlog", 0}, {"source-b", "backlog", 1}, {"target-a", "assigned", 0}, {"target-b", "assigned", 1}} {
		if _, err := repository.pool.Exec(ctx, `
			INSERT INTO scrum_cards(id,project_id,title,column_name,board_order)
			VALUES($1,$2,$1,$3,$4)
		`, seed.id, projectID, seed.column, seed.order); err != nil {
			t.Fatal(err)
		}
	}
	sourceRevision, err := repository.GetScrumCard(ctx, projectID, "source-a")
	if err != nil {
		t.Fatal(err)
	}
	targetRevision, err := repository.GetScrumCard(ctx, projectID, "target-b")
	if err != nil {
		t.Fatal(err)
	}
	previous, updated, err := repository.MoveScrumCard(ctx, ScrumCardMove{
		ProjectID: projectID, CardID: "source-a", Column: ScrumCardAssigned, BeforeCardID: "target-b",
		ExpectedUpdatedAt: sourceRevision.UpdatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if previous.Column != "backlog" || updated.Column != "assigned" || updated.BoardOrder != 1 {
		t.Fatalf("unexpected transition previous=%+v updated=%+v", previous, updated)
	}
	var metrics ScrumFlowMetrics
	if err := json.Unmarshal(updated.FlowMetrics, &metrics); err != nil {
		t.Fatal(err)
	}
	if metrics.Column != "assigned" || metrics.RegressionCount != 0 {
		t.Fatalf("atomic move metrics=%+v", metrics)
	}
	target, err := repository.ListScrumCardPage(ctx, projectID, ScrumCardPageRequest{Column: "assigned", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(target.Items) != 3 || target.Items[0].ID != "target-a" || target.Items[1].ID != "source-a" || target.Items[2].ID != "target-b" {
		t.Fatalf("target order=%v", scrumCardIDs(target.Items))
	}
	targetAfter, err := repository.GetScrumCard(ctx, projectID, "target-b")
	if err != nil {
		t.Fatal(err)
	}
	if !targetAfter.UpdatedAt.After(targetRevision.UpdatedAt) {
		t.Fatalf("reordered target revision did not advance before=%s after=%s", targetRevision.UpdatedAt, targetAfter.UpdatedAt)
	}
	source, err := repository.ListScrumCardPage(ctx, projectID, ScrumCardPageRequest{Column: "backlog", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(source.Items) != 1 || source.Items[0].ID != "source-b" || source.Items[0].BoardOrder != 0 {
		t.Fatalf("source post-state=%+v", source.Items)
	}
}

func TestMoveScrumCardRejectsUnknownBeforeWithoutMutation(t *testing.T) {
	repository, _, ctx := scrumChannelOperationTestRepository(t)
	var projectID int64
	if err := repository.pool.QueryRow(ctx, `
		INSERT INTO projects(location,name) VALUES('/tmp/scrum-move-reject','Scrum move reject') RETURNING id
	`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `
		INSERT INTO scrum_cards(id,project_id,title,column_name,board_order)
		VALUES('source',$1,'Source','backlog',0)
	`, projectID); err != nil {
		t.Fatal(err)
	}
	sourceRevision, err := repository.GetScrumCard(ctx, projectID, "source")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.MoveScrumCard(ctx, ScrumCardMove{
		ProjectID: projectID, CardID: "source", Column: ScrumCardAssigned, BeforeCardID: "missing",
		ExpectedUpdatedAt: sourceRevision.UpdatedAt,
	}); err == nil {
		t.Fatal("expected unknown before card to fail")
	}
	card, err := repository.GetScrumCard(ctx, projectID, "source")
	if err != nil {
		t.Fatal(err)
	}
	if card.Column != "backlog" || card.BoardOrder != 0 {
		t.Fatalf("failed move mutated card: %+v", card)
	}
}

func TestMoveScrumCardRejectsStaleRevisionWithoutReordering(t *testing.T) {
	repository, _, ctx := scrumChannelOperationTestRepository(t)
	var projectID int64
	if err := repository.pool.QueryRow(ctx, `
		INSERT INTO projects(location,name) VALUES('/tmp/scrum-move-stale','Scrum move stale') RETURNING id
	`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `
		INSERT INTO scrum_cards(id,project_id,title,column_name,board_order)
		VALUES('source',$1,'Source','backlog',0),('target',$1,'Target','assigned',0)
	`, projectID); err != nil {
		t.Fatal(err)
	}
	current, err := repository.GetScrumCard(ctx, projectID, "source")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `
		UPDATE scrum_cards SET description='newer state',
		 updated_at=GREATEST(clock_timestamp(),updated_at+interval '1 microsecond')
		WHERE project_id=$1 AND id='source'
	`, projectID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.MoveScrumCard(ctx, ScrumCardMove{
		ProjectID: projectID, CardID: "source", Column: ScrumCardAssigned,
		BeforeCardID: "target", ExpectedUpdatedAt: current.UpdatedAt,
	}); !errors.Is(err, ErrScrumCardVersionConflict) {
		t.Fatalf("MoveScrumCard stale error=%v", err)
	}
	after, err := repository.GetScrumCard(ctx, projectID, "source")
	if err != nil {
		t.Fatal(err)
	}
	if after.Column != "backlog" || after.BoardOrder != 0 || string(after.Description) != "newer state" {
		t.Fatalf("stale move changed authoritative card: %+v", after)
	}
}

func TestMoveScrumCardRejectsActiveCardWithoutMutation(t *testing.T) {
	repository, _, ctx := scrumChannelOperationTestRepository(t)
	var projectID int64
	if err := repository.pool.QueryRow(ctx, `
		INSERT INTO projects(location,name) VALUES('/tmp/scrum-move-active','Scrum move active') RETURNING id
	`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `
		INSERT INTO scrum_cards(id,project_id,title,column_name,board_order)
		VALUES('source',$1,'Source','in_progress',0),('target',$1,'Target','done',0)
	`, projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `
		UPDATE scrum_cards
		SET job_id='101',sync_job_id='101',play_state='running',step_context_cursor=7,
		    updated_at=GREATEST(clock_timestamp(),updated_at+interval '1 microsecond')
		WHERE project_id=$1 AND id='source'
	`, projectID); err != nil {
		t.Fatal(err)
	}
	before, err := repository.GetScrumCard(ctx, projectID, "source")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.MoveScrumCard(ctx, ScrumCardMove{
		ProjectID: projectID, CardID: "source", Column: ScrumCardDone,
		BeforeCardID: "target", ExpectedUpdatedAt: before.UpdatedAt,
	}); !errors.Is(err, ErrScrumCardActiveMove) {
		t.Fatalf("active move error=%v", err)
	}
	after, err := repository.GetScrumCard(ctx, projectID, "source")
	if err != nil {
		t.Fatal(err)
	}
	if after.Column != before.Column || after.BoardOrder != before.BoardOrder ||
		after.PlayState != before.PlayState || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("active move changed authoritative card before=%+v after=%+v", before, after)
	}
}

func scrumCardIDs(cards []DBScrumCardSummary) []string {
	ids := make([]string, 0, len(cards))
	for _, card := range cards {
		ids = append(ids, card.ID)
	}
	return ids
}
