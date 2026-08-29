package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) RecordGeneratedWorkloadDeploymentNamespaceRequalification(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	execution GeneratedWorkloadDeploymentExecutionCommand,
	proof GeneratedWorkloadDeploymentNamespacePreflight,
) (GeneratedWorkloadDeploymentNamespaceRequalificationRecord, bool, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false,
			fmt.Errorf("record deployment namespace requalification requires PostgreSQL and context")
	}
	if err := validateGeneratedDeploymentExecutionCommand(command, execution); err != nil {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false, err
	}
	if !generatedDeploymentSlotRequiresNamespaceRequalification(execution.Slot) {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false,
			fmt.Errorf("deployment slot %s does not require namespace requalification", execution.Slot.Name)
	}
	bound, proofJSON, err := BindGeneratedWorkloadDeploymentNamespacePreflight(proof)
	if err != nil {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false,
			fmt.Errorf("deployment namespace requalification proof is invalid: %w", err)
	}
	if bound.ComposeProject != command.ComposeProject ||
		!GeneratedWorkloadDeploymentNamespaceVacant(bound) {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false,
			fmt.Errorf("deployment namespace requalification lacks an exact vacant proof")
	}
	identity, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false, err
	}
	defer tx.Rollback(ctx)
	if err := lockGeneratedDeploymentAuthorityTx(ctx, tx, authority, command); err != nil {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false, err
	}
	deployment, err := lockGeneratedDeploymentTx(ctx, tx, identity.OperationID)
	if err != nil {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false, err
	}
	if err := requireGeneratedDeploymentNamespaceRequalificationAuthorityTx(
		ctx, tx, authority, command, identity, deployment,
	); err != nil {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false, err
	}
	existing, found, err := loadGeneratedDeploymentNamespaceRequalificationTx(
		ctx, tx, identity.OperationID, execution.Slot.Ordinal, authority.Attempt, true,
	)
	if err != nil {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false, err
	}
	if found {
		if err := requireGeneratedDeploymentNamespaceRequalificationIdentity(
			existing, authority, command, execution, bound, proofJSON,
		); err != nil {
			return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false, err
		}
		return existing, false, nil
	}
	if err := requireGeneratedDeploymentExecutionAppendTx(
		ctx, tx, identity.OperationID, authority, execution,
	); err != nil {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false, err
	}
	evidenceID, observedAt, err := insertGeneratedDeploymentNamespaceRequalificationEvidenceTx(
		ctx, tx, authority, command, identity.OperationID, execution, bound, proofJSON,
	)
	if err != nil {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO generated_workload_deployment_namespace_requalifications(
		 operation_id,job_id,generation,step_id,slot_name,slot_ordinal,
		 command_sha256,workspace_sha256,compose_project,step_attempt,worker_id,
		 proof_json,proof_sha256,evidence_id,observed_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`, identity.OperationID, command.Authority.JobID, command.Authority.Generation,
		command.Authority.StepID, execution.Slot.Name, execution.Slot.Ordinal,
		execution.CommandSHA256, execution.WorkspaceSHA256, command.ComposeProject,
		authority.Attempt, authority.WorkerID, proofJSON, bound.SHA256, evidenceID, observedAt)
	if err != nil {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false,
			fmt.Errorf("record deployment namespace requalification: %w", err)
	}
	record, found, err := loadGeneratedDeploymentNamespaceRequalificationTx(
		ctx, tx, identity.OperationID, execution.Slot.Ordinal, authority.Attempt, true,
	)
	if err != nil || !found {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false,
			fmt.Errorf("reload deployment namespace requalification: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return GeneratedWorkloadDeploymentNamespaceRequalificationRecord{}, false, err
	}
	return record, true, nil
}

func requireGeneratedDeploymentNamespaceRequalificationIdentity(
	record GeneratedWorkloadDeploymentNamespaceRequalificationRecord,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	execution GeneratedWorkloadDeploymentExecutionCommand,
	proof GeneratedWorkloadDeploymentNamespacePreflight,
	proofJSON string,
) error {
	identity, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		return err
	}
	if record.OperationID != identity.OperationID || record.JobID != command.Authority.JobID ||
		record.Generation != command.Authority.Generation || record.StepID != command.Authority.StepID ||
		record.Slot != execution.Slot || record.CommandSHA256 != execution.CommandSHA256 ||
		record.WorkspaceSHA256 != execution.WorkspaceSHA256 ||
		record.ComposeProject != command.ComposeProject || record.StepAttempt != authority.Attempt ||
		record.WorkerID != authority.WorkerID || record.ProofJSON != proofJSON ||
		record.ProofSHA256 != proof.SHA256 {
		return fmt.Errorf(
			"%w: deployment namespace requalification replay differs",
			ErrGeneratedWorkloadDeploymentConflict,
		)
	}
	return nil
}

func insertGeneratedDeploymentNamespaceRequalificationEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	operationID string,
	execution GeneratedWorkloadDeploymentExecutionCommand,
	proof GeneratedWorkloadDeploymentNamespacePreflight,
	proofJSON string,
) (int64, time.Time, error) {
	sourceRef := operationID + ":" + strconv.Itoa(execution.Slot.Ordinal) + ":" +
		strconv.FormatInt(authority.Attempt, 10)
	payload := evidence.Record{
		JobID: command.Authority.JobID, StepID: command.Authority.StepID,
		Kind:       evidence.KindDeploymentObservation,
		SourceType: generatedWorkloadDeploymentNamespaceRequalificationSource,
		SourceRef:  sourceRef, Excerpt: proofJSON, Hash: proof.SHA256,
		Summary:    "Raw Docker API observation proved the protected Compose namespace vacant immediately before execution.",
		Confidence: 1,
		Metadata: map[string]any{
			"schema":                  GeneratedWorkloadDeploymentNamespaceRequalificationV1,
			"deployment_operation_id": operationID,
			"slot":                    execution.Slot.Name, "ordinal": execution.Slot.Ordinal,
			"command_sha256":   execution.CommandSHA256,
			"workspace_sha256": execution.WorkspaceSHA256,
			"compose_project":  command.ComposeProject,
			"step_attempt":     authority.Attempt, "worker_id": authority.WorkerID,
			"namespace_vacant": true, "proof_sha256": proof.SHA256,
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("encode deployment namespace requalification evidence: %w", err)
	}
	var evidenceID int64
	var observedAt time.Time
	if err := tx.QueryRow(ctx, `
		INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json)
		VALUES($1,$2,$3,$4,$5,$6::jsonb) RETURNING id,created_at
	`, command.Authority.JobID, command.Authority.StepID, payload.Kind,
		payload.SourceType, payload.SourceRef, string(encoded)).Scan(
		&evidenceID, &observedAt,
	); err != nil {
		return 0, time.Time{}, fmt.Errorf("insert deployment namespace requalification evidence: %w", err)
	}
	return evidenceID, observedAt, nil
}
