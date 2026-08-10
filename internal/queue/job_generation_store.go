package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func lockCurrentReplanTailTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID int64,
	boundarySort int,
) ([]replanStepRecord, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, action, sort_index, status, generation
		FROM job_steps
		WHERE job_id=$1 AND sort_index >= $2
		  AND superseded_at_generation IS NULL
		ORDER BY sort_index ASC, id ASC
		FOR UPDATE
	`, jobID, boundarySort)
	if err != nil {
		return nil, fmt.Errorf("lock current replan tail for job %d: %w", jobID, err)
	}
	defer rows.Close()
	records := make([]replanStepRecord, 0, 16)
	for rows.Next() {
		var record replanStepRecord
		if err := rows.Scan(
			&record.ID, &record.Action, &record.SortIndex, &record.Status, &record.Generation,
		); err != nil {
			return nil, fmt.Errorf("scan current replan tail for job %d: %w", jobID, err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read current replan tail for job %d: %w", jobID, err)
	}
	return records, nil
}

func rejectAssignedRetiringStepsTx(ctx context.Context, tx pgx.Tx, jobID int64, stepIDs []int64) error {
	if len(stepIDs) == 0 {
		return fmt.Errorf("%w: replan tail has no retiring steps", ErrInvalidJobGeneration)
	}
	var nodeID string
	var stepID int64
	err := tx.QueryRow(ctx, `
		SELECT id, assigned_step_id
		FROM task_nodes
		WHERE job_id=$1 AND assigned_step_id=ANY($2::bigint[])
		ORDER BY id ASC
		LIMIT 1
	`, jobID, stepIDs).Scan(&nodeID, &stepID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check task assignments for job %d replan: %w", jobID, err)
	}
	return fmt.Errorf(
		"%w: task node %q remains assigned to retiring step %d",
		ErrInvalidJobGeneration, nodeID, stepID,
	)
}

func retireReplanTailTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID int64,
	stepIDs []int64,
	newGeneration int64,
) error {
	result, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status=CASE
		      WHEN status IN ($4, $5, $6) THEN $7
		      ELSE status
		    END,
		    worker_id=CASE
		      WHEN status IN ($4, $5, $6) THEN NULL
		      ELSE worker_id
		    END,
		    finished_at=CASE
		      WHEN status IN ($4, $5, $6) THEN COALESCE(finished_at, NOW())
		      ELSE finished_at
		    END,
		    superseded_at_generation=$3,
		    updated_at=NOW()
		WHERE job_id=$1 AND id=ANY($2::bigint[])
		  AND superseded_at_generation IS NULL
	`, jobID, stepIDs, newGeneration,
		model.StepStatusPending, model.StepStatusRunning, model.StepStatusWaiting,
		model.StepStatusCanceled)
	if err != nil {
		return fmt.Errorf("supersede job %d replan tail: %w", jobID, err)
	}
	if result.RowsAffected() != int64(len(stepIDs)) {
		return fmt.Errorf("%w: job %d replan tail changed before supersession", ErrStaleJobGeneration, jobID)
	}
	return nil
}

func insertReplanTailTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation int64,
	seeds []stepSeed,
) error {
	if len(seeds) == 0 {
		return fmt.Errorf("%w: new replan tail is empty", ErrInvalidJobGeneration)
	}
	for _, seed := range seeds {
		if _, err := tx.Exec(ctx, `
			INSERT INTO job_steps (job_id, action, sort_index, status, generation)
			VALUES ($1, $2, $3, $4, $5)
		`, jobID, seed.action, seed.sortIndex, model.StepStatusPending, generation); err != nil {
			return fmt.Errorf(
				"insert generation %d step %s@%d for job %d: %w",
				generation, seed.action, seed.sortIndex, jobID, err,
			)
		}
	}
	return nil
}
