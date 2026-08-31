package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ClaimNextStep(ctx context.Context, workerID string) (*model.ClaimedStep, error) {
	if workerID == "" || workerID != strings.TrimSpace(workerID) || len(workerID) > 256 ||
		!utf8.ValidString(workerID) || strings.ContainsRune(workerID, '\x00') {
		return nil, fmt.Errorf("worker identity must be exact PostgreSQL-compatible text of at most 256 bytes")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var databaseNow time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&databaseNow); err != nil {
		return nil, fmt.Errorf("read database claim clock: %w", err)
	}

	var candidateJobID int64
	err = tx.QueryRow(ctx, `
		SELECT jobs.id
		FROM jobs
		WHERE jobs.status IN ($1, $2)
		  AND EXISTS (
		      SELECT 1 FROM job_steps AS candidate
		      WHERE candidate.job_id=jobs.id
		        AND candidate.superseded_at_generation IS NULL
		        AND candidate.generation=jobs.current_generation
		        AND (
		            (candidate.status=$3 AND candidate.current_attempt=0) OR
		            (candidate.status=$4 AND EXISTS (
		                SELECT 1 FROM job_step_attempts AS attempts
		                WHERE attempts.job_id=candidate.job_id
		                  AND attempts.generation=candidate.generation
		                  AND attempts.step_id=candidate.id
		                  AND attempts.attempt=candidate.current_attempt
		                  AND attempts.status=$5
		                  AND attempts.expires_at<=$6
		            ))
		        )
		        AND NOT EXISTS (
		            SELECT 1 FROM job_steps AS predecessor
		            WHERE predecessor.job_id=candidate.job_id
		              AND predecessor.superseded_at_generation IS NULL
		              AND predecessor.sort_index<candidate.sort_index
		              AND predecessor.status<>$7
		        )
		  )
		ORDER BY jobs.id ASC
		FOR UPDATE OF jobs SKIP LOCKED
		LIMIT 1
	`, model.JobStatusPending, model.JobStatusRunning, model.StepStatusPending,
		model.StepStatusRunning, model.StepAttemptActive, databaseNow,
		model.StepStatusCompleted).Scan(&candidateJobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	step, job, err := scanClaim(tx.QueryRow(ctx, `
		SELECT
			s.id, s.job_id, s.action, s.sort_index, s.status, s.generation, s.superseded_at_generation,
			s.worker_id, s.output, s.error,
			s.started_at, s.finished_at, s.created_at, s.updated_at,
			j.id, j.instruction, j.pipeline, j.status, j.result, j.error, j.metadata, j.current_generation,
			j.created_at, j.updated_at, j.completed_at
		FROM job_steps AS s
		JOIN jobs AS j ON j.id=s.job_id
		WHERE s.job_id=$1
		  AND s.superseded_at_generation IS NULL
		  AND s.generation=j.current_generation
		  AND j.status IN ($2, $3)
		  AND (
		      (s.status=$4 AND s.current_attempt=0) OR
		      (s.status=$5 AND EXISTS (
		          SELECT 1 FROM job_step_attempts AS attempts
		          WHERE attempts.job_id=s.job_id
		            AND attempts.generation=s.generation
		            AND attempts.step_id=s.id
		            AND attempts.attempt=s.current_attempt
		            AND attempts.status=$6
		            AND attempts.expires_at<=$7
		      ))
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM job_steps AS predecessor
		      WHERE predecessor.job_id=s.job_id
		        AND predecessor.superseded_at_generation IS NULL
		        AND predecessor.sort_index<s.sort_index
		        AND predecessor.status<>$8
		  )
		ORDER BY s.sort_index ASC, s.id ASC
		FOR UPDATE OF s SKIP LOCKED
		LIMIT 1
	`, candidateJobID, model.JobStatusPending, model.JobStatusRunning,
		model.StepStatusPending, model.StepStatusRunning, model.StepAttemptActive,
		databaseNow, model.StepStatusCompleted))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := validatePipeline(job.Pipeline); err != nil {
		return nil, fmt.Errorf("claim job %d: %w", job.ID, err)
	}

	var previousAttempt int64
	if err := tx.QueryRow(ctx, `
		SELECT current_attempt FROM job_steps WHERE id=$1 FOR UPDATE
	`, step.ID).Scan(&previousAttempt); err != nil {
		return nil, err
	}
	if step.Status == model.StepStatusPending && previousAttempt != 0 {
		return nil, fmt.Errorf("%w: pending step %d has attempt %d", ErrStepNotWritable, step.ID, previousAttempt)
	}
	if step.Status == model.StepStatusRunning {
		result, err := tx.Exec(ctx, `
			UPDATE job_step_attempts
			SET status=$5,finished_at=$6
			WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4
			  AND status=$7 AND expires_at<=$6
		`, step.JobID, step.Generation, step.ID, previousAttempt,
			model.StepAttemptExpired, databaseNow, model.StepAttemptActive)
		if err != nil {
			return nil, err
		}
		if result.RowsAffected() != 1 {
			return nil, staleStepAttemptError(model.StepAttemptAuthority{
				JobID: step.JobID, Generation: step.Generation, StepID: step.ID,
				Attempt: previousAttempt, WorkerID: step.WorkerID,
			}, "expired attempt lost reclaim authority", nil)
		}
	}
	attemptNumber := previousAttempt + 1
	var leaseExpiresAt time.Time
	if err := tx.QueryRow(ctx, `
		INSERT INTO job_step_attempts (
			job_id,generation,step_id,attempt,worker_id,claimed_at,renewed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$6)
		RETURNING expires_at
	`, step.JobID, step.Generation, step.ID, attemptNumber, workerID, databaseNow).Scan(&leaseExpiresAt); err != nil {
		return nil, fmt.Errorf("create step %d attempt %d: %w", step.ID, attemptNumber, err)
	}
	stepUpdate, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status=$2,worker_id=$3,current_attempt=$4,
		    started_at=COALESCE(started_at,$5),finished_at=NULL,updated_at=$5
		WHERE id=$1 AND status=$6 AND current_attempt=$7
	`, step.ID, model.StepStatusRunning, workerID, attemptNumber, databaseNow,
		step.Status, previousAttempt)
	if err != nil {
		return nil, err
	}
	if stepUpdate.RowsAffected() != 1 {
		return nil, fmt.Errorf("%w: step %d lost claim authority", ErrStepNotWritable, step.ID)
	}
	step.Status, step.WorkerID = model.StepStatusRunning, workerID
	if step.StartedAt == nil {
		startedAt := databaseNow
		step.StartedAt = &startedAt
	}
	priorJobStatus := job.Status
	jobUpdate, err := tx.Exec(ctx, `
		UPDATE jobs SET status=$2, updated_at=NOW()
		WHERE id=$1 AND status=$3
	`, job.ID, model.JobStatusRunning, model.JobStatusPending)
	if err != nil {
		return nil, err
	}
	if (priorJobStatus == model.JobStatusPending && jobUpdate.RowsAffected() != 1) ||
		(priorJobStatus == model.JobStatusRunning && jobUpdate.RowsAffected() != 0) {
		return nil, fmt.Errorf("%w: job %d changed during step claim", ErrStepNotWritable, job.ID)
	}
	job.Status = model.JobStatusRunning
	contexts, err := loadClaimedStepContexts(ctx, tx, job.ID, step.ID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	authority := model.StepAttemptAuthority{
		JobID: step.JobID, Generation: step.Generation, StepID: step.ID,
		Attempt: attemptNumber, WorkerID: workerID,
	}
	return &model.ClaimedStep{
		Job: job, Step: step, Authority: authority,
		LeaseExpiresAt: leaseExpiresAt, Contexts: contexts,
	}, nil
}
