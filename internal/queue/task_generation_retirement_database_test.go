package queue

import (
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestPostgresReplanSupersedesCognitionObligationsExactly(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "commit")
	command := testReplanCommand(
		t, fixture.Job.ID, "cognition-retirement", "Replace the current cognition generation.",
	)
	replanned, err := fixture.Repository.ReplanJob(fixture.Context, command)
	if err != nil {
		t.Fatal(err)
	}
	if replanned.CurrentGeneration != 2 || replanned.Status != model.JobStatusRunning {
		t.Fatalf("replanned job=%+v", replanned)
	}
	assertLifecycleCognitionRetired(t, fixture, command.OperationID, LifecycleReplanJob)
	var retiredAttempt model.StepAttemptStatus
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT status FROM job_step_attempts
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4 AND worker_id=$5
	`, fixture.Authority.JobID, fixture.Authority.Generation, fixture.Authority.StepID,
		fixture.Authority.Attempt, fixture.Authority.WorkerID).Scan(&retiredAttempt); err != nil {
		t.Fatal(err)
	}
	if retiredAttempt != model.StepAttemptSuperseded {
		t.Fatalf("retired cognition attempt status=%q", retiredAttempt)
	}

	var ledgerID string
	var eventGeneration, eventVersion, projectionVersion int64
	var reason string
	var containsNode bool
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT events.ledger_id,events.job_generation,events.ledger_version,
		       events.payload->>'reason',events.payload->'node_ids' @> to_jsonb(ARRAY[$2]::text[]),
		       supersessions.created_version
		FROM task_events AS events
		JOIN task_node_generation_supersessions AS supersessions
		  ON supersessions.ledger_id=events.ledger_id
		 AND supersessions.created_version=events.ledger_version
		WHERE events.job_id=$1 AND events.event_kind=$3 AND supersessions.node_id=$2
	`, fixture.Job.ID, fixture.NodeID, taskstate.EventNodeGenerationSuperseded).Scan(
		&ledgerID, &eventGeneration, &eventVersion, &reason, &containsNode, &projectionVersion,
	); err != nil {
		t.Fatal(err)
	}
	wantReason := "Job generation 2 superseded cognition obligations from generation 1."
	if eventGeneration != 1 || eventVersion != projectionVersion || reason != wantReason || !containsNode {
		t.Fatalf("supersession event=%d/%d projection=%d reason=%q contains=%t",
			eventGeneration, eventVersion, projectionVersion, reason, containsNode)
	}
	state, err := fixture.Repository.TaskLedger(fixture.Context, fixture.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	oldNode := taskLedgerStateNodeForTest(t, state, fixture.NodeID)
	if oldNode.Status != taskstate.NodeCanceled ||
		oldNode.StatusReason != "The cognition episode was canceled by code authority." ||
		len(state.NodeSupersessions) != 1 || state.NodeSupersessions[0].NodeID != fixture.NodeID {
		t.Fatalf("retired task state node=%+v supersessions=%+v", oldNode, state.NodeSupersessions)
	}

	underlying, err := taskGenerationRetirementCommand(
		fixture.Job.ID, uint64(eventVersion-1), 1, 2, []taskstate.NodeID{fixture.NodeID},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertTaskGenerationCommandReplay(t, fixture, underlying)
	before := taskGenerationRetirementCounts(t, fixture)
	beforeCognition := lifecycleCognitionCounts(t, fixture, command.OperationID)
	replayed, err := fixture.Repository.ReplanJob(fixture.Context, command)
	if err != nil || !reflect.DeepEqual(replayed, replanned) {
		t.Fatalf("exact replan replay=%+v err=%v want %+v", replayed, err, replanned)
	}
	if after := taskGenerationRetirementCounts(t, fixture); after != before {
		t.Fatalf("replan replay counts=%v want %v", after, before)
	}
	if after := lifecycleCognitionCounts(t, fixture, command.OperationID); after != beforeCognition {
		t.Fatalf("replan cognition replay counts=%v want %v", after, beforeCognition)
	}
	changed := command
	changed.Feedback = "Changed retry content."
	if _, err := fixture.Repository.ReplanJob(fixture.Context, changed); !errors.Is(err, ErrLifecycleOperationConflict) {
		t.Fatalf("changed replan retry error=%v", err)
	}

	assertSupersededNodeRejectsMutation(t, fixture, state.Version)
	assertTaskGenerationSupersessionImmutable(t, fixture, ledgerID)
	completeReplannedJob(t, fixture)
	closed, err := fixture.Repository.TaskLedger(fixture.Context, fixture.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != taskstate.LedgerClosed ||
		taskLedgerStateNodeForTest(t, closed, initialTaskRootNodeID).Status != taskstate.NodeDone ||
		taskLedgerStateNodeForTest(t, closed, fixture.NodeID).Status != taskstate.NodeCanceled {
		t.Fatalf("closed ledger did not ignore superseded history: %+v", closed)
	}
}

func TestPostgresReplanRollsBackCognitionObligationRetirement(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "rollback")
	command := testReplanCommand(
		t, fixture.Job.ID, "cognition-retirement-rollback", "Roll this generation change back.",
	)
	normalized, feedbackSHA, err := normalizeReplanJobCommand(command)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := describeLifecycleOperation(normalized.OperationID, LifecycleReplanJob, normalized)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	inside, err := replanJobTx(fixture.Context, tx, normalized, feedbackSHA, descriptor)
	if err != nil {
		_ = tx.Rollback(fixture.Context)
		t.Fatal(err)
	}
	if inside.CurrentGeneration != 2 {
		_ = tx.Rollback(fixture.Context)
		t.Fatalf("transactional replan generation=%d", inside.CurrentGeneration)
	}
	var insideSupersessions int
	if err := tx.QueryRow(fixture.Context, `
		SELECT COUNT(*) FROM task_node_generation_supersessions WHERE job_id=$1
	`, fixture.Job.ID).Scan(&insideSupersessions); err != nil {
		_ = tx.Rollback(fixture.Context)
		t.Fatal(err)
	}
	if insideSupersessions != 1 {
		_ = tx.Rollback(fixture.Context)
		t.Fatalf("transactional supersessions=%d want 1", insideSupersessions)
	}
	if err := tx.Rollback(fixture.Context); err != nil {
		t.Fatal(err)
	}

	var generation, generations, supersessions, events, operations int
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT jobs.current_generation,
		       (SELECT COUNT(*) FROM job_generations WHERE job_id=jobs.id),
		       (SELECT COUNT(*) FROM task_node_generation_supersessions WHERE job_id=jobs.id),
		       (SELECT COUNT(*) FROM task_events WHERE job_id=jobs.id AND event_kind=$2),
		       (SELECT COUNT(*) FROM job_lifecycle_operations WHERE job_id=jobs.id AND kind=$3)
		FROM jobs WHERE jobs.id=$1
	`, fixture.Job.ID, taskstate.EventNodeGenerationSuperseded, LifecycleReplanJob).Scan(
		&generation, &generations, &supersessions, &events, &operations,
	); err != nil {
		t.Fatal(err)
	}
	if generation != 1 || generations != 1 || supersessions != 0 || events != 0 || operations != 0 {
		t.Fatalf("rollback state=%d/%d/%d/%d/%d", generation, generations, supersessions, events, operations)
	}
	state, err := fixture.Repository.TaskLedger(fixture.Context, fixture.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.NodeSupersessions) != 0 || taskLedgerStateNodeForTest(t, state, fixture.NodeID).Status != taskstate.NodeActive {
		t.Fatalf("rollback ledger=%+v", state)
	}
}
