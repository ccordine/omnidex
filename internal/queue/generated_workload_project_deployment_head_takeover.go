package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func takeOverGeneratedWorkloadDeploymentExecutorTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	operationID string,
	record GeneratedWorkloadDeploymentRecord,
	head GeneratedWorkloadProjectDeploymentHead,
	headFound bool,
) error {
	if record.Current.StepAttempt == authority.Attempt &&
		record.Current.WorkerID == authority.WorkerID {
		return nil
	}
	if authority.Attempt <= record.Current.StepAttempt {
		return fmt.Errorf(
			"%w: project deployment takeover attempt is not newer",
			ErrGeneratedWorkloadProjectDeploymentHeadConflict,
		)
	}
	if record.State != GeneratedWorkloadDeploymentPrepared &&
		record.State != GeneratedWorkloadDeploymentApplying &&
		record.State != GeneratedWorkloadDeploymentIndeterminate {
		return fmt.Errorf(
			"%w: deployment %s cannot be taken over from %s",
			ErrGeneratedWorkloadProjectDeploymentHeadConflict, operationID, record.State,
		)
	}
	if headFound && head.Candidate != nil {
		if head.Candidate.DeploymentID != operationID ||
			head.Candidate.Executor != record.Current {
			return fmt.Errorf(
				"%w: project deployment candidate differs from current executor",
				ErrGeneratedWorkloadProjectDeploymentHeadConflict,
			)
		}
	} else if record.State != GeneratedWorkloadDeploymentPrepared {
		return fmt.Errorf(
			"%w: active deployment takeover has no exact project candidate",
			ErrGeneratedWorkloadProjectDeploymentHeadConflict,
		)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE generated_workload_deployments
		SET current_step_attempt=$2,current_worker_id=$3,
		    updated_at=GREATEST(clock_timestamp(),updated_at+INTERVAL '1 microsecond')
		WHERE id=$1 AND current_step_attempt=$4 AND current_worker_id=$5
	`, operationID, authority.Attempt, authority.WorkerID,
		record.Current.StepAttempt, record.Current.WorkerID)
	if err != nil {
		return fmt.Errorf("take over generated deployment executor: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf(
			"%w: generated deployment executor changed during takeover",
			ErrGeneratedWorkloadProjectDeploymentHeadConflict,
		)
	}
	return nil
}
