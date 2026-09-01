package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// transitionCodingPlanAuthorityTx retires the current generation's persisted
// plan under the same transaction that retires or cancels its job authority.
// A generation that has not reached plan review has no plan to transition.
func transitionCodingPlanAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	job model.Job,
	target model.CodingPlanState,
) error {
	if job.Pipeline != model.PipelineCoding && job.Pipeline != model.PipelineChat &&
		job.Pipeline != model.PipelineScrum {
		return nil
	}
	if target != model.CodingPlanStateSuperseded && target != model.CodingPlanStateCanceled {
		return fmt.Errorf("coding plan lifecycle target %q is unsupported", target)
	}

	var current model.CodingPlanState
	err := tx.QueryRow(ctx, `
		SELECT state
		FROM coding_plans
		WHERE job_id=$1 AND generation=$2
		FOR UPDATE
	`, job.ID, job.CurrentGeneration).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock coding plan for job %d: %w", job.ID, err)
	}
	if current != model.CodingPlanStateReview && current != model.CodingPlanStateFrozen {
		return fmt.Errorf(
			"%w: current coding plan for job %d is already %q",
			ErrCodingPlanState,
			job.ID,
			current,
		)
	}
	result, err := tx.Exec(ctx, `
		UPDATE coding_plans
		SET state=$3,revision=revision+1,updated_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND state=$4
	`, job.ID, job.CurrentGeneration, target, current)
	if err != nil {
		return fmt.Errorf("transition coding plan for job %d to %q: %w", job.ID, target, err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("%w: coding plan for job %d changed during lifecycle transition", ErrCodingPlanState, job.ID)
	}
	return nil
}
