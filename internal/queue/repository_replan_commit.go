package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func createReplanGenerationTx(
	ctx context.Context,
	tx pgx.Tx,
	command ReplanJobCommand,
	feedbackSHA string,
	currentGeneration, newGeneration int64,
	boundary replanBoundary,
) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_generations (
			job_id, generation, predecessor_generation, purpose,
			boundary_action, feedback, feedback_sha256
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, command.JobID, newGeneration, currentGeneration, jobGenerationPurposeReplan,
		boundary.action, command.Feedback, feedbackSHA); err != nil {
		return fmt.Errorf("create generation %d for job %d: %w", newGeneration, command.JobID, err)
	}
	return nil
}

func advanceReplannedJobTx(
	ctx context.Context,
	tx pgx.Tx,
	command ReplanJobCommand,
	currentGeneration, newGeneration int64,
) (model.Job, error) {
	result, err := tx.Exec(ctx, `
		UPDATE jobs
		SET current_generation=$2, status=$3, result=NULL, error=NULL,
		    completed_at=NULL, updated_at=NOW()
		WHERE id=$1 AND current_generation=$4
		  AND status IN ($5, $6, $7)
	`, command.JobID, newGeneration, model.JobStatusRunning, currentGeneration,
		model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting)
	if err != nil {
		return model.Job{}, fmt.Errorf("advance job %d to generation %d: %w", command.JobID, newGeneration, err)
	}
	if result.RowsAffected() != 1 {
		return model.Job{}, fmt.Errorf("%w: job %d generation changed during replan", ErrStaleJobGeneration, command.JobID)
	}
	return scanJob(tx.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata,
		       current_generation, created_at, updated_at, completed_at
		FROM jobs WHERE id=$1
	`, command.JobID))
}
