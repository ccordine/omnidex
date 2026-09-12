package queue_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gryph/omnidex/database"
	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFreshRuntimeSchemaCommitsIndependentTerminalLifecycleOperations(t *testing.T) {
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for isolated PostgreSQL lifecycle coverage")
	}

	t.Run("claimed step failure", func(t *testing.T) {
		_, repository := freshLifecycleRepository(t, databaseURL)
		ctx := context.Background()
		job, err := repository.EnqueueCodingJob(ctx, "exercise a terminal step failure", t.TempDir())
		if err != nil {
			t.Fatalf("enqueue failure fixture: %v", err)
		}
		claim, err := repository.ClaimNextStep(ctx, "lifecycle-failure-fixture")
		if err != nil {
			t.Fatalf("claim failure fixture: %v", err)
		}
		if claim == nil || claim.Job.ID != job.ID {
			t.Fatalf("claimed job = %#v, want job %d", claim, job.ID)
		}
		operationID, err := queue.NewLifecycleOperationID()
		if err != nil {
			t.Fatalf("construct failure operation identity: %v", err)
		}
		const failure = "fixture processing failure"
		command := queue.FailStepCommand{
			OperationID: operationID,
			Authority:   claim.Authority,
			StepID:      claim.Step.ID,
			Error:       failure,
		}
		if err := repository.FailStep(ctx, command); err != nil {
			t.Fatalf("commit failed-step lifecycle operation: %v", err)
		}
		if err := repository.FailStep(ctx, command); err != nil {
			t.Fatalf("replay identical failure command: %v", err)
		}
		command.Error = "different failure text"
		if err := repository.FailStep(ctx, command); !errors.Is(err, queue.ErrLifecycleOperationConflict) {
			t.Fatalf("changed command did not conflict: %v", err)
		}
		details, err := repository.CurrentJobDetails(ctx, job.ID)
		if err != nil {
			t.Fatalf("read failed job: %v", err)
		}
		if details.Job.Status != model.JobStatusFailed || details.Job.Error != failure {
			t.Fatalf("failed job authority = status %q error %q", details.Job.Status, details.Job.Error)
		}
		if len(details.Steps) != 2 ||
			details.Steps[0].Action != "v3_coding_plan" ||
			details.Steps[0].Status != model.StepStatusFailed || details.Steps[0].Error != failure ||
			details.Steps[1].Action != "v3_coding" || details.Steps[1].Status != model.StepStatusPending {
			t.Fatalf("failed step authority = %#v", details.Steps)
		}
	})

	t.Run("pending job cancellation", func(t *testing.T) {
		_, repository := freshLifecycleRepository(t, databaseURL)
		ctx := context.Background()
		workspaceRoot := t.TempDir()
		job, err := repository.EnqueueCodingJob(ctx, "exercise terminal cancellation", workspaceRoot)
		if err != nil {
			t.Fatalf("enqueue cancellation fixture: %v", err)
		}
		workspaceIdentity, err := projectroot.DirectoryIdentity(workspaceRoot)
		if err != nil {
			t.Fatalf("attest cancellation workspace: %v", err)
		}
		operationID, err := queue.NewLifecycleOperationID()
		if err != nil {
			t.Fatalf("construct cancellation operation identity: %v", err)
		}
		const reason = "fixture cancellation"
		command := queue.CancelJobCommand{
			OperationID:   operationID,
			JobID:         job.ID,
			Reason:        reason,
			WorkspaceRoot: workspaceRoot, WorkspaceIdentity: workspaceIdentity,
		}
		result, err := repository.CancelJob(ctx, command)
		if err != nil {
			t.Fatalf("commit cancel-job lifecycle operation: %v", err)
		}
		if !result.Applied || result.Job.Status != model.JobStatusCanceled || result.Job.Error != reason {
			t.Fatalf("cancellation result = %#v", result)
		}
		replay, err := repository.CancelJob(ctx, command)
		if err != nil || replay.Applied || replay.Job.ID != result.Job.ID || replay.Job.Error != reason {
			t.Fatalf("identical cancellation replay = %+v, %v", replay, err)
		}
		command.Reason = "different cancellation reason"
		if _, err := repository.CancelJob(ctx, command); !errors.Is(err, queue.ErrLifecycleOperationConflict) {
			t.Fatalf("changed cancellation did not conflict: %v", err)
		}
		details, err := repository.CurrentJobDetails(ctx, job.ID)
		if err != nil {
			t.Fatalf("read canceled job: %v", err)
		}
		if len(details.Steps) != 2 ||
			details.Steps[0].Action != "v3_coding_plan" || details.Steps[0].Status != model.StepStatusCanceled ||
			details.Steps[1].Action != "v3_coding" || details.Steps[1].Status != model.StepStatusCanceled {
			t.Fatalf("canceled step authority = %#v", details.Steps)
		}
	})
}

func freshLifecycleRepository(t *testing.T, databaseURL string) (*pgxpool.Pool, *queue.Repository) {
	t.Helper()
	schema := "omnidex_lifecycle_test_" + lifecycleNonce(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.ConnectRuntime(ctx, databaseURL, schema, database.SetupSQL())
	if err != nil {
		t.Fatalf("install fresh lifecycle schema %q: %v", schema, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			t.Errorf("drop lifecycle test schema %q: %v", schema, err)
		}
		pool.Close()
	})
	var hashColumns int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema=$1 AND table_name IN (
			'job_generations','job_lifecycle_operations','lifecycle_operation_registry',
			'channel_session_turn_operations'
		) AND column_name LIKE '%sha256%'
	`, schema).Scan(&hashColumns); err != nil {
		t.Fatal(err)
	}
	if hashColumns != 0 {
		t.Fatalf("fresh lifecycle schema retained %d hash columns", hashColumns)
	}
	authority, err := modelconfig.Freeze(modelconfig.Config{})
	if err != nil {
		t.Fatalf("freeze empty model authority: %v", err)
	}
	return pool, queue.New(pool, authority)
}

func lifecycleNonce(t *testing.T) string {
	t.Helper()
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatalf("generate lifecycle test identity: %v", err)
	}
	return hex.EncodeToString(value[:])
}
