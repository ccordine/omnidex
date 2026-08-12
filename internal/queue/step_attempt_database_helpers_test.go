package queue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func activateStepAttemptForTest(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	jobID, generation, stepID int64,
	workerID string,
) model.StepAttemptAuthority {
	t.Helper()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	authority := activateStepAttemptInTxForTest(t, ctx, tx, jobID, generation, stepID, workerID)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return authority
}

func activateStepAttemptInTxForTest(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation, stepID int64,
	workerID string,
) model.StepAttemptAuthority {
	t.Helper()
	var currentAttempt int64
	if err := tx.QueryRow(ctx, `
		SELECT current_attempt FROM job_steps
		WHERE job_id=$1 AND generation=$2 AND id=$3
		FOR UPDATE
	`, jobID, generation, stepID).Scan(&currentAttempt); err != nil {
		t.Fatal(err)
	}
	attempt := currentAttempt + 1
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_step_attempts (
			job_id,generation,step_id,attempt,worker_id,claimed_at,renewed_at
		) VALUES ($1,$2,$3,$4,$5,clock_timestamp(),clock_timestamp())
	`, jobID, generation, stepID, attempt, workerID); err != nil {
		t.Fatal(err)
	}
	if result, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status=$4,worker_id=$5,current_attempt=$6,
			started_at=COALESCE(started_at,clock_timestamp()),updated_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND id=$3 AND status=$7 AND current_attempt=$8
	`, jobID, generation, stepID, model.StepStatusRunning, workerID, attempt,
		model.StepStatusPending, currentAttempt); err != nil {
		t.Fatal(err)
	} else if result.RowsAffected() != 1 {
		t.Fatalf("activate test step %d attempt %d lost exact pending state", stepID, attempt)
	}
	if result, err := tx.Exec(ctx, `
		UPDATE jobs SET status=$2,updated_at=clock_timestamp()
		WHERE id=$1 AND current_generation=$3 AND status IN ($4,$2)
	`, jobID, model.JobStatusRunning, generation, model.JobStatusPending); err != nil {
		t.Fatal(err)
	} else if result.RowsAffected() != 1 {
		t.Fatalf("activate test step %d found no exact current job generation", stepID)
	}
	return model.StepAttemptAuthority{
		JobID: jobID, Generation: generation, StepID: stepID,
		Attempt: attempt, WorkerID: workerID,
	}
}

func testStepAttemptWorker(label string, stepID int64) string {
	return fmt.Sprintf("%s-%d-%d", label, stepID, time.Now().UnixNano())
}

func completeStepAttemptForTest(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	authority model.StepAttemptAuthority,
) {
	t.Helper()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if result, err := tx.Exec(ctx, `
		UPDATE job_step_attempts
		SET status=$5,finished_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4
		  AND worker_id=$6 AND status=$7
	`, authority.JobID, authority.Generation, authority.StepID, authority.Attempt,
		model.StepAttemptCompleted, authority.WorkerID, model.StepAttemptActive); err != nil {
		t.Fatal(err)
	} else if result.RowsAffected() != 1 {
		t.Fatalf("complete test attempt lost exact authority: %+v", authority)
	}
	if result, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status=$4,output='test fixture completed',finished_at=clock_timestamp(),updated_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND id=$3 AND current_attempt=$5 AND worker_id=$6
	`, authority.JobID, authority.Generation, authority.StepID, model.StepStatusCompleted,
		authority.Attempt, authority.WorkerID); err != nil {
		t.Fatal(err)
	} else if result.RowsAffected() != 1 {
		t.Fatalf("complete test step lost exact authority: %+v", authority)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE jobs SET status=$2,result='test fixture completed',completed_at=clock_timestamp(),updated_at=clock_timestamp()
		WHERE id=$1
	`, authority.JobID, model.JobStatusCompleted); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func replaceStepAttemptForTest(
	t *testing.T,
	pool *pgxpool.Pool,
	old model.StepAttemptAuthority,
) model.StepAttemptAuthority {
	t.Helper()
	ctx := t.Context()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if result, err := tx.Exec(ctx, `
		UPDATE job_step_attempts SET status='expired',finished_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4
		  AND worker_id=$5 AND status='active'
	`, old.JobID, old.Generation, old.StepID, old.Attempt, old.WorkerID); err != nil {
		t.Fatal(err)
	} else if result.RowsAffected() != 1 {
		t.Fatal("test replacement did not expire exactly one step attempt")
	}
	replacement := model.StepAttemptAuthority{
		JobID: old.JobID, Generation: old.Generation, StepID: old.StepID,
		Attempt: old.Attempt + 1, WorkerID: "step-attempt-replacement",
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_step_attempts (
			job_id,generation,step_id,attempt,worker_id,claimed_at,renewed_at
		) VALUES ($1,$2,$3,$4,$5,clock_timestamp(),clock_timestamp())
	`, replacement.JobID, replacement.Generation, replacement.StepID,
		replacement.Attempt, replacement.WorkerID); err != nil {
		t.Fatal(err)
	}
	if result, err := tx.Exec(ctx, `
		UPDATE job_steps SET current_attempt=$4,worker_id=$5,updated_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND id=$3 AND current_attempt=$6 AND status='running'
	`, replacement.JobID, replacement.Generation, replacement.StepID,
		replacement.Attempt, replacement.WorkerID, old.Attempt); err != nil {
		t.Fatal(err)
	} else if result.RowsAffected() != 1 {
		t.Fatal("test replacement did not bind exactly one running step")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return replacement
}
