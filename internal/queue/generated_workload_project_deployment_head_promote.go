package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// sealGeneratedWorkloadProjectDeploymentHeadTx is the sole seal integration
// hook. Its caller must already have validated and sealed the exact deployment
// receipt in tx. Replaying an already-promoted exact head is a zero delta.
func sealGeneratedWorkloadProjectDeploymentHeadTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	receipt GeneratedWorkloadDeploymentReceipt,
) (GeneratedWorkloadProjectDeploymentHead, error) {
	if ctx == nil || tx == nil {
		return GeneratedWorkloadProjectDeploymentHead{}, fmt.Errorf(
			"promote project deployment head requires one active transaction",
		)
	}
	if err := validateGeneratedDeploymentExecutionAuthority(authority, command); err != nil {
		return GeneratedWorkloadProjectDeploymentHead{}, err
	}
	identity, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		return GeneratedWorkloadProjectDeploymentHead{}, err
	}
	if err := validateGeneratedWorkloadDeploymentReceipt(command, receipt, identity); err != nil {
		return GeneratedWorkloadProjectDeploymentHead{}, err
	}
	head, found, err := lockGeneratedWorkloadProjectDeploymentHeadTx(
		ctx, tx, command.Authority.ProjectID,
	)
	if err != nil {
		return GeneratedWorkloadProjectDeploymentHead{}, err
	}
	if found && head.ActiveDeploymentID == identity.OperationID {
		if head.Candidate != nil || head.ComposeProject != command.ComposeProject ||
			head.Endpoint == nil || head.Endpoint.Scheme != receipt.EndpointScheme ||
			head.Endpoint.Host != receipt.EndpointHost || head.Endpoint.Port != receipt.EndpointPort ||
			head.Endpoint.Path != receipt.EndpointPath {
			return GeneratedWorkloadProjectDeploymentHead{}, fmt.Errorf(
				"%w: replayed sealed deployment differs from the active project head",
				ErrGeneratedWorkloadProjectDeploymentHeadConflict,
			)
		}
		return head, nil
	}
	if !found || head.Candidate == nil ||
		head.Candidate.DeploymentID != identity.OperationID ||
		head.Candidate.Authority.JobID != authority.JobID ||
		head.Candidate.Authority.Generation != authority.Generation ||
		head.Candidate.Authority.StepID != authority.StepID ||
		head.Candidate.Executor.StepAttempt != authority.Attempt ||
		head.Candidate.Executor.WorkerID != authority.WorkerID ||
		head.ActiveDeploymentID != command.PriorDeploymentID ||
		head.ComposeProject != command.ComposeProject {
		return GeneratedWorkloadProjectDeploymentHead{}, fmt.Errorf(
			"%w: sealed deployment is not the exact fenced project candidate",
			ErrGeneratedWorkloadProjectDeploymentHeadConflict,
		)
	}
	reservation, err := generatedWorkloadProjectDeploymentReservation(head)
	if err != nil {
		return GeneratedWorkloadProjectDeploymentHead{}, err
	}
	return advanceGeneratedWorkloadProjectDeploymentHeadTx(
		ctx, tx, reservation, command, receipt,
	)
}

func advanceGeneratedWorkloadProjectDeploymentHeadTx(
	ctx context.Context,
	tx pgx.Tx,
	reservation GeneratedWorkloadProjectDeploymentReservation,
	command GeneratedWorkloadDeploymentCommand,
	receipt GeneratedWorkloadDeploymentReceipt,
) (GeneratedWorkloadProjectDeploymentHead, error) {
	if err := validateGeneratedWorkloadProjectDeploymentReservation(reservation); err != nil {
		return GeneratedWorkloadProjectDeploymentHead{}, err
	}
	identity, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		return GeneratedWorkloadProjectDeploymentHead{}, err
	}
	if reservation.ProjectID != command.Authority.ProjectID ||
		reservation.DeploymentID != identity.OperationID ||
		reservation.Authority != command.Authority {
		return GeneratedWorkloadProjectDeploymentHead{}, fmt.Errorf(
			"%w: reservation differs from deployment command authority",
			ErrGeneratedWorkloadProjectDeploymentHeadConflict,
		)
	}
	head, found, err := lockGeneratedWorkloadProjectDeploymentHeadTx(
		ctx, tx, reservation.ProjectID,
	)
	if err != nil {
		return GeneratedWorkloadProjectDeploymentHead{}, err
	}
	if !found || head.Revision != reservation.Revision || head.Fence != reservation.Fence ||
		head.Candidate == nil || head.Candidate.DeploymentID != reservation.DeploymentID ||
		head.Candidate.Authority != reservation.Authority ||
		head.Candidate.Executor != reservation.Executor {
		return GeneratedWorkloadProjectDeploymentHead{}, fmt.Errorf(
			"%w: project deployment candidate or fence is stale",
			ErrGeneratedWorkloadProjectDeploymentHeadConflict,
		)
	}
	var applied bool
	err = tx.QueryRow(ctx, `
		SELECT status='applied' AND project_id=$2 AND compose_project=$3 AND
		       prior_deployment_id IS NOT DISTINCT FROM NULLIF($4,'') AND
		       healthy_endpoint_port=$5 AND receipt_json::JSONB->>'endpoint_scheme'=$6 AND
		       receipt_json::JSONB->>'endpoint_host'=$7 AND
		       receipt_json::JSONB->>'endpoint_path'=$8
		FROM generated_workload_deployments WHERE id=$1 FOR KEY SHARE
	`, reservation.DeploymentID, reservation.ProjectID, command.ComposeProject,
		command.PriorDeploymentID, receipt.EndpointPort, receipt.EndpointScheme,
		receipt.EndpointHost, receipt.EndpointPath).Scan(&applied)
	if err != nil {
		return GeneratedWorkloadProjectDeploymentHead{}, fmt.Errorf(
			"inspect sealed project deployment candidate: %w", err,
		)
	}
	if !applied {
		return GeneratedWorkloadProjectDeploymentHead{}, fmt.Errorf(
			"%w: candidate has no exact sealed applied receipt",
			ErrGeneratedWorkloadProjectDeploymentHeadConflict,
		)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE generated_workload_project_deployment_heads
		SET active_deployment_id=$4,active_endpoint_scheme=$5,
		    active_endpoint_host=$6,active_endpoint_port=$7,active_endpoint_path=$8,
		    revision=revision+1,candidate_deployment_id=NULL,candidate_job_id=NULL,
		    candidate_generation=NULL,candidate_step_id=NULL,candidate_step_attempt=NULL,
		    candidate_worker_id=NULL,
		    updated_at=GREATEST(clock_timestamp(),updated_at+INTERVAL '1 microsecond')
		WHERE project_id=$1 AND revision=$2 AND fence=$3
		  AND candidate_deployment_id=$4
	`, reservation.ProjectID, reservation.Revision, reservation.Fence,
		reservation.DeploymentID, receipt.EndpointScheme, receipt.EndpointHost,
		receipt.EndpointPort, receipt.EndpointPath)
	if err != nil {
		return GeneratedWorkloadProjectDeploymentHead{}, fmt.Errorf(
			"promote project deployment head: %w", err,
		)
	}
	if tag.RowsAffected() != 1 {
		return GeneratedWorkloadProjectDeploymentHead{}, fmt.Errorf(
			"%w: project deployment head changed during promotion",
			ErrGeneratedWorkloadProjectDeploymentHeadConflict,
		)
	}
	return reloadGeneratedWorkloadProjectDeploymentHeadTx(
		ctx, tx, reservation.ProjectID,
	)
}
