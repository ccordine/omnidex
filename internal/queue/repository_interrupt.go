package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) InterruptJob(ctx context.Context, jobID int64, feedback string) (model.Job, error) {
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
	v3Authority, err := jobUsesV3AuthorityTx(ctx, tx, jobID)
	if err != nil {
		return model.Job{}, err
	}
	if v3Authority {
		successor, err := r.reviseV3AuthorityTx(ctx, tx, job, feedback, "interrupt")
		if err != nil {
			return model.Job{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return model.Job{}, err
		}
		return successor, nil
	}

	stepID, stepStatus, err := findInterruptStep(ctx, tx, jobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Job{}, fmt.Errorf("job has no available step for interruption")
		}
		return model.Job{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO step_contexts (step_id, key, value)
		VALUES ($1, $2, $3)
	`, stepID, "user_feedback", feedback); err != nil {
		return model.Job{}, err
	}

	switch stepStatus {
	case model.StepStatusRunning:
		if _, err := tx.Exec(ctx, `
			UPDATE job_steps
			SET status = $2, worker_id = NULL, output = NULL, started_at = NULL, updated_at = NOW(), error = NULL
			WHERE id = $1 AND status = $3
		`, stepID, model.StepStatusPending, model.StepStatusRunning); err != nil {
			return model.Job{}, err
		}
	case model.StepStatusWaiting:
		if _, err := tx.Exec(ctx, `
			UPDATE job_steps
			SET status = $2, output = $3, finished_at = NOW(), updated_at = NOW(), error = NULL
			WHERE id = $1 AND status = $4
		`, stepID, model.StepStatusCompleted, feedback, model.StepStatusWaiting); err != nil {
			return model.Job{}, err
		}
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
		if err := completeTelemetryRunForJob(ctx, tx, jobID, model.JobStatusCompleted, map[string]any{"job_id": jobID, "result": feedback}, map[string]any{"interrupt_step_id": stepID}); err != nil {
			return model.Job{}, err
		}
		if err := recordTelemetryJobEvent(ctx, tx, jobID, "run_completed", map[string]any{"job_id": jobID, "step_id": stepID, "source": "interrupt"}); err != nil {
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

func findInterruptStep(ctx context.Context, tx pgx.Tx, jobID int64) (int64, string, error) {
	statuses := []struct {
		status    string
		direction string
	}{
		{status: model.StepStatusRunning, direction: "ASC"},
		{status: model.StepStatusWaiting, direction: "ASC"},
		{status: model.StepStatusCompleted, direction: "DESC"},
		{status: model.StepStatusPending, direction: "ASC"},
	}

	for _, candidate := range statuses {
		var stepID int64
		var stepStatus string
		query := fmt.Sprintf(`
			SELECT id, status
			FROM job_steps
			WHERE job_id = $1 AND status = $2
			ORDER BY sort_index %s, id %s
			FOR UPDATE
			LIMIT 1
		`, candidate.direction, candidate.direction)
		err := tx.QueryRow(ctx, query, jobID, candidate.status).Scan(&stepID, &stepStatus)
		if err == nil {
			return stepID, stepStatus, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return 0, "", err
		}
	}

	return 0, "", pgx.ErrNoRows
}
