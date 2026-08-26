package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CurrentGeneratedWorkloadDeploymentRollbackAttempt(
	ctx context.Context,
	command GeneratedWorkloadDeploymentCommand,
) (*GeneratedWorkloadDeploymentRollbackAttemptRecord, error) {
	identity, plan, err := r.validateGeneratedDeploymentRollbackAttemptRead(ctx, command)
	if err != nil {
		return nil, err
	}
	record, err := scanGeneratedDeploymentRollbackAttempt(
		r.pool.QueryRow(ctx, `
			SELECT operation_id,job_id,generation,step_id,step_attempt,worker_id,
			       command_sha256,workspace_sha256,status,succeeded,evidence_id,
			       result_sha256,started_at,completed_at
			FROM generated_workload_deployment_rollback_attempts
			WHERE operation_id=$1 ORDER BY step_attempt DESC LIMIT 1
		`, identity.OperationID), command, identity.OperationID, plan.Execution.CommandSHA256,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load current deployment rollback attempt: %w", err)
	}
	return &record, nil
}

func (r *Repository) GeneratedWorkloadDeploymentRollbackAttempts(
	ctx context.Context,
	command GeneratedWorkloadDeploymentCommand,
) ([]GeneratedWorkloadDeploymentRollbackAttemptRecord, error) {
	identity, plan, err := r.validateGeneratedDeploymentRollbackAttemptRead(ctx, command)
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT operation_id,job_id,generation,step_id,step_attempt,worker_id,
		       command_sha256,workspace_sha256,status,succeeded,evidence_id,
		       result_sha256,started_at,completed_at
		FROM generated_workload_deployment_rollback_attempts
		WHERE operation_id=$1 ORDER BY step_attempt
	`, identity.OperationID)
	if err != nil {
		return nil, fmt.Errorf("list deployment rollback attempts: %w", err)
	}
	defer rows.Close()
	records := make([]GeneratedWorkloadDeploymentRollbackAttemptRecord, 0)
	for rows.Next() {
		record, err := scanGeneratedDeploymentRollbackAttempt(
			rows, command, identity.OperationID, plan.Execution.CommandSHA256,
		)
		if err != nil {
			return nil, fmt.Errorf("scan deployment rollback attempt: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list deployment rollback attempts: %w", err)
	}
	return records, nil
}

func (r *Repository) validateGeneratedDeploymentRollbackAttemptRead(
	ctx context.Context,
	command GeneratedWorkloadDeploymentCommand,
) (generatedWorkloadDeploymentIdentity, GeneratedWorkloadDeploymentRollbackPlan, error) {
	if ctx == nil {
		return generatedWorkloadDeploymentIdentity{}, GeneratedWorkloadDeploymentRollbackPlan{}, fmt.Errorf("load deployment rollback attempts requires a context")
	}
	if err := ctx.Err(); err != nil {
		return generatedWorkloadDeploymentIdentity{}, GeneratedWorkloadDeploymentRollbackPlan{}, fmt.Errorf("load deployment rollback attempts: %w", err)
	}
	if r == nil || r.pool == nil {
		return generatedWorkloadDeploymentIdentity{}, GeneratedWorkloadDeploymentRollbackPlan{}, ErrRepositoryNotConfigured
	}
	identity, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		return generatedWorkloadDeploymentIdentity{}, GeneratedWorkloadDeploymentRollbackPlan{}, err
	}
	plan, found, err := loadGeneratedDeploymentRollbackPlanTx(
		ctx, r.pool, identity.OperationID, false,
	)
	if err != nil {
		return generatedWorkloadDeploymentIdentity{}, GeneratedWorkloadDeploymentRollbackPlan{}, err
	}
	if !found {
		return generatedWorkloadDeploymentIdentity{}, GeneratedWorkloadDeploymentRollbackPlan{}, fmt.Errorf("deployment rollback plan is unavailable")
	}
	if err := validateGeneratedWorkloadDeploymentRollbackPlan(command, plan); err != nil {
		return generatedWorkloadDeploymentIdentity{}, GeneratedWorkloadDeploymentRollbackPlan{}, fmt.Errorf("validate durable deployment rollback plan: %w", err)
	}
	return identity, plan, nil
}

func loadGeneratedDeploymentRollbackAttemptTx(
	ctx context.Context,
	querier generatedDeploymentExecutionQuerier,
	command GeneratedWorkloadDeploymentCommand,
	operationID string,
	stepAttempt int64,
	plan GeneratedWorkloadDeploymentRollbackPlan,
	lock bool,
) (GeneratedWorkloadDeploymentRollbackAttemptRecord, bool, error) {
	query := `
		SELECT operation_id,job_id,generation,step_id,step_attempt,worker_id,
		       command_sha256,workspace_sha256,status,succeeded,evidence_id,
		       result_sha256,started_at,completed_at
		FROM generated_workload_deployment_rollback_attempts
		WHERE operation_id=$1 AND step_attempt=$2`
	if lock {
		query += ` FOR UPDATE`
	}
	record, err := scanGeneratedDeploymentRollbackAttempt(
		querier.QueryRow(ctx, query, operationID, stepAttempt), command, operationID,
		plan.Execution.CommandSHA256,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, false, nil
	}
	if err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, false, fmt.Errorf(
			"load deployment rollback attempt: %w", err,
		)
	}
	return record, true, nil
}

type generatedDeploymentRollbackAttemptScanner interface {
	Scan(...any) error
}

func scanGeneratedDeploymentRollbackAttempt(
	row generatedDeploymentRollbackAttemptScanner,
	command GeneratedWorkloadDeploymentCommand,
	operationID string,
	commandSHA256 string,
) (GeneratedWorkloadDeploymentRollbackAttemptRecord, error) {
	var record GeneratedWorkloadDeploymentRollbackAttemptRecord
	var jobID, generation, stepID int64
	var succeeded *bool
	var evidenceID *int64
	var resultSHA *string
	err := row.Scan(
		&record.OperationID, &jobID, &generation, &stepID, &record.StepAttempt,
		&record.WorkerID, &record.CommandSHA256, &record.WorkspaceSHA256,
		&record.Status, &succeeded, &evidenceID, &resultSHA,
		&record.StartedAt, &record.CompletedAt,
	)
	if err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, err
	}
	record.Succeeded = succeeded
	if evidenceID != nil {
		record.EvidenceID = *evidenceID
	}
	if resultSHA != nil {
		record.ResultSHA256 = *resultSHA
	}
	authority := model.StepAttemptAuthority{
		JobID: jobID, Generation: generation, StepID: stepID,
		Attempt: record.StepAttempt, WorkerID: record.WorkerID,
	}
	if err := validateStepAttemptAuthority(authority); err != nil ||
		jobID != command.Authority.JobID || generation != command.Authority.Generation ||
		stepID != command.Authority.StepID || record.OperationID != operationID ||
		record.CommandSHA256 != commandSHA256 ||
		record.WorkspaceSHA256 != command.WorkspaceSHA256 || record.StartedAt.IsZero() {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, fmt.Errorf("durable deployment rollback attempt is invalid")
	}
	if record.Status == GeneratedWorkloadDeploymentExecutionStarted {
		if record.Succeeded != nil || record.EvidenceID != 0 || record.ResultSHA256 != "" || record.CompletedAt != nil {
			return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, fmt.Errorf("durable started rollback attempt is not immutable")
		}
	} else if record.Status == GeneratedWorkloadDeploymentExecutionCompleted {
		if record.Succeeded == nil || record.EvidenceID <= 0 ||
			!repositoryMutationHexDigest(record.ResultSHA256) || record.CompletedAt == nil {
			return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, fmt.Errorf("durable completed rollback attempt is incomplete")
		}
	} else {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, fmt.Errorf("durable deployment rollback attempt status is invalid")
	}
	return record, nil
}
