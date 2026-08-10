package queue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresTaskAssignmentRejectsSupersededGeneration(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("task-ledger-generation-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	oldStepID := taskGenerationStepID(t, ctx, pool, job.ID, 1)

	addID, err := taskstate.NewCommandID(marker, "add-unassigned-task")
	if err != nil {
		t.Fatal(err)
	}
	addCommand := taskstate.AddNodeCommand{
		CommandID: addID, ExpectedVersion: initialTaskLedgerVersion, Actor: taskstate.AuthorityCode,
		ID: "generation-task", Kind: taskstate.NodeTask, Title: "Generation-bound task", Priority: 50,
		AcceptanceCriteria: []string{}, Metadata: taskstate.EmptyJSONObject(),
	}
	if _, err := repository.ApplyTaskCommand(ctx, job.ID, 1, addCommand); err != nil {
		t.Fatal(err)
	}

	if _, err := repository.ReplanJob(ctx, testReplanCommand(t, job.ID, "task-generation", "Create a fresh execution generation.")); err != nil {
		t.Fatal(err)
	}
	newStepID := taskGenerationStepID(t, ctx, pool, job.ID, 2)
	if newStepID == oldStepID {
		t.Fatalf("replan reused superseded step %d", oldStepID)
	}
	if _, err := repository.ApplyTaskCommand(ctx, job.ID, 1, addCommand); err != nil {
		t.Fatalf("exact generation-one replay after replan: %v", err)
	}
	if _, err := repository.ApplyTaskCommand(ctx, job.ID, 2, addCommand); !errors.Is(err, taskstate.ErrCommandIDConflict) {
		t.Fatalf("cross-generation replay error=%v", err)
	}
	staleEntryID, err := taskstate.NewCommandID(marker, "stale-step-less-entry")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ApplyTaskCommand(ctx, job.ID, 1, taskstate.AddEntryCommand{
		CommandID: staleEntryID, ExpectedVersion: initialTaskLedgerVersion + 2, Actor: taskstate.AuthorityCode,
		ID: "stale-entry", Kind: taskstate.EntryNote, Content: "This stale command must not commit.",
		Metadata: taskstate.EmptyJSONObject(), Refs: []taskstate.Ref{},
	}); !errors.Is(err, ErrStaleJobGeneration) {
		t.Fatalf("step-less stale generation error=%v", err)
	}

	staleID, err := taskstate.NewCommandID(marker, "assign-superseded-step")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ApplyTaskCommand(ctx, job.ID, 2, taskstate.AssignNodeStepCommand{
		CommandID: staleID, ExpectedVersion: initialTaskLedgerVersion + 2, Actor: taskstate.AuthorityCode,
		NodeID: "generation-task", StepID: oldStepID,
	}); !errors.Is(err, ErrStaleJobGeneration) {
		t.Fatalf("superseded assignment error=%v", err)
	}
	assertTaskAssignmentState(
		t, ctx, pool, job.ID,
		int64(initialTaskLedgerVersion+2), int64(initialTaskLedgerVersion+2), nil,
	)

	currentID, err := taskstate.NewCommandID(marker, "assign-current-step")
	if err != nil {
		t.Fatal(err)
	}
	command := taskstate.AssignNodeStepCommand{
		CommandID: currentID, ExpectedVersion: initialTaskLedgerVersion + 2, Actor: taskstate.AuthorityCode,
		NodeID: "generation-task", StepID: newStepID,
	}
	accepted, err := repository.ApplyTaskCommand(ctx, job.ID, 2, command)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := repository.ApplyTaskCommand(ctx, job.ID, 2, command)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.CommandID != replayed.CommandID || accepted.CommandSHA256 != replayed.CommandSHA256 ||
		accepted.Version != replayed.Version || accepted.Version != initialTaskLedgerVersion+3 {
		t.Fatalf("exact replay changed event identity: accepted=%+v replayed=%+v", accepted, replayed)
	}
	assertTaskAssignmentState(
		t, ctx, pool, job.ID,
		int64(initialTaskLedgerVersion+3), int64(initialTaskLedgerVersion+3), &newStepID,
	)
}

func taskGenerationStepID(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	jobID, generation int64,
) int64 {
	t.Helper()
	var stepID int64
	if err := pool.QueryRow(ctx, `
		SELECT id
		FROM job_steps
		WHERE job_id=$1 AND generation=$2 AND superseded_at_generation IS NULL
		ORDER BY sort_index, id
		LIMIT 1
	`, jobID, generation).Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	return stepID
}

func assertTaskAssignmentState(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	jobID, wantVersion, wantEvents int64,
	wantStepID *int64,
) {
	t.Helper()
	var version, eventCount int64
	var assigned sql.NullInt64
	if err := pool.QueryRow(ctx, `
		SELECT ledgers.version,
		       (SELECT COUNT(*) FROM task_events WHERE job_id=ledgers.job_id),
		       nodes.assigned_step_id
		FROM task_ledgers AS ledgers
		JOIN task_nodes AS nodes ON nodes.ledger_id=ledgers.id AND nodes.id='generation-task'
		WHERE ledgers.job_id=$1
	`, jobID).Scan(&version, &eventCount, &assigned); err != nil {
		t.Fatal(err)
	}
	if version != wantVersion || eventCount != wantEvents {
		t.Fatalf("ledger state version/events=%d/%d want %d/%d", version, eventCount, wantVersion, wantEvents)
	}
	if wantStepID == nil {
		if assigned.Valid {
			t.Fatalf("superseded assignment mutated node to step %d", assigned.Int64)
		}
		return
	}
	if !assigned.Valid || assigned.Int64 != *wantStepID {
		t.Fatalf("assigned step=%v want %d", assigned, *wantStepID)
	}
}
