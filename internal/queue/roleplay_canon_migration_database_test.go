package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestRoleplayCanonMigrationPreservesExistingCancellationReceipts(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	ctx := t.Context()
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "116")); err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(
		ctx,
		"roleplay-canon-cancel-upgrade",
		model.PipelineCoding,
		[]byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	command := testCancelCommand(
		t,
		job.ID,
		"roleplay-canon-existing-cancel",
		"preserve exact cancellation receipt during roleplay authority upgrade",
	)
	if _, err := repository.CancelJob(ctx, command); err != nil {
		t.Fatal(err)
	}

	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "117")); err != nil {
		t.Fatalf("apply roleplay canon migration over cancellation receipt: %v", err)
	}

	var kind string
	var reason string
	if err := pool.QueryRow(ctx, `
		SELECT kind, command_payload->>'reason'
		FROM job_lifecycle_operations
		WHERE operation_id=$1
	`, command.OperationID).Scan(&kind, &reason); err != nil {
		t.Fatal(err)
	}
	if kind != string(LifecycleCancelJob) || reason != command.Reason {
		t.Fatalf("preserved cancellation receipt kind/reason=%q/%q", kind, reason)
	}
}
