package queue

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ReplanJob(ctx context.Context, jobID int64, feedback string) (model.Job, error) {
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
	var planStepID int64
	var planSortIndex int
	if err := tx.QueryRow(ctx, `
		SELECT id, sort_index
		FROM job_steps
		WHERE job_id = $1
		  AND action IN ('v3_coding', 'v3_subtask', 'v3_planning', 'plan')
		ORDER BY
		  CASE
		    WHEN status = 'running' THEN 0
		    WHEN status = 'waiting' THEN 1
		    WHEN status = 'completed' AND action IN ('v3_coding', 'v3_subtask') THEN 2
		    WHEN action IN ('v3_coding', 'v3_subtask') THEN 3
		    ELSE 4
		  END ASC,
		  CASE WHEN status = 'completed' THEN -sort_index ELSE sort_index END ASC,
		  id ASC
		FOR UPDATE
		LIMIT 1
	`, jobID).Scan(&planStepID, &planSortIndex); err != nil {
		return model.Job{}, fmt.Errorf("job has no coding or planning step to requeue: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO step_contexts (step_id, key, value)
		VALUES ($1, $2, $3)
	`, planStepID, "replan_feedback", feedback); err != nil {
		return model.Job{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO step_contexts (step_id, key, value)
		VALUES ($1, $2, $3)
	`, planStepID, "user_feedback", feedback); err != nil {
		return model.Job{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2,
		    worker_id = NULL,
		    output = NULL,
		    error = NULL,
		    started_at = NULL,
		    finished_at = NULL,
		    updated_at = NOW()
		WHERE job_id = $1
		  AND sort_index >= $3
	`, jobID, model.StepStatusPending, planSortIndex); err != nil {
		return model.Job{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE jobs
		SET status = $2,
		    result = NULL,
		    error = NULL,
		    completed_at = NULL,
		    updated_at = NOW()
		WHERE id = $1
	`, jobID, model.JobStatusRunning); err != nil {
		return model.Job{}, err
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
