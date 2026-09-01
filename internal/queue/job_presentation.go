package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const (
	maxCurrentJobPresentationSteps = 128
)

var ErrInvalidJobPresentation = errors.New("invalid current job presentation")

type JobPresentation struct {
	Job   model.Job
	Steps []model.Step
}

// CurrentJobPresentation reads the bounded, current-generation state needed by
// the server-rendered chat UI from one repeatable-read snapshot. It deliberately
// excludes prompts, responses, command streams, diffs, workspace snapshots,
// and every other raw context class from the presentation projection.
func (r *Repository) CurrentJobPresentation(ctx context.Context, jobID int64) (JobPresentation, error) {
	if jobID <= 0 {
		return JobPresentation{}, fmt.Errorf("%w: positive job ID is required", ErrInvalidJobPresentation)
	}
	if r == nil || r.pool == nil {
		return JobPresentation{}, fmt.Errorf("%w: PostgreSQL repository is required", ErrInvalidJobPresentation)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return JobPresentation{}, fmt.Errorf("begin job %d presentation read: %w", jobID, err)
	}
	defer tx.Rollback(ctx)

	job, err := scanJob(tx.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata, current_generation,
		       created_at, updated_at, completed_at
		FROM jobs
		WHERE id=$1
	`, jobID))
	if err != nil {
		return JobPresentation{}, err
	}
	if job.CurrentGeneration <= 0 {
		return JobPresentation{}, fmt.Errorf(
			"%w: job %d has invalid generation %d",
			ErrInvalidJobPresentation, job.ID, job.CurrentGeneration,
		)
	}

	steps, err := readCurrentPresentationSteps(ctx, tx, job)
	if err != nil {
		return JobPresentation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return JobPresentation{}, fmt.Errorf("commit job %d presentation read: %w", jobID, err)
	}
	return JobPresentation{Job: job, Steps: steps}, nil
}

func readCurrentPresentationSteps(ctx context.Context, tx pgx.Tx, job model.Job) ([]model.Step, error) {
	rows, err := tx.Query(ctx, `
		SELECT s.id, s.job_id, s.action, s.sort_index, s.status, s.generation,
		       s.superseded_at_generation, s.worker_id, s.output, s.error,
		       s.started_at, s.finished_at, s.created_at, s.updated_at
		FROM job_steps AS s
		JOIN jobs AS j ON j.id=s.job_id
		WHERE j.id=$1
		  AND s.generation = j.current_generation
		  AND s.superseded_at_generation IS NULL
		ORDER BY s.sort_index ASC, s.id ASC
		LIMIT $2
	`, job.ID, maxCurrentJobPresentationSteps+1)
	if err != nil {
		return nil, fmt.Errorf("read job %d presentation steps: %w", job.ID, err)
	}
	defer rows.Close()
	steps := make([]model.Step, 0, 8)
	for rows.Next() {
		step, scanErr := scanStep(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan job %d presentation step: %w", job.ID, scanErr)
		}
		steps = append(steps, step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate job %d presentation steps: %w", job.ID, err)
	}
	if len(steps) > maxCurrentJobPresentationSteps {
		return nil, fmt.Errorf(
			"%w: job %d has more than %d current steps",
			ErrInvalidJobPresentation, job.ID, maxCurrentJobPresentationSteps,
		)
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("%w: job %d has no current steps", ErrInvalidJobPresentation, job.ID)
	}
	return steps, nil
}
