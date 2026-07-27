package queue

import (
	"context"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CompleteStep(ctx context.Context, stepID int64, output, contextKey, contextValue string) error {
	output = SanitizeUTF8Text(output)
	contextKey = SanitizeUTF8Text(strings.TrimSpace(contextKey))
	contextValue = SanitizeUTF8Text(contextValue)
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var jobID int64
	if err := tx.QueryRow(ctx, `SELECT job_id FROM job_steps WHERE id = $1`, stepID).Scan(&jobID); err != nil {
		return err
	}

	var jobStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM jobs WHERE id = $1 FOR UPDATE`, jobID).Scan(&jobStatus); err != nil {
		return err
	}
	if jobStatus == model.JobStatusCanceled {
		return tx.Commit(ctx)
	}

	stepUpdate, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2, output = $3, finished_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status IN ($4, $5)
	`, stepID, model.StepStatusCompleted, output, model.StepStatusRunning, model.StepStatusWaiting)
	if err != nil {
		return err
	}
	if stepUpdate.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}

	if contextKey != "" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO step_contexts (step_id, key, value)
			VALUES ($1, $2, $3)
		`, stepID, contextKey, contextValue); err != nil {
			return err
		}
	}

	var openSteps int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_steps
		WHERE job_id = $1 AND status IN ($2, $3, $4)
	`, jobID, model.StepStatusPending, model.StepStatusRunning, model.StepStatusWaiting).Scan(&openSteps); err != nil {
		return err
	}

	if openSteps == 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE jobs
			SET status = $2, result = COALESCE(NULLIF($3, ''), result), completed_at = NOW(), updated_at = NOW()
			WHERE id = $1 AND status IN ($4, $5, $6)
		`, jobID, model.JobStatusCompleted, output, model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting); err != nil {
			return err
		}
		if err := completeTelemetryRunForJob(ctx, tx, jobID, model.JobStatusCompleted, map[string]any{"job_id": jobID, "result": output}, map[string]any{"terminal_step_id": stepID, "context_key": contextKey}); err != nil {
			return err
		}
		if err := recordTelemetryJobEvent(ctx, tx, jobID, "run_completed", map[string]any{"job_id": jobID, "step_id": stepID}); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE jobs
			SET updated_at = NOW()
			WHERE id = $1 AND status <> $2
		`, jobID, model.JobStatusCanceled); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *Repository) FailStep(ctx context.Context, stepID int64, errMsg string) error {
	errMsg = SanitizeUTF8Text(errMsg)
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var jobID int64
	if err := tx.QueryRow(ctx, `SELECT job_id FROM job_steps WHERE id = $1`, stepID).Scan(&jobID); err != nil {
		return err
	}

	var jobStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM jobs WHERE id = $1 FOR UPDATE`, jobID).Scan(&jobStatus); err != nil {
		return err
	}
	if jobStatus == model.JobStatusCanceled {
		return tx.Commit(ctx)
	}

	stepUpdate, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2, error = $3, finished_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status IN ($4, $5)
	`, stepID, model.StepStatusFailed, errMsg, model.StepStatusRunning, model.StepStatusWaiting)
	if err != nil {
		return err
	}
	if stepUpdate.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE jobs
		SET status = $2, error = $3, completed_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status IN ($4, $5, $6)
	`, jobID, model.JobStatusFailed, errMsg, model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting); err != nil {
		return err
	}
	if err := completeTelemetryRunForJob(ctx, tx, jobID, model.JobStatusFailed, map[string]any{"job_id": jobID, "error": errMsg}, map[string]any{"failed_step_id": stepID}); err != nil {
		return err
	}
	if err := recordTelemetryJobEvent(ctx, tx, jobID, "run_failed", map[string]any{"job_id": jobID, "step_id": stepID, "error": errMsg}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
