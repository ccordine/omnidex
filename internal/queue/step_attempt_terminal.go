package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func terminalizeStepAttemptsForAuthorityChangeTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation int64,
	stepIDs []int64,
	terminal model.StepAttemptStatus,
) error {
	if jobID <= 0 || generation <= 0 || len(stepIDs) == 0 {
		return fmt.Errorf("terminalize step attempts: exact job, generation, and steps are required")
	}
	if terminal != model.StepAttemptCanceled && terminal != model.StepAttemptSuperseded {
		return fmt.Errorf("terminalize step attempts: status %q is not an authority-change terminal", terminal)
	}
	rows, err := tx.Query(ctx, `
		SELECT id,status,current_attempt,COALESCE(worker_id,'')
		FROM job_steps
		WHERE job_id=$1 AND generation=$2 AND id=ANY($3::bigint[])
		ORDER BY id FOR UPDATE
	`, jobID, generation, stepIDs)
	if err != nil {
		return err
	}
	type stepState struct {
		id, attempt    int64
		status, worker string
	}
	states := make([]stepState, 0, len(stepIDs))
	for rows.Next() {
		var state stepState
		if err := rows.Scan(&state.id, &state.status, &state.attempt, &state.worker); err != nil {
			rows.Close()
			return err
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(states) != len(stepIDs) {
		return fmt.Errorf("%w: job %d generation %d step set changed", ErrStaleJobGeneration, jobID, generation)
	}
	for _, state := range states {
		if state.status != model.StepStatusRunning {
			continue
		}
		if state.attempt <= 0 || state.worker == "" {
			return fmt.Errorf("%w: running step %d has no attempt authority", ErrStaleStepAttempt, state.id)
		}
		result, err := tx.Exec(ctx, `
			UPDATE job_step_attempts
			SET status=$5,finished_at=clock_timestamp()
			WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4
			  AND worker_id=$6 AND status=$7
		`, jobID, generation, state.id, state.attempt, terminal, state.worker,
			model.StepAttemptActive)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return fmt.Errorf("%w: running step %d active attempt changed", ErrStaleStepAttempt, state.id)
		}
	}
	return nil
}

func lockCurrentNonterminalStepIDsTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation int64,
) ([]int64, error) {
	rows, err := tx.Query(ctx, `
		SELECT id FROM job_steps
		WHERE job_id=$1 AND generation=$2 AND superseded_at_generation IS NULL
		  AND status IN ($3,$4,$5)
		ORDER BY id FOR UPDATE
	`, jobID, generation, model.StepStatusPending, model.StepStatusRunning, model.StepStatusWaiting)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0, 16)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
