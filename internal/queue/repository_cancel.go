package queue

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CancelJob(ctx context.Context, jobID int64, reason string) (model.Job, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Job{}, err
	}
	defer rollbackTx(ctx, tx, "job cancellation")

	job, err := cancelJobTx(ctx, tx, jobID, reason)
	if err != nil {
		return model.Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Job{}, err
	}
	return job, nil
}

func cancelJobTx(ctx context.Context, tx pgx.Tx, jobID int64, reason string) (model.Job, error) {
	reason = SanitizeUTF8Text(strings.TrimSpace(reason))
	if reason == "" {
		reason = "canceled by user"
	}

	row := tx.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata, created_at, updated_at, completed_at
		FROM jobs
		WHERE id = $1
		FOR UPDATE
	`, jobID)

	job, err := scanJob(row)
	if err != nil {
		return model.Job{}, err
	}

	switch job.Status {
	case model.JobStatusCompleted, model.JobStatusFailed:
		return model.Job{}, fmt.Errorf("job is already %s", job.Status)
	case model.JobStatusCanceled:
		return job, nil
	}

	if _, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2,
		    error = COALESCE(NULLIF(error, ''), $3),
		    finished_at = COALESCE(finished_at, NOW()),
		    updated_at = NOW()
		WHERE job_id = $1
		  AND status IN ($4, $5, $6)
	`, jobID, model.StepStatusCanceled, reason, model.StepStatusPending, model.StepStatusRunning, model.StepStatusWaiting); err != nil {
		return model.Job{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE jobs
		SET status = $2, error = $3, completed_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, jobID, model.JobStatusCanceled, reason); err != nil {
		return model.Job{}, err
	}
	if err := completeTelemetryRunForJob(ctx, tx, jobID, model.JobStatusCanceled, map[string]any{"job_id": jobID, "error": reason}, map[string]any{"cancel_reason": reason}); err != nil {
		return model.Job{}, err
	}
	if err := recordTelemetryJobEvent(ctx, tx, jobID, "run_cancelled", map[string]any{"job_id": jobID, "reason": reason}); err != nil {
		return model.Job{}, err
	}

	row = tx.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata, created_at, updated_at, completed_at
		FROM jobs
		WHERE id = $1
	`, jobID)

	job, err = scanJob(row)
	if err != nil {
		return model.Job{}, err
	}

	return job, nil
}
