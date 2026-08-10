package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func lockActiveStepAttemptsForLifecycleTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation int64,
	stepIDs []int64,
) error {
	if jobID <= 0 || generation <= 0 || len(stepIDs) == 0 {
		return fmt.Errorf("cognition lifecycle attempt lock requires exact job, generation, and steps")
	}
	var expected int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM job_steps
		WHERE job_id=$1 AND generation=$2 AND id=ANY($3::bigint[]) AND status=$4
	`, jobID, generation, stepIDs, model.StepStatusRunning).Scan(&expected); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `
		SELECT steps.id,steps.status,steps.current_attempt,COALESCE(steps.worker_id,''),
		       attempts.status
		FROM job_steps steps
		JOIN job_step_attempts attempts
		  ON attempts.job_id=steps.job_id AND attempts.generation=steps.generation
		 AND attempts.step_id=steps.id AND attempts.attempt=steps.current_attempt
		 AND attempts.worker_id=steps.worker_id
		WHERE steps.job_id=$1 AND steps.generation=$2 AND steps.id=ANY($3::bigint[])
		  AND steps.status=$4
		ORDER BY steps.id FOR UPDATE OF attempts
	`, jobID, generation, stepIDs, model.StepStatusRunning)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var stepID, attempt int64
		var stepStatus, worker string
		var attemptStatus *model.StepAttemptStatus
		if err := rows.Scan(&stepID, &stepStatus, &attempt, &worker, &attemptStatus); err != nil {
			return err
		}
		count++
		if stepStatus != model.StepStatusRunning {
			continue
		}
		if attempt <= 0 || worker == "" || attemptStatus == nil || *attemptStatus != model.StepAttemptActive {
			return fmt.Errorf("%w: running step %d has no exact active attempt", ErrStaleStepAttempt, stepID)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != expected {
		return fmt.Errorf("%w: cognition lifecycle active attempt set changed", ErrStaleStepAttempt)
	}
	return nil
}
