package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func stepJobIDTx(ctx context.Context, tx pgx.Tx, stepID int64) (int64, error) {
	if stepID <= 0 {
		return 0, fmt.Errorf("step identity must be positive")
	}
	var jobID int64
	if err := tx.QueryRow(ctx, `
		SELECT job_id
		FROM job_steps
		WHERE id = $1
	`, stepID).Scan(&jobID); err != nil {
		return 0, err
	}
	return jobID, nil
}

func lockCurrentGenerationStep(ctx context.Context, tx pgx.Tx, jobID, stepID int64) (string, error) {
	if jobID <= 0 || stepID <= 0 {
		return "", fmt.Errorf("job and step identities must be positive")
	}
	var jobStatus string
	var currentGeneration int64
	if err := tx.QueryRow(ctx, `
		SELECT status, current_generation
		FROM jobs
		WHERE id = $1
		FOR UPDATE
	`, jobID).Scan(&jobStatus, &currentGeneration); err != nil {
		return "", err
	}
	var stepGeneration int64
	var supersededAt *int64
	if err := tx.QueryRow(ctx, `
		SELECT generation, superseded_at_generation
		FROM job_steps
		WHERE id = $1 AND job_id = $2
		FOR UPDATE
	`, stepID, jobID).Scan(&stepGeneration, &supersededAt); err != nil {
		return "", err
	}
	if supersededAt != nil || stepGeneration != currentGeneration {
		return "", fmt.Errorf(
			"%w: step %d generation %d is not job %d generation %d",
			ErrStaleJobGeneration, stepID, stepGeneration, jobID, currentGeneration,
		)
	}
	return jobStatus, nil
}

func lockCurrentGenerationStepByID(ctx context.Context, tx pgx.Tx, stepID int64) (int64, string, error) {
	jobID, err := stepJobIDTx(ctx, tx, stepID)
	if err != nil {
		return 0, "", err
	}
	status, err := lockCurrentGenerationStep(ctx, tx, jobID, stepID)
	if err != nil {
		return 0, "", err
	}
	return jobID, status, nil
}

func requireRunningCurrentStepTx(ctx context.Context, tx pgx.Tx, jobID, stepID int64) error {
	jobStatus, err := lockCurrentGenerationStep(ctx, tx, jobID, stepID)
	if err != nil {
		return err
	}
	if jobStatus != model.JobStatusRunning {
		return fmt.Errorf(
			"%w: job %d status is %q, expected %q",
			ErrStepNotWritable, jobID, jobStatus, model.JobStatusRunning,
		)
	}
	var stepStatus string
	if err := tx.QueryRow(ctx, `
		SELECT status FROM job_steps WHERE id=$1 AND job_id=$2
	`, stepID, jobID).Scan(&stepStatus); err != nil {
		return err
	}
	if stepStatus != model.StepStatusRunning {
		return fmt.Errorf("%w: step %d status is %q, expected %q", ErrStepNotWritable, stepID, stepStatus, model.StepStatusRunning)
	}
	return nil
}
