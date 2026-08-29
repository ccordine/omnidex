package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) GeneratedWorkloadDeploymentNeedsNamespaceRequalification(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	execution GeneratedWorkloadDeploymentExecutionCommand,
) (bool, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return false, fmt.Errorf("inspect deployment namespace requalification requires PostgreSQL and context")
	}
	if err := validateGeneratedDeploymentExecutionCommand(command, execution); err != nil {
		return false, err
	}
	if !generatedDeploymentSlotRequiresNamespaceRequalification(execution.Slot) {
		return false, nil
	}
	identity, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		return false, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if err := lockGeneratedDeploymentAuthorityTx(ctx, tx, authority, command); err != nil {
		return false, err
	}
	deployment, err := lockGeneratedDeploymentTx(ctx, tx, identity.OperationID)
	if err != nil {
		return false, err
	}
	if err := requireGeneratedDeploymentNamespaceRequalificationAuthorityTx(
		ctx, tx, authority, command, identity, deployment,
	); err != nil {
		return false, err
	}
	record, found, err := loadGeneratedDeploymentExecutionTx(
		ctx, tx, identity.OperationID, execution.Slot.Ordinal, true,
	)
	if err != nil {
		return false, err
	}
	if found {
		if err := requireGeneratedDeploymentExecutionIdentity(record, execution); err != nil {
			return false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}
	if err := requireGeneratedDeploymentExecutionAppendTx(
		ctx, tx, identity.OperationID, authority, execution,
	); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func requireGeneratedDeploymentNamespaceRequalificationAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	identity generatedWorkloadDeploymentIdentity,
	deployment GeneratedWorkloadDeploymentRecord,
) error {
	if err := requireGeneratedDeploymentIdentity(deployment, identity); err != nil {
		return err
	}
	if deployment.State != GeneratedWorkloadDeploymentApplying ||
		deployment.Current.StepAttempt != authority.Attempt ||
		deployment.Current.WorkerID != authority.WorkerID {
		return fmt.Errorf(
			"%w: deployment namespace requalification lacks exact current executor",
			ErrGeneratedWorkloadDeploymentState,
		)
	}
	head, found, err := lockGeneratedWorkloadProjectDeploymentHeadTx(
		ctx, tx, command.Authority.ProjectID,
	)
	if err != nil {
		return err
	}
	if !found || head.ComposeProject != command.ComposeProject || head.Candidate == nil ||
		head.Candidate.DeploymentID != identity.OperationID ||
		head.Candidate.Authority != command.Authority ||
		head.Candidate.Executor.StepAttempt != authority.Attempt ||
		head.Candidate.Executor.WorkerID != authority.WorkerID {
		return fmt.Errorf(
			"%w: deployment namespace requalification lacks exact project candidate",
			ErrGeneratedWorkloadProjectDeploymentHeadConflict,
		)
	}
	return nil
}

func loadGeneratedDeploymentNamespaceRequalificationTx(
	ctx context.Context,
	querier generatedDeploymentExecutionQuerier,
	operationID string,
	slotOrdinal int,
	stepAttempt int64,
	lock bool,
) (GeneratedWorkloadDeploymentNamespaceRequalificationRecord, bool, error) {
	query := `
		SELECT operation_id,job_id,generation,step_id,slot_name,slot_ordinal,
		       command_sha256,workspace_sha256,compose_project,step_attempt,worker_id,
		       proof_json,proof_sha256,evidence_id,observed_at
		FROM generated_workload_deployment_namespace_requalifications
		WHERE operation_id=$1 AND slot_ordinal=$2 AND step_attempt=$3`
	if lock {
		query += ` FOR UPDATE`
	}
	var record GeneratedWorkloadDeploymentNamespaceRequalificationRecord
	err := querier.QueryRow(ctx, query, operationID, slotOrdinal, stepAttempt).Scan(
		&record.OperationID, &record.JobID, &record.Generation, &record.StepID,
		&record.Slot.Name, &record.Slot.Ordinal, &record.CommandSHA256,
		&record.WorkspaceSHA256, &record.ComposeProject, &record.StepAttempt,
		&record.WorkerID, &record.ProofJSON, &record.ProofSHA256,
		&record.EvidenceID, &record.ObservedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false, nil
	}
	if err != nil {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false,
			fmt.Errorf("load deployment namespace requalification: %w", err)
	}
	if err := json.Unmarshal([]byte(record.ProofJSON), &record.Proof); err != nil {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false,
			fmt.Errorf("decode deployment namespace requalification: %w", err)
	}
	record.Proof.SHA256 = record.ProofSHA256
	bound, encoded, err := BindGeneratedWorkloadDeploymentNamespacePreflight(record.Proof)
	if err != nil || encoded != record.ProofJSON || !GeneratedWorkloadDeploymentNamespaceVacant(bound) ||
		record.ProofSHA256 != bound.SHA256 || record.EvidenceID <= 0 || record.ObservedAt.IsZero() ||
		record.OperationID == "" || record.JobID <= 0 || record.Generation <= 0 || record.StepID <= 0 ||
		!generatedDeploymentSlotRequiresNamespaceRequalification(record.Slot) ||
		!validSHA256Digest(record.CommandSHA256) ||
		!validSHA256Digest(record.WorkspaceSHA256) ||
		record.ComposeProject != bound.ComposeProject || record.StepAttempt <= 0 || record.WorkerID == "" {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false,
			fmt.Errorf("durable deployment namespace requalification is invalid")
	}
	record.Proof = bound
	return record, true, nil
}
