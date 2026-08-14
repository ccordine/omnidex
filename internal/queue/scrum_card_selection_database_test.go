package queue

import (
	"strconv"
	"testing"
)

func TestScrumPlayAndAutoWorkQueriesSelectExactBoundedState(t *testing.T) {
	repository, _, ctx := replanTestRepository(t)
	var projectID int64
	if err := repository.pool.QueryRow(ctx, `
		INSERT INTO projects(location,name,settings)
		VALUES('/tmp/scrum-selection','Scrum selection',
		       '{"scrum_auto_work":{"enabled":true,"source_columns":["ready","assigned"]}}')
		RETURNING id
	`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	for _, seed := range []struct {
		id, column, state string
		queue, board      int
	}{{"ready", "ready", "", 0, 0}, {"assigned", "assigned", "", 0, 0}, {"queued", "assigned", "queued", 7, 1}} {
		if _, err := repository.pool.Exec(ctx, `
			INSERT INTO scrum_cards(id,project_id,title,column_name,play_state,queue_order,board_order)
			VALUES($1,$2,$1,$3,$4,$5,$6)
		`, seed.id, projectID, seed.column, seed.state, seed.queue, seed.board); err != nil {
			t.Fatal(err)
		}
	}
	queued, found, err := repository.FindNextQueuedScrumCard(ctx, projectID)
	if err != nil || !found || queued.ID != "queued" {
		t.Fatalf("queued selection found=%v card=%+v err=%v", found, queued, err)
	}
	eligible, found, err := repository.FindNextEligibleScrumCard(ctx, projectID, []string{"ready", "assigned"})
	if err != nil || !found || eligible.ID != "ready" {
		t.Fatalf("eligible selection found=%v card=%+v err=%v", found, eligible, err)
	}
	global, found, err := repository.FindGlobalScrumAutoWorkCandidate(ctx)
	if err != nil || !found || global.ProjectID != projectID || global.CardID != "queued" {
		t.Fatalf("global selection found=%v candidate=%+v err=%v", found, global, err)
	}
	snapshot, err := repository.ScrumPlayQueueSnapshot(ctx, projectID, 1)
	if err != nil || snapshot.QueuedCount != 1 || len(snapshot.QueuedCardIDs) != 1 || snapshot.QueuedCardIDs[0] != "queued" {
		t.Fatalf("queue snapshot=%+v err=%v", snapshot, err)
	}
	complete, err := repository.ScrumProjectComplete(ctx, projectID)
	if err != nil || complete {
		t.Fatalf("incomplete project complete=%v err=%v", complete, err)
	}
}

func TestScrumPlaySelectionRejectsMultipleRunningCards(t *testing.T) {
	repository, _, ctx := replanTestRepository(t)
	var projectID int64
	if err := repository.pool.QueryRow(ctx, `
		INSERT INTO projects(location,name) VALUES('/tmp/scrum-running','Scrum running') RETURNING id
	`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"running-a", "running-b"} {
		if err := func() error {
			tx, err := repository.pool.Begin(ctx)
			if err != nil {
				return err
			}
			defer tx.Rollback(ctx) //nolint:errcheck -- committed transactions return ErrTxClosed.
			var jobID int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO jobs(project_id,instruction,pipeline,status,metadata)
				VALUES($1,$2,'scrum','pending','{}'::jsonb) RETURNING id
			`, projectID, "run "+id).Scan(&jobID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO job_generations(job_id,generation,purpose) VALUES($1,1,'initial')
			`, jobID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO scrum_cards(id,project_id,title,column_name,play_state,job_id,sync_job_id)
				VALUES($1,$2,$1,'in_progress','running',$3,$3)
			`, id, projectID, strconv.FormatInt(jobID, 10)); err != nil {
				return err
			}
			return tx.Commit(ctx)
		}(); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := repository.FindRunningScrumCard(ctx, projectID); err == nil {
		t.Fatal("expected duplicate running cards to violate the exact selection invariant")
	}
}
