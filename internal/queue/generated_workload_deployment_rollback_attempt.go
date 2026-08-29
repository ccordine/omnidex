package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const generatedWorkloadDeploymentRollbackEvidenceSource = "generated_workload_deployment_rollback"

func (r *Repository) BeginGeneratedWorkloadDeploymentRollbackAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	plan GeneratedWorkloadDeploymentRollbackPlan,
) (GeneratedWorkloadDeploymentRollbackAttemptRecord, bool, error) {
	identity, err := validateGeneratedDeploymentRollbackAttemptRequest(
		ctx, r, authority, command, plan,
	)
	if err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, false, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, false, fmt.Errorf("begin deployment rollback attempt transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockGeneratedDeploymentAuthorityTx(ctx, tx, authority, command); err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, false, err
	}
	if err := requireGeneratedDeploymentRollbackMutationTx(
		ctx, tx, authority, command, identity, plan,
	); err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, false, err
	}
	if err := requireGeneratedDeploymentForwardQuiescenceTx(
		ctx, tx, identity.OperationID,
	); err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, false, err
	}
	record, found, err := loadGeneratedDeploymentRollbackAttemptTx(
		ctx, tx, command, identity.OperationID, authority.Attempt, plan, true,
	)
	if err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, false, err
	}
	if found {
		if err := requireGeneratedDeploymentRollbackAttemptIdentity(record, authority, plan); err != nil {
			return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, false, err
		}
		return record, false, nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO generated_workload_deployment_rollback_attempts(
		 operation_id,job_id,generation,step_id,step_attempt,worker_id,
		 command_sha256,workspace_sha256,status)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,'started')
	`, identity.OperationID, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID, plan.Execution.CommandSHA256,
		plan.Execution.WorkspaceSHA256); err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, false, fmt.Errorf("persist deployment rollback attempt start: %w", err)
	}
	record, found, err = loadGeneratedDeploymentRollbackAttemptTx(
		ctx, tx, command, identity.OperationID, authority.Attempt, plan, true,
	)
	if err != nil || !found {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, false, fmt.Errorf("reload deployment rollback attempt start: %w", err)
	}
	if err := requireGeneratedDeploymentRollbackAttemptIdentity(record, authority, plan); err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, false, fmt.Errorf("commit deployment rollback attempt start: %w", err)
	}
	return record, true, nil
}

func (r *Repository) CompleteGeneratedWorkloadDeploymentRollbackAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	plan GeneratedWorkloadDeploymentRollbackPlan,
	result evidence.Record,
) (GeneratedWorkloadDeploymentRollbackAttemptRecord, error) {
	identity, err := validateGeneratedDeploymentRollbackAttemptRequest(
		ctx, r, authority, command, plan,
	)
	if err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, err
	}
	succeeded, payload, resultSHA, err := generatedDeploymentRollbackAttemptEvidence(
		command, identity.OperationID, authority.Attempt, plan, result,
	)
	if err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, fmt.Errorf("begin deployment rollback completion transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockGeneratedDeploymentAuthorityTx(ctx, tx, authority, command); err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, err
	}
	if err := requireGeneratedDeploymentRollbackMutationTx(
		ctx, tx, authority, command, identity, plan,
	); err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, err
	}
	record, found, err := loadGeneratedDeploymentRollbackAttemptTx(
		ctx, tx, command, identity.OperationID, authority.Attempt, plan, true,
	)
	if err != nil || !found {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, fmt.Errorf("deployment rollback attempt was not durably started: %w", err)
	}
	if err := requireGeneratedDeploymentRollbackAttemptIdentity(record, authority, plan); err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, err
	}
	if record.Status == GeneratedWorkloadDeploymentExecutionCompleted {
		if record.Succeeded == nil || *record.Succeeded != succeeded || record.ResultSHA256 != resultSHA {
			return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, fmt.Errorf("%w: completed deployment rollback result differs", ErrGeneratedWorkloadDeploymentConflict)
		}
		if err := tx.Commit(ctx); err != nil {
			return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, err
		}
		return record, nil
	}
	var evidenceID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json)
		VALUES($1,$2,$3,$4,$5,$6::jsonb) RETURNING id
	`, command.Authority.JobID, command.Authority.StepID, result.Kind,
		generatedWorkloadDeploymentRollbackEvidenceSource,
		identity.OperationID, payload).Scan(&evidenceID); err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, fmt.Errorf("persist deployment rollback command evidence: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE generated_workload_deployment_rollback_attempts
		SET status='completed',succeeded=$3,evidence_id=$4,result_sha256=$5,
		    completed_at=clock_timestamp()
		WHERE operation_id=$1 AND step_attempt=$2
	`, identity.OperationID, authority.Attempt, succeeded, evidenceID, resultSHA); err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, fmt.Errorf("complete deployment rollback attempt: %w", err)
	}
	record, found, err = loadGeneratedDeploymentRollbackAttemptTx(
		ctx, tx, command, identity.OperationID, authority.Attempt, plan, true,
	)
	if err != nil || !found {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, fmt.Errorf("reload completed deployment rollback attempt: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return GeneratedWorkloadDeploymentRollbackAttemptRecord{}, fmt.Errorf("commit deployment rollback attempt completion: %w", err)
	}
	return record, nil
}

func validateGeneratedDeploymentRollbackAttemptRequest(
	ctx context.Context,
	r *Repository,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	plan GeneratedWorkloadDeploymentRollbackPlan,
) (generatedWorkloadDeploymentIdentity, error) {
	if ctx == nil {
		return generatedWorkloadDeploymentIdentity{}, fmt.Errorf("deployment rollback attempt requires a context")
	}
	if err := ctx.Err(); err != nil {
		return generatedWorkloadDeploymentIdentity{}, fmt.Errorf("deployment rollback attempt: %w", err)
	}
	if r == nil || r.pool == nil {
		return generatedWorkloadDeploymentIdentity{}, ErrRepositoryNotConfigured
	}
	if err := validateGeneratedDeploymentExecutionAuthority(authority, command); err != nil {
		return generatedWorkloadDeploymentIdentity{}, err
	}
	if err := validateGeneratedWorkloadDeploymentRollbackPlan(command, plan); err != nil {
		return generatedWorkloadDeploymentIdentity{}, err
	}
	return generatedWorkloadDeploymentOperation(command)
}
