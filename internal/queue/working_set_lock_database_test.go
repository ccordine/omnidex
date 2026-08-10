package queue

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5"
)

func TestPostgresWorkingSetWriteSerializesBehindGenerationCutover(t *testing.T) {
	ctx, repository, pool := openWorkingSetDatabase(t)
	marker := fmt.Sprintf("working-set-lock-%d", time.Now().UnixNano())
	job := enqueueWorkingSetTestJob(t, ctx, repository, marker)
	authority := claimWorkingSetTestJob(t, ctx, repository, job)
	if _, err := repository.CreateCurrentWorkingSet(
		ctx, authority, workingset.Budget{MaxItems: 2, MaxBytes: 32},
	); err != nil {
		t.Fatal(err)
	}

	cutover, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer cutover.Rollback(ctx)
	var lockedID int64
	if err := cutover.QueryRow(ctx, `SELECT id FROM jobs WHERE id=$1 FOR UPDATE`, job.ID).Scan(&lockedID); err != nil {
		t.Fatal(err)
	}

	type result struct {
		err error
	}
	commandID := workingSetDatabaseCommandID(t, marker, "blocked-write")
	finished := make(chan result, 1)
	go func() {
		_, applyErr := repository.ApplyWorkingSetCommand(ctx, authority, workingset.AcquireCommand{
			CommandID:       commandID,
			ExpectedVersion: 0, Actor: taskstate.AuthorityCode,
			Request: workingSetDatabaseRequest(
				"item-1", workingset.Scope{Kind: workingset.ScopeTask, ID: "task-1"},
			),
		})
		finished <- result{err: applyErr}
	}()
	select {
	case early := <-finished:
		t.Fatalf("working-set write bypassed the job generation lock: %v", early.err)
	case <-time.After(150 * time.Millisecond):
	}
	advanceWorkingSetGenerationTx(t, ctx, cutover, job.ID)
	if err := cutover.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case completed := <-finished:
		if !errors.Is(completed.err, ErrStaleStepAttempt) {
			t.Fatalf("serialized write error=%v, want ErrStaleStepAttempt", completed.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("working-set write did not finish after generation cutover committed")
	}

	historical, err := repository.WorkingSetForGeneration(ctx, job.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if historical.Version != 0 || len(historical.Items) != 0 {
		t.Fatalf("stale write mutated generation one: %+v", historical)
	}
	events, err := repository.ListWorkingSetEvents(ctx, job.ID, 1, 0, 1)
	if err != nil || len(events) != 0 {
		t.Fatalf("stale write persisted events=%+v error=%v", events, err)
	}
}
