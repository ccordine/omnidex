package queue

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type preInlineExecutionMigrationJob struct {
	Job      model.Job
	Step     model.Step
	LedgerID taskstate.LedgerID
}

func seedPreInlineExecutionMigrationJob(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	instruction, pipeline, action string,
	metadata json.RawMessage,
) preInlineExecutionMigrationJob {
	t.Helper()
	assertPreInlineExecutionMigrationSchema(t, ctx, pool)
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	fixture := seedPreInlineExecutionMigrationJobTx(
		t, ctx, tx, instruction, pipeline, action, metadata,
	)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func seedPreInlineExecutionMigrationJobTx(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	instruction, pipeline, action string,
	metadata json.RawMessage,
) preInlineExecutionMigrationJob {
	t.Helper()
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	projectID, err := resolveProjectID(ctx, tx, metadata)
	if err != nil {
		t.Fatal(err)
	}
	var runID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO omni_runs (status,prompt_summary)
		VALUES ('running',$1)
		RETURNING id::text
	`, instruction).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	job, err := scanJob(tx.QueryRow(ctx, `
		INSERT INTO jobs (instruction,pipeline,status,metadata,project_id)
		VALUES (
			$1,$2,'pending',
			jsonb_set($3::jsonb,'{telemetry_run_id}',to_jsonb($4::text),true),$5
		)
		RETURNING id,instruction,pipeline,status,result,error,metadata,
		          current_generation,created_at,updated_at,completed_at
	`, instruction, pipeline, string(metadata), runID, projectID))
	if err != nil {
		t.Fatal(err)
	}
	if err := createTaskLedgerTx(ctx, tx, job.ID, runID); err != nil {
		t.Fatal(err)
	}
	var ledgerID taskstate.LedgerID
	if err := tx.QueryRow(ctx, `
		UPDATE task_ledgers SET version=3,updated_at=NOW()
		WHERE job_id=$1 RETURNING id
	`, job.ID).Scan(&ledgerID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO task_nodes (
			ledger_id,job_id,id,kind,title,status,priority,created_by,
			acceptance_criteria,metadata,created_version,updated_version
		) VALUES (
			$1,$2,'goal:root','goal',$3,'ready',100,'code',
			'[]'::jsonb,'{}'::jsonb,1,3
		)
	`, ledgerID, job.ID, instruction); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO task_entries (
			ledger_id,job_id,id,scope_node_id,kind,status,authority,content,
			content_sha256,created_by,metadata,created_version,updated_version
		) VALUES (
			$1,$2,'entry:user-instruction','goal:root','note','active','user',
			'Current user instruction',
			encode(digest('Current user instruction','sha256'),'hex'),
			'user','{}'::jsonb,2,2
		)
	`, ledgerID, job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_generations (job_id,generation,purpose)
		VALUES ($1,1,'initial')
	`, job.ID); err != nil {
		t.Fatal(err)
	}
	step, err := scanStep(tx.QueryRow(ctx, `
		INSERT INTO job_steps (job_id,action,sort_index,status,generation)
		VALUES ($1,$2,0,'pending',1)
		RETURNING id,job_id,action,sort_index,status,generation,
		          superseded_at_generation,worker_id,output,error,
		          started_at,finished_at,created_at,updated_at
	`, job.ID, action))
	if err != nil {
		t.Fatal(err)
	}
	return preInlineExecutionMigrationJob{Job: job, Step: step, LedgerID: ledgerID}
}

func claimPreInlineExecutionMigrationJob(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	fixture preInlineExecutionMigrationJob,
	workerID string,
) *model.ClaimedStep {
	t.Helper()
	assertPreInlineExecutionMigrationSchema(t, ctx, pool)
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var expiresAt time.Time
	if err := tx.QueryRow(ctx, `
		INSERT INTO job_step_attempts (
			job_id,generation,step_id,attempt,worker_id
		) VALUES ($1,1,$2,1,$3)
		RETURNING expires_at
	`, fixture.Job.ID, fixture.Step.ID, workerID).Scan(&expiresAt); err != nil {
		t.Fatal(err)
	}
	step, err := scanStep(tx.QueryRow(ctx, `
		UPDATE job_steps
		SET status='running',worker_id=$2,current_attempt=1,
		    started_at=NOW(),updated_at=NOW()
		WHERE id=$1 AND job_id=$3 AND status='pending' AND current_attempt=0
		RETURNING id,job_id,action,sort_index,status,generation,
		          superseded_at_generation,worker_id,output,error,
		          started_at,finished_at,created_at,updated_at
	`, fixture.Step.ID, workerID, fixture.Job.ID))
	if err != nil {
		t.Fatal(err)
	}
	job, err := scanJob(tx.QueryRow(ctx, `
		UPDATE jobs SET status='running',updated_at=NOW()
		WHERE id=$1 AND status='pending'
		RETURNING id,instruction,pipeline,status,result,error,metadata,
		          current_generation,created_at,updated_at,completed_at
	`, fixture.Job.ID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE task_nodes SET status='active',updated_at=NOW()
		WHERE ledger_id=$1 AND id='goal:root' AND status='ready'
	`, fixture.LedgerID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	authority := model.StepAttemptAuthority{
		JobID: job.ID, Generation: 1, StepID: step.ID, Attempt: 1, WorkerID: workerID,
	}
	return &model.ClaimedStep{
		Job: job, Step: step, Authority: authority, LeaseExpiresAt: expiresAt,
		Contexts: []model.StepContext{},
	}
}

func seedPreInlineExecutionMigrationClaim(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	marker string,
) *model.ClaimedStep {
	t.Helper()
	fixture := seedPreInlineExecutionMigrationJob(
		t, ctx, pool, marker, model.PipelineCoding, "v3_coding", nil,
	)
	return claimPreInlineExecutionMigrationJob(t, ctx, pool, fixture, marker+"-worker")
}

// cancelPreInlineExecutionMigrationClaimAuthority ends only the historical
// attempt authority under test. Calling the current CancelJob path here would
// incorrectly require the task_nodes shape introduced by migration 113.
func cancelPreInlineExecutionMigrationClaimAuthority(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	claim *model.ClaimedStep,
) {
	t.Helper()
	assertPreInlineExecutionMigrationSchema(t, ctx, pool)
	result, err := pool.Exec(ctx, `
		UPDATE job_step_attempts
		SET status=$6,finished_at=NOW()
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4
		  AND worker_id=$5 AND status=$7
	`, claim.Authority.JobID, claim.Authority.Generation, claim.Authority.StepID,
		claim.Authority.Attempt, claim.Authority.WorkerID,
		model.StepAttemptCanceled, model.StepAttemptActive)
	if err != nil {
		t.Fatal(err)
	}
	if result.RowsAffected() != 1 {
		t.Fatalf("historical attempt cancellation rows=%d want 1", result.RowsAffected())
	}
}

func completePreInlineExecutionMigrationClaim(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	claim *model.ClaimedStep,
) {
	t.Helper()
	assertPreInlineExecutionMigrationSchema(t, ctx, pool)
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := requireNoOpenStationGapsTx(ctx, tx, claim.Authority); err != nil {
		t.Fatal(err)
	}
	if result, err := tx.Exec(ctx, `
		UPDATE job_step_attempts
		SET status=$6,finished_at=NOW()
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4
		  AND worker_id=$5 AND status=$7
	`, claim.Authority.JobID, claim.Authority.Generation, claim.Authority.StepID,
		claim.Authority.Attempt, claim.Authority.WorkerID,
		model.StepAttemptCompleted, model.StepAttemptActive); err != nil {
		t.Fatal(err)
	} else if result.RowsAffected() != 1 {
		t.Fatalf("historical attempt completion rows=%d want 1", result.RowsAffected())
	}
	if result, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status=$6,output='historical fixture completed',finished_at=NOW(),updated_at=NOW()
		WHERE job_id=$1 AND generation=$2 AND id=$3 AND current_attempt=$4
		  AND worker_id=$5 AND status=$7
	`, claim.Authority.JobID, claim.Authority.Generation, claim.Authority.StepID,
		claim.Authority.Attempt, claim.Authority.WorkerID,
		model.StepStatusCompleted, model.StepStatusRunning); err != nil {
		t.Fatal(err)
	} else if result.RowsAffected() != 1 {
		t.Fatalf("historical step completion rows=%d want 1", result.RowsAffected())
	}
	if result, err := tx.Exec(ctx, `
		UPDATE jobs
		SET status=$2,result='historical fixture completed',completed_at=NOW(),updated_at=NOW()
		WHERE id=$1 AND status=$3
	`, claim.Authority.JobID, model.JobStatusCompleted, model.JobStatusRunning); err != nil {
		t.Fatal(err)
	} else if result.RowsAffected() != 1 {
		t.Fatalf("historical job completion rows=%d want 1", result.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func assertPreInlineExecutionMigrationSchema(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) {
	t.Helper()
	var inlineExecutionExists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema=current_schema() AND table_name='task_nodes'
			  AND column_name='inline_execution'
		)
	`).Scan(&inlineExecutionExists); err != nil {
		t.Fatal(err)
	}
	if inlineExecutionExists {
		t.Fatal("pre-inline-execution migration fixture requires schema authority before migration 113")
	}
}
