package queue

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestPostgresCancelRequiresExactReasonAndReplaysWithoutMutation(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("cancel-authority-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	operationID := testLifecycleOperationID(t, "cancel-exact-replay", job.ID)
	for _, invalid := range []string{"   ", "bad\x00reason", "bad" + string([]byte{0xff})} {
		if _, err := repository.CancelJob(ctx, CancelJobCommand{
			OperationID: operationID, JobID: job.ID, Reason: invalid,
		}); err == nil {
			t.Fatalf("invalid cancel reason %q was accepted", invalid)
		}
	}

	reason := "deliberate exact cancellation"
	command := CancelJobCommand{
		OperationID: operationID, JobID: job.ID, Reason: "  " + reason + "  ",
	}
	canceled, err := repository.CancelJob(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if canceled.Status != model.JobStatusCanceled || canceled.Error != reason {
		t.Fatalf("canceled job=%+v", canceled)
	}
	before, err := repository.ListTaskEvents(ctx, job.ID, 0, maxTaskEventPageSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) < 2 || before[len(before)-2].Event.Reason != reason ||
		before[len(before)-1].Event.Reason != reason {
		t.Fatalf("canonical cancellation events=%+v", before)
	}
	if _, err := repository.CancelJob(ctx, command); err != nil {
		t.Fatalf("exact cancel replay: %v", err)
	}
	after, err := repository.ListTaskEvents(ctx, job.ID, 0, maxTaskEventPageSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("exact replay appended events: before=%d after=%d", len(before), len(after))
	}
	changed := command
	changed.Reason = reason + " changed"
	if _, err := repository.CancelJob(ctx, changed); !errors.Is(err, ErrLifecycleOperationConflict) {
		t.Fatalf("changed cancel replay error=%v", err)
	}
	other, err := repository.EnqueueJob(
		ctx, marker+"-different-scope", model.PipelineCoding, []byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	differentScope := command
	differentScope.JobID = other.ID
	if _, err := repository.CancelJob(ctx, differentScope); !errors.Is(err, ErrLifecycleOperationConflict) {
		t.Fatalf("changed cancel scope error=%v", err)
	}
	if _, err := repository.CancelJob(ctx, testCancelCommand(
		t, other.ID, "cancel-scope-cleanup", "close changed-scope fixture",
	)); err != nil {
		t.Fatal(err)
	}
	secondIdentity := command
	secondIdentity.OperationID = testLifecycleOperationID(t, "cancel-distinct-operation", job.ID)
	if _, err := repository.CancelJob(ctx, secondIdentity); !errors.Is(err, ErrStepNotWritable) {
		t.Fatalf("distinct post-cancel operation error=%v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET error='tampered reason' WHERE id=$1`, job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CancelJob(ctx, command); !errors.Is(err, taskstate.ErrInvalidState) {
		t.Fatalf("tampered cancel authority error=%v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET error=$2 WHERE id=$1`, job.ID, reason); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE task_nodes SET status='active' WHERE job_id=$1 AND id=$2
	`, job.ID, initialTaskRootNodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CancelJob(ctx, command); !errors.Is(err, taskstate.ErrInvalidState) {
		t.Fatalf("tampered cancellation root replay error=%v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE task_nodes SET status='canceled' WHERE job_id=$1 AND id=$2
	`, job.ID, initialTaskRootNodeID); err != nil {
		t.Fatal(err)
	}
	var ledgerClosedAt time.Time
	if err := pool.QueryRow(ctx, `SELECT closed_at FROM task_ledgers WHERE job_id=$1`, job.ID).Scan(&ledgerClosedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE task_ledgers SET status='active', closed_at=NULL WHERE job_id=$1
	`, job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CancelJob(ctx, command); !errors.Is(err, taskstate.ErrInvalidState) {
		t.Fatalf("tampered cancellation ledger replay error=%v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE task_ledgers SET status='canceled', closed_at=$2 WHERE job_id=$1
	`, job.ID, ledgerClosedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE job_lifecycle_operations SET result_job_status='failed' WHERE operation_id=$1
	`, command.OperationID); err == nil {
		t.Fatal("immutable cancellation receipt accepted UPDATE")
	}
	if _, err := pool.Exec(ctx, `
		DELETE FROM lifecycle_operation_registry WHERE operation_id=$1
	`, command.OperationID); err == nil {
		t.Fatal("immutable cancellation registry accepted DELETE")
	}
}
