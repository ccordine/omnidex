package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// releaseGeneratedWorkloadProjectDeploymentCandidateTx is deliberately not an
// exported standalone operation. A candidate may be released only by the same
// transaction that commits its failed or cleanly rolled-back terminal state.
func releaseGeneratedWorkloadProjectDeploymentCandidateTx(
	ctx context.Context,
	tx pgx.Tx,
	command GeneratedWorkloadDeploymentCommand,
	operationID string,
	executor GeneratedWorkloadDeploymentExecutor,
	requireCandidate bool,
) error {
	head, found, err := lockGeneratedWorkloadProjectDeploymentHeadTx(
		ctx, tx, command.Authority.ProjectID,
	)
	if err != nil {
		return err
	}
	if !found {
		if requireCandidate {
			return fmt.Errorf(
				"%w: terminal deployment has no project head",
				ErrGeneratedWorkloadProjectDeploymentHeadConflict,
			)
		}
		return nil
	}
	if head.Candidate == nil {
		if requireCandidate {
			return fmt.Errorf(
				"%w: terminal deployment has no exact project candidate",
				ErrGeneratedWorkloadProjectDeploymentHeadConflict,
			)
		}
		return nil
	}
	if head.Candidate.DeploymentID != operationID ||
		head.Candidate.Authority != command.Authority ||
		head.Candidate.Executor != executor {
		return fmt.Errorf(
			"%w: terminal release candidate authority differs",
			ErrGeneratedWorkloadProjectDeploymentHeadConflict,
		)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE generated_workload_project_deployment_heads
		SET fence=fence+1,candidate_deployment_id=NULL,candidate_job_id=NULL,
		    candidate_generation=NULL,candidate_step_id=NULL,
		    candidate_step_attempt=NULL,candidate_worker_id=NULL,
		    updated_at=GREATEST(clock_timestamp(),updated_at+INTERVAL '1 microsecond')
		WHERE project_id=$1 AND revision=$2 AND fence=$3
		  AND candidate_deployment_id=$4 AND candidate_step_attempt=$5
		  AND candidate_worker_id=$6
	`, head.ProjectID, head.Revision, head.Fence, operationID,
		executor.StepAttempt, executor.WorkerID)
	if err != nil {
		return fmt.Errorf("release terminal project deployment candidate: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf(
			"%w: terminal project candidate changed during release",
			ErrGeneratedWorkloadProjectDeploymentHeadConflict,
		)
	}
	return nil
}
