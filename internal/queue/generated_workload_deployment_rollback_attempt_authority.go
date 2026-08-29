package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func requireGeneratedDeploymentRollbackMutationTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	identity generatedWorkloadDeploymentIdentity,
	plan GeneratedWorkloadDeploymentRollbackPlan,
) error {
	deployment, err := lockGeneratedDeploymentTx(ctx, tx, identity.OperationID)
	if err != nil {
		return err
	}
	if err := requireGeneratedDeploymentIdentity(deployment, identity); err != nil {
		return err
	}
	if deployment.State != GeneratedWorkloadDeploymentApplying &&
		deployment.State != GeneratedWorkloadDeploymentIndeterminate {
		return fmt.Errorf(
			"%w: deployment rollback attempt cannot mutate from %s",
			ErrGeneratedWorkloadDeploymentState, deployment.State,
		)
	}
	if deployment.Current.StepAttempt != authority.Attempt ||
		deployment.Current.WorkerID != authority.WorkerID {
		return staleStepAttemptError(authority, "deployment rollback executor authority changed", nil)
	}
	durablePlan, found, err := loadGeneratedDeploymentRollbackPlanTx(
		ctx, tx, identity.OperationID, true,
	)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("deployment rollback plan is unavailable")
	}
	if err := validateGeneratedWorkloadDeploymentRollbackPlan(command, durablePlan); err != nil {
		return fmt.Errorf("validate durable deployment rollback plan: %w", err)
	}
	if !equalGeneratedWorkloadDeploymentRollbackPlans(durablePlan, plan) {
		return fmt.Errorf("%w: deployment rollback plan differs", ErrGeneratedWorkloadDeploymentConflict)
	}
	var initialStartExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
		 SELECT 1 FROM generated_workload_deployment_executions
		 WHERE operation_id=$1 AND slot_name='initial_start' AND slot_ordinal=30
		)
	`, identity.OperationID).Scan(&initialStartExists); err != nil {
		return fmt.Errorf("inspect deployment initial_start ownership: %w", err)
	}
	if !initialStartExists {
		return fmt.Errorf(
			"%w: destructive deployment rollback requires a durable initial_start execution",
			ErrGeneratedWorkloadDeploymentState,
		)
	}
	head, found, err := lockGeneratedWorkloadProjectDeploymentHeadTx(
		ctx, tx, command.Authority.ProjectID,
	)
	if err != nil {
		return err
	}
	if !found || head.Candidate == nil {
		return fmt.Errorf("%w: deployment rollback has no reserved candidate", ErrGeneratedWorkloadProjectDeploymentHeadConflict)
	}
	candidate := head.Candidate
	if candidate.DeploymentID != identity.OperationID ||
		candidate.Authority != command.Authority ||
		candidate.Executor.StepAttempt != authority.Attempt ||
		candidate.Executor.WorkerID != authority.WorkerID {
		return fmt.Errorf("%w: deployment rollback candidate authority differs", ErrGeneratedWorkloadProjectDeploymentHeadConflict)
	}
	return nil
}

func requireGeneratedDeploymentRollbackAttemptIdentity(
	record GeneratedWorkloadDeploymentRollbackAttemptRecord,
	authority model.StepAttemptAuthority,
	plan GeneratedWorkloadDeploymentRollbackPlan,
) error {
	if record.StepAttempt != authority.Attempt || record.WorkerID != authority.WorkerID ||
		record.CommandSHA256 != plan.Execution.CommandSHA256 ||
		record.WorkspaceSHA256 != plan.Execution.WorkspaceSHA256 {
		return fmt.Errorf("%w: deployment rollback attempt identity differs", ErrGeneratedWorkloadDeploymentConflict)
	}
	return nil
}

func requireGeneratedDeploymentForwardQuiescenceTx(
	ctx context.Context,
	tx pgx.Tx,
	operationID string,
) error {
	quiescent, err := generatedDeploymentForwardQuiescentTx(ctx, tx, operationID)
	if err != nil {
		return err
	}
	if !quiescent {
		return fmt.Errorf(
			"%w: deployment forward-command quiescence is unproven",
			ErrGeneratedWorkloadDeploymentState,
		)
	}
	return nil
}

func generatedDeploymentForwardQuiescentTx(
	ctx context.Context,
	tx pgx.Tx,
	operationID string,
) (bool, error) {
	var quiescent bool
	if err := tx.QueryRow(ctx, `
		SELECT NOT EXISTS(
		 SELECT 1 FROM generated_workload_deployment_executions
		 WHERE operation_id=$1 AND status='started'
		)
	`, operationID).Scan(&quiescent); err != nil {
		return false, fmt.Errorf("inspect deployment forward-command quiescence: %w", err)
	}
	return quiescent, nil
}
