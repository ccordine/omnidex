package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func createInterruptGenerationTx(
	ctx context.Context,
	tx pgx.Tx,
	command ReplanJobCommand,
	feedbackSHA string,
	currentGeneration int64,
	newGeneration int64,
	boundary replanBoundary,
) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_generations (
			job_id, generation, predecessor_generation, purpose,
			boundary_action, feedback, feedback_sha256
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, command.JobID, newGeneration, currentGeneration, jobGenerationPurposeInterrupt,
		boundary.action, command.Feedback, feedbackSHA); err != nil {
		return fmt.Errorf(
			"create interrupted generation %d for job %d: %w",
			newGeneration,
			command.JobID,
			err,
		)
	}
	return nil
}

func insertInterruptedTailTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID int64,
	generation int64,
	boundary replanBoundary,
) error {
	if len(boundary.seeds) == 0 {
		return fmt.Errorf("%w: new interrupted tail is empty", ErrInvalidJobGeneration)
	}
	for index, seed := range boundary.seeds {
		status := model.StepStatusPending
		if index == 0 {
			if seed.action != boundary.action || seed.sortIndex != boundary.sortIndex {
				return fmt.Errorf("%w: interrupted boundary seed is not canonical", ErrInvalidJobGeneration)
			}
			status = model.StepStatusWaiting
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO job_steps (job_id, action, sort_index, status, generation)
			VALUES ($1, $2, $3, $4, $5)
		`, jobID, seed.action, seed.sortIndex, status, generation); err != nil {
			return fmt.Errorf(
				"insert interrupted generation %d step %s@%d for job %d: %w",
				generation,
				seed.action,
				seed.sortIndex,
				jobID,
				err,
			)
		}
	}
	return nil
}

func advanceInterruptedJobTx(
	ctx context.Context,
	tx pgx.Tx,
	command ReplanJobCommand,
	currentGeneration int64,
	newGeneration int64,
) (model.Job, error) {
	result, err := tx.Exec(ctx, `
		UPDATE jobs
		SET current_generation=$2, status=$3, result=NULL, error=NULL,
		    completed_at=NULL, updated_at=NOW()
		WHERE id=$1 AND current_generation=$4
		  AND status IN ($5, $6)
	`, command.JobID, newGeneration, model.JobStatusWaiting, currentGeneration,
		model.JobStatusPending, model.JobStatusRunning)
	if err != nil {
		return model.Job{}, fmt.Errorf(
			"advance job %d to interrupted generation %d: %w",
			command.JobID,
			newGeneration,
			err,
		)
	}
	if result.RowsAffected() != 1 {
		return model.Job{}, fmt.Errorf(
			"%w: job %d generation changed during interruption",
			ErrStaleJobGeneration,
			command.JobID,
		)
	}
	return scanJob(tx.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata,
		       current_generation, created_at, updated_at, completed_at
		FROM jobs WHERE id=$1
	`, command.JobID))
}
