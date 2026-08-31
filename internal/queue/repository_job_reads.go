package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ListJobs(ctx context.Context, status string, limit, offset int) ([]model.Job, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("job list limit must be positive")
	}
	if offset < 0 {
		return nil, fmt.Errorf("job list offset must be non-negative")
	}

	args := []any{}
	query := `
		SELECT id, instruction, pipeline, status, result, error, metadata, current_generation,
		       created_at, updated_at, completed_at
		FROM jobs
	`

	if status != "" {
		query += ` WHERE status = $1`
		args = append(args, status)
	}

	query += fmt.Sprintf(" ORDER BY id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]model.Job, 0, limit)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return jobs, nil
}

func (r *Repository) CurrentJobDetails(ctx context.Context, jobID int64) (model.JobDetails, error) {
	if jobID <= 0 {
		return model.JobDetails{}, fmt.Errorf("current job details require a positive job ID")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return model.JobDetails{}, err
	}
	defer tx.Rollback(ctx)
	row := tx.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata, current_generation,
		       created_at, updated_at, completed_at
		FROM jobs
		WHERE id = $1
	`, jobID)

	job, err := scanJob(row)
	if err != nil {
		return model.JobDetails{}, err
	}

	stepsRows, err := tx.Query(ctx, `
		SELECT id, job_id, action, sort_index, status, generation, superseded_at_generation,
		       worker_id, output, error, started_at, finished_at, created_at, updated_at
		FROM job_steps
		WHERE job_id = $1
		  AND superseded_at_generation IS NULL
		ORDER BY sort_index ASC, id ASC
	`, jobID)
	if err != nil {
		return model.JobDetails{}, err
	}
	steps := []model.Step{}
	for stepsRows.Next() {
		step, err := scanStep(stepsRows)
		if err != nil {
			stepsRows.Close()
			return model.JobDetails{}, err
		}
		steps = append(steps, step)
	}
	if err := stepsRows.Err(); err != nil {
		stepsRows.Close()
		return model.JobDetails{}, err
	}
	stepsRows.Close()

	if err := tx.Commit(ctx); err != nil {
		return model.JobDetails{}, err
	}
	return model.JobDetails{Job: job, Steps: steps}, nil
}

func (r *Repository) GetStepRuntimeState(ctx context.Context, jobID, stepID int64) (string, string, error) {
	var jobStatus string
	var stepStatus string
	err := r.pool.QueryRow(ctx, `
		SELECT j.status, s.status
		FROM jobs j
		JOIN job_steps s ON s.job_id = j.id
		WHERE j.id = $1 AND s.id = $2
	`, jobID, stepID).Scan(&jobStatus, &stepStatus)
	if err != nil {
		return "", "", err
	}
	return jobStatus, stepStatus, nil
}
