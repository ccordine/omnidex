package queue

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/taskstate"
)

func assertTaskGenerationCommandReplay(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	command taskstate.SupersedeNodeGenerationCommand,
) {
	t.Helper()
	tx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := applyQueueOwnedTaskCommandTx(fixture.Context, tx, fixture.Job.ID, 1, command)
	_ = tx.Rollback(fixture.Context)
	if err != nil || replayed.Kind != taskstate.EventNodeGenerationSuperseded {
		t.Fatalf("exact task-generation replay=%+v err=%v", replayed, err)
	}
	changed := command
	changed.Reason = "Changed task-generation retry content."
	tx, err = fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	_, err = applyQueueOwnedTaskCommandTx(fixture.Context, tx, fixture.Job.ID, 1, changed)
	_ = tx.Rollback(fixture.Context)
	if !errors.Is(err, taskstate.ErrCommandIDConflict) {
		t.Fatalf("changed task-generation replay error=%v", err)
	}
}

func assertSupersededNodeRejectsMutation(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	version uint64,
) {
	t.Helper()
	commandID, err := taskstate.NewCommandID("stale-cognition-node", fmt.Sprint(fixture.Job.ID))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	_, err = applyQueueOwnedTaskCommandTx(fixture.Context, tx, fixture.Job.ID, 2, taskstate.AddEntryCommand{
		CommandID: commandID, ExpectedVersion: version, Actor: taskstate.AuthorityCode,
		ID: "stale-cognition-entry", ScopeNodeID: fixture.NodeID,
		Kind: taskstate.EntryNote, Content: "This stale write must fail.",
		Metadata: taskstate.EmptyJSONObject(), Refs: []taskstate.Ref{},
	})
	_ = tx.Rollback(fixture.Context)
	if !errors.Is(err, taskstate.ErrInvalidState) {
		t.Fatalf("superseded node mutation error=%v", err)
	}
}

type taskGenerationCounts struct{ supersessions, events, generations, operations int }

func taskGenerationRetirementCounts(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
) taskGenerationCounts {
	t.Helper()
	var counts taskGenerationCounts
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT
		 (SELECT COUNT(*) FROM task_node_generation_supersessions WHERE job_id=$1),
		 (SELECT COUNT(*) FROM task_events WHERE job_id=$1 AND event_kind=$2),
		 (SELECT COUNT(*) FROM job_generations WHERE job_id=$1),
		 (SELECT COUNT(*) FROM job_lifecycle_operations WHERE job_id=$1 AND kind=$3)
	`, fixture.Job.ID, taskstate.EventNodeGenerationSuperseded, LifecycleReplanJob).Scan(
		&counts.supersessions, &counts.events, &counts.generations, &counts.operations,
	); err != nil {
		t.Fatal(err)
	}
	return counts
}

func assertTaskGenerationSupersessionImmutable(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	ledgerID string,
) {
	t.Helper()
	var triggers int
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT COUNT(*) FROM pg_trigger
		WHERE tgrelid='task_node_generation_supersessions'::regclass AND NOT tgisinternal
	`).Scan(&triggers); err != nil {
		t.Fatal(err)
	}
	if triggers != 4 {
		t.Fatalf("task-node supersession triggers=%d want 4", triggers)
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`UPDATE task_node_generation_supersessions SET reason='changed' WHERE ledger_id=$1 AND node_id=$2`, []any{ledgerID, fixture.NodeID}},
		{`DELETE FROM task_node_generation_supersessions WHERE ledger_id=$1 AND node_id=$2`, []any{ledgerID, fixture.NodeID}},
		{`TRUNCATE task_node_generation_supersessions`, nil},
	}
	for _, statement := range statements {
		tx, err := fixture.Pool.Begin(fixture.Context)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(fixture.Context, statement.query, statement.args...)
		_ = tx.Rollback(fixture.Context)
		if err == nil {
			t.Fatalf("immutable task-node supersession accepted %q", statement.query)
		}
	}
}

func completeReplannedJob(t *testing.T, fixture taskGenerationRetirementFixture) {
	t.Helper()
	var count int
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT COUNT(*) FROM job_steps
		WHERE job_id=$1 AND generation=2 AND superseded_at_generation IS NULL
	`, fixture.Job.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < count; index++ {
		claim, err := fixture.Repository.ClaimNextStep(
			fixture.Context, fmt.Sprintf("retirement-close-%d-%d", fixture.Job.ID, index),
		)
		if err != nil {
			t.Fatal(err)
		}
		if claim == nil || claim.Job.ID != fixture.Job.ID || claim.Authority.Generation != 2 {
			t.Fatalf("generation-2 claim=%+v", claim)
		}
		if err := fixture.Repository.CompleteStep(fixture.Context, CompleteStepCommand{
			OperationID: testLifecycleOperationID(t, "retirement-close", claim.Step.ID),
			Authority:   claim.Authority, StepID: claim.Step.ID, Output: "verified generation-two step",
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func taskLedgerStateNodeForTest(
	t *testing.T,
	state taskstate.MaterializedState,
	id taskstate.NodeID,
) taskstate.Node {
	t.Helper()
	for _, node := range state.Nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("task node %q is missing", id)
	return taskstate.Node{}
}
