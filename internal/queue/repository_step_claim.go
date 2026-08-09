package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ClaimNextStep(ctx context.Context, workerID string) (*model.ClaimedStep, error) {
	if workerID == "" || workerID != strings.TrimSpace(workerID) || len(workerID) > 256 ||
		!utf8.ValidString(workerID) || strings.ContainsRune(workerID, '\x00') {
		return nil, fmt.Errorf("worker identity must be exact PostgreSQL-compatible text of at most 256 bytes")
	}
	if paused, err := r.IsAIPaused(ctx); err != nil {
		return nil, err
	} else if paused {
		return nil, nil
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var candidateJobID int64
	err = tx.QueryRow(ctx, `
		SELECT jobs.id
		FROM jobs
		WHERE jobs.status IN ($1, $2)
		  AND EXISTS (
		      SELECT 1 FROM job_steps AS candidate
		      WHERE candidate.job_id=jobs.id AND candidate.status=$3
		        AND candidate.superseded_at_generation IS NULL
		        AND candidate.generation=jobs.current_generation
		        AND NOT EXISTS (
		            SELECT 1 FROM job_steps AS predecessor
		            WHERE predecessor.job_id=candidate.job_id
		              AND predecessor.superseded_at_generation IS NULL
		              AND predecessor.sort_index<candidate.sort_index
		              AND predecessor.status<>$4
		        )
		  )
		ORDER BY jobs.id ASC
		FOR UPDATE OF jobs SKIP LOCKED
		LIMIT 1
	`, model.JobStatusPending, model.JobStatusRunning, model.StepStatusPending,
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
		WHERE s.job_id=$1 AND s.status=$2
		  AND s.superseded_at_generation IS NULL
		  AND s.generation=j.current_generation
		  AND j.status IN ($3, $4)
		  AND NOT EXISTS (
		      SELECT 1 FROM job_steps AS predecessor
		      WHERE predecessor.job_id=s.job_id
		        AND predecessor.superseded_at_generation IS NULL
		        AND predecessor.sort_index<s.sort_index
		        AND predecessor.status<>$5
		  )
		ORDER BY s.sort_index ASC, s.id ASC
		FOR UPDATE OF s SKIP LOCKED
		LIMIT 1
	`, candidateJobID, model.StepStatusPending, model.JobStatusPending, model.JobStatusRunning,
		model.StepStatusCompleted))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	stepUpdate, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status=$2, worker_id=$3, started_at=NOW(), updated_at=NOW()
		WHERE id=$1 AND status=$4
	`, step.ID, model.StepStatusRunning, workerID, model.StepStatusPending)
	if err != nil {
		return nil, err
	}
	if stepUpdate.RowsAffected() != 1 {
		return nil, fmt.Errorf("%w: step %d lost pending claim authority", ErrStepNotWritable, step.ID)
	}
	step.Status, step.WorkerID = model.StepStatusRunning, workerID
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
	if err := activateInitialTaskRootTx(ctx, tx, job.ID, job.CurrentGeneration); err != nil {
		return nil, err
	}
	if err := markTelemetryRunRunningForJob(ctx, tx, job.ID); err != nil {
		return nil, err
	}
	if err := recordTelemetryJobEvent(ctx, tx, job.ID, "run_running", map[string]any{
		"job_id": job.ID, "step_id": step.ID, "action": step.Action,
	}); err != nil {
		return nil, err
	}
	contexts, err := loadClaimedStepContexts(ctx, tx, job.ID, step.ID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &model.ClaimedStep{Job: job, Step: step, Contexts: contexts}, nil
}
