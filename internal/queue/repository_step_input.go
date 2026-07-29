package queue

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) PauseStepForInput(ctx context.Context, stepID int64, stepOutput string, question string, extraContexts map[string]string) error {
	stepOutput = SanitizeUTF8Text(stepOutput)
	question = SanitizeUTF8Text(question)
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
		SET status = $2, output = $3, updated_at = NOW()
		WHERE id = $1 AND status = $4
	`, stepID, model.StepStatusWaiting, stepOutput, model.StepStatusRunning)
	if err != nil {
		return err
	}
	if stepUpdate.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}

	if strings.TrimSpace(question) != "" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO step_contexts (step_id, key, value)
			VALUES ($1, $2, $3)
		`, stepID, "input_question", strings.TrimSpace(question)); err != nil {
			return err
		}
	}

	for key, value := range extraContexts {
		k := SanitizeUTF8Text(strings.TrimSpace(key))
		if k == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO step_contexts (step_id, key, value)
			VALUES ($1, $2, $3)
		`, stepID, k, SanitizeUTF8Text(value)); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE jobs
		SET status = $2, updated_at = NOW(), error = NULL
		WHERE id = $1 AND status IN ($3, $4)
	`, jobID, model.JobStatusWaiting, model.JobStatusPending, model.JobStatusRunning); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) SubmitJobFeedback(ctx context.Context, jobID int64, feedback string) (model.Job, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Job{}, err
	}
	defer tx.Rollback(ctx)

	feedback = SanitizeUTF8Text(strings.TrimSpace(feedback))
	if feedback == "" {
		return model.Job{}, fmt.Errorf("feedback is required")
	}

	job, err := lockedJobTx(ctx, tx, jobID)
	if err != nil {
		return model.Job{}, err
	}
	if job.Status == model.JobStatusCanceled || job.Status == model.JobStatusCompleted || job.Status == model.JobStatusFailed {
		return model.Job{}, fmt.Errorf("job is already %s", job.Status)
	}

	var stepID int64
	err = tx.QueryRow(ctx, `
		SELECT id
		FROM job_steps
		WHERE job_id = $1 AND status = $2
		ORDER BY sort_index ASC, id ASC
		FOR UPDATE
		LIMIT 1
	`, jobID, model.StepStatusWaiting).Scan(&stepID)
	if err != nil {
		return model.Job{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2, output = $3, finished_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, stepID, model.StepStatusCompleted, feedback); err != nil {
		return model.Job{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO step_contexts (step_id, key, value)
		VALUES ($1, $2, $3)
	`, stepID, "user_feedback", feedback); err != nil {
		return model.Job{}, err
	}

	var openSteps int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_steps
		WHERE job_id = $1 AND status IN ($2, $3, $4)
	`, jobID, model.StepStatusPending, model.StepStatusRunning, model.StepStatusWaiting).Scan(&openSteps); err != nil {
		return model.Job{}, err
	}

	if openSteps == 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE jobs
			SET status = $2, result = COALESCE(NULLIF($3, ''), result), completed_at = NOW(), updated_at = NOW(), error = NULL
			WHERE id = $1 AND status IN ($4, $5, $6)
		`, jobID, model.JobStatusCompleted, feedback, model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting); err != nil {
			return model.Job{}, err
		}
		if err := completeTelemetryRunForJob(ctx, tx, jobID, model.JobStatusCompleted, map[string]any{"job_id": jobID, "result": feedback}, map[string]any{"user_feedback_step_id": stepID}); err != nil {
			return model.Job{}, err
		}
		if err := recordTelemetryJobEvent(ctx, tx, jobID, "run_completed", map[string]any{"job_id": jobID, "step_id": stepID, "source": "user_feedback"}); err != nil {
			return model.Job{}, err
		}
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE jobs
			SET status = $2, updated_at = NOW(), error = NULL
			WHERE id = $1 AND status IN ($3, $4, $5)
		`, jobID, model.JobStatusRunning, model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting); err != nil {
			return model.Job{}, err
		}
	}

	row := tx.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata, created_at, updated_at, completed_at
		FROM jobs
		WHERE id = $1
	`, jobID)

	job, err = scanJob(row)
	if err != nil {
		return model.Job{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.Job{}, err
	}

	return job, nil
}
