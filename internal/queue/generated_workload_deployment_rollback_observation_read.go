package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) GeneratedWorkloadDeploymentRollbackObservation(
	ctx context.Context,
	command GeneratedWorkloadDeploymentCommand,
	rollbackStepAttempt int64,
) (*GeneratedWorkloadDeploymentRollbackObservationRecord, error) {
	if rollbackStepAttempt <= 0 {
		return nil, fmt.Errorf("rollback observation requires a positive attempt identity")
	}
	identity, plan, err := r.validateGeneratedDeploymentRollbackAttemptRead(ctx, command)
	if err != nil {
		return nil, err
	}
	record, found, err := loadGeneratedDeploymentRollbackObservationTx(
		ctx, r.pool, identity.OperationID, rollbackStepAttempt, false,
	)
	if err != nil || !found {
		return nil, err
	}
	if record.Basis != GeneratedWorkloadDeploymentRollbackObservationCommandAttempt {
		return nil, fmt.Errorf("rollback attempt observation has an invalid basis")
	}
	_, outcome, err := canonicalGeneratedDeploymentRollbackObservation(
		plan, record.Observation,
	)
	if err != nil || outcome != record.Outcome {
		return nil, fmt.Errorf("validate durable rollback observation outcome: %w", err)
	}
	return &record, nil
}

func (r *Repository) GeneratedWorkloadDeploymentPreRollbackObservation(
	ctx context.Context,
	command GeneratedWorkloadDeploymentCommand,
	observerStepAttempt int64,
) (*GeneratedWorkloadDeploymentRollbackObservationRecord, error) {
	if observerStepAttempt <= 0 {
		return nil, fmt.Errorf("pre-attempt rollback observation requires a positive observer attempt")
	}
	identity, plan, err := r.validateGeneratedDeploymentRollbackAttemptRead(ctx, command)
	if err != nil {
		return nil, err
	}
	record, found, err := loadGeneratedDeploymentRollbackObservationTx(
		ctx, r.pool, identity.OperationID, -observerStepAttempt, false,
	)
	if err != nil || !found {
		return nil, err
	}
	if record.Basis != GeneratedWorkloadDeploymentRollbackObservationPreAttempt {
		return nil, fmt.Errorf("pre-attempt rollback observation has an invalid basis")
	}
	_, outcome, err := canonicalGeneratedDeploymentRollbackObservation(plan, record.Observation)
	if err != nil || outcome != record.Outcome {
		return nil, fmt.Errorf("validate durable pre-attempt rollback observation outcome: %w", err)
	}
	return &record, nil
}

func (r *Repository) CurrentGeneratedWorkloadDeploymentPreRollbackObservation(
	ctx context.Context,
	command GeneratedWorkloadDeploymentCommand,
) (*GeneratedWorkloadDeploymentRollbackObservationRecord, error) {
	identity, plan, err := r.validateGeneratedDeploymentRollbackAttemptRead(ctx, command)
	if err != nil {
		return nil, err
	}
	var stepAttempt int64
	err = r.pool.QueryRow(ctx, `
		SELECT rollback_step_attempt
		FROM generated_workload_deployment_rollback_observations
		WHERE operation_id=$1 AND basis='pre_attempt'
		ORDER BY observer_step_attempt DESC LIMIT 1
	`, identity.OperationID).Scan(&stepAttempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load current pre-attempt rollback observation: %w", err)
	}
	record, found, err := loadGeneratedDeploymentRollbackObservationTx(
		ctx, r.pool, identity.OperationID, stepAttempt, false,
	)
	if err != nil || !found {
		return nil, err
	}
	if record.Basis != GeneratedWorkloadDeploymentRollbackObservationPreAttempt {
		return nil, fmt.Errorf("current pre-attempt rollback observation has an invalid basis")
	}
	_, outcome, err := canonicalGeneratedDeploymentRollbackObservation(plan, record.Observation)
	if err != nil || outcome != record.Outcome {
		return nil, fmt.Errorf("validate current pre-attempt rollback observation outcome: %w", err)
	}
	return &record, nil
}
