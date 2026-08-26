package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const generatedWorkloadDeploymentExecutionEvidenceSource = "generated_workload_deployment_execution"

func (r *Repository) BeginGeneratedWorkloadDeploymentExecution(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	execution GeneratedWorkloadDeploymentExecutionCommand,
) (GeneratedWorkloadDeploymentExecutionRecord, bool, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, false, fmt.Errorf("begin deployment execution requires PostgreSQL and context")
	}
	if err := validateGeneratedDeploymentExecutionCommand(command, execution); err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, false, err
	}
	identity, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, false, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, false, err
	}
	defer tx.Rollback(ctx)
	if err := lockGeneratedDeploymentAuthorityTx(ctx, tx, authority, command); err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, false, err
	}
	deployment, err := lockGeneratedDeploymentTx(ctx, tx, identity.OperationID)
	if err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, false, err
	}
	if err := requireGeneratedDeploymentIdentity(deployment, identity); err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, false, err
	}
	if deployment.State != GeneratedWorkloadDeploymentApplying {
		return GeneratedWorkloadDeploymentExecutionRecord{}, false, fmt.Errorf("%w: deployment execution cannot start from %s", ErrGeneratedWorkloadDeploymentState, deployment.State)
	}
	record, found, err := loadGeneratedDeploymentExecutionTx(ctx, tx, identity.OperationID, execution.Slot.Ordinal, true)
	if err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, false, err
	}
	if found {
		if err := requireGeneratedDeploymentExecutionIdentity(record, execution); err != nil {
			return GeneratedWorkloadDeploymentExecutionRecord{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return GeneratedWorkloadDeploymentExecutionRecord{}, false, err
		}
		return record, false, nil
	}
	if err := requireGeneratedDeploymentExecutionAppendTx(
		ctx, tx, identity.OperationID, authority, execution,
	); err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, false, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO generated_workload_deployment_executions(
		 operation_id,slot_name,slot_ordinal,command_sha256,workspace_sha256,
		 status,step_attempt,worker_id)
		VALUES($1,$2,$3,$4,$5,'started',$6,$7)
	`, identity.OperationID, execution.Slot.Name, execution.Slot.Ordinal,
		execution.CommandSHA256, execution.WorkspaceSHA256, authority.Attempt, authority.WorkerID)
	if err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, false, fmt.Errorf("begin deployment execution: %w", err)
	}
	record, found, err = loadGeneratedDeploymentExecutionTx(ctx, tx, identity.OperationID, execution.Slot.Ordinal, true)
	if err != nil || !found {
		return GeneratedWorkloadDeploymentExecutionRecord{}, false, fmt.Errorf("reload begun deployment execution: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, false, err
	}
	return record, true, nil
}

func (r *Repository) CompleteGeneratedWorkloadDeploymentExecution(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
	execution GeneratedWorkloadDeploymentExecutionCommand,
	result evidence.Record,
) (GeneratedWorkloadDeploymentExecutionRecord, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, fmt.Errorf("complete deployment execution requires PostgreSQL and context")
	}
	if err := validateGeneratedDeploymentExecutionCommand(command, execution); err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, err
	}
	identity, err := generatedWorkloadDeploymentOperation(command)
	if err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, err
	}
	succeeded, payload, resultSHA, err := generatedDeploymentExecutionEvidence(
		command, identity.OperationID, execution, result,
	)
	if err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockGeneratedDeploymentAuthorityTx(ctx, tx, authority, command); err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, err
	}
	deployment, err := lockGeneratedDeploymentTx(ctx, tx, identity.OperationID)
	if err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, err
	}
	if err := requireGeneratedDeploymentIdentity(deployment, identity); err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, err
	}
	if deployment.State != GeneratedWorkloadDeploymentApplying {
		return GeneratedWorkloadDeploymentExecutionRecord{}, fmt.Errorf(
			"%w: deployment execution cannot complete from %s",
			ErrGeneratedWorkloadDeploymentState, deployment.State,
		)
	}
	record, found, err := loadGeneratedDeploymentExecutionTx(ctx, tx, identity.OperationID, execution.Slot.Ordinal, true)
	if err != nil || !found {
		return GeneratedWorkloadDeploymentExecutionRecord{}, fmt.Errorf("deployment execution was not durably started: %w", err)
	}
	if err := requireGeneratedDeploymentExecutionIdentity(record, execution); err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, err
	}
	if record.StepAttempt != authority.Attempt || record.WorkerID != authority.WorkerID {
		return GeneratedWorkloadDeploymentExecutionRecord{}, fmt.Errorf(
			"%w: deployment execution owner differs from current executor",
			ErrGeneratedWorkloadDeploymentConflict,
		)
	}
	if record.Status == GeneratedWorkloadDeploymentExecutionCompleted {
		if record.ResultSHA256 != resultSHA || record.Succeeded == nil || *record.Succeeded != succeeded {
			return GeneratedWorkloadDeploymentExecutionRecord{}, fmt.Errorf("%w: completed deployment execution result differs", ErrGeneratedWorkloadDeploymentConflict)
		}
		if err := tx.Commit(ctx); err != nil {
			return GeneratedWorkloadDeploymentExecutionRecord{}, err
		}
		return record, nil
	}
	var evidenceID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json)
		VALUES($1,$2,$3,$4,$5,$6::jsonb) RETURNING id
	`, command.Authority.JobID, command.Authority.StepID, result.Kind,
		generatedWorkloadDeploymentExecutionEvidenceSource,
		identity.OperationID, payload).Scan(&evidenceID); err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, fmt.Errorf("insert deployment execution evidence: %w", err)
	}
	_, err = tx.Exec(ctx, `
		UPDATE generated_workload_deployment_executions
		SET status='completed',succeeded=$3,evidence_id=$4,result_sha256=$5,
		    completed_at=clock_timestamp()
		WHERE operation_id=$1 AND slot_ordinal=$2
	`, identity.OperationID, execution.Slot.Ordinal, succeeded, evidenceID, resultSHA)
	if err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, fmt.Errorf("complete deployment execution: %w", err)
	}
	record, _, err = loadGeneratedDeploymentExecutionTx(ctx, tx, identity.OperationID, execution.Slot.Ordinal, true)
	if err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, err
	}
	return record, nil
}

func generatedDeploymentExecutionEvidence(
	command GeneratedWorkloadDeploymentCommand,
	operationID string,
	execution GeneratedWorkloadDeploymentExecutionCommand,
	record evidence.Record,
) (bool, string, string, error) {
	if record.JobID != 0 && record.JobID != command.Authority.JobID ||
		record.StepID != 0 && record.StepID != command.Authority.StepID {
		return false, "", "", fmt.Errorf("deployment execution evidence owner differs")
	}
	record.JobID, record.StepID = command.Authority.JobID, command.Authority.StepID
	if err := record.Validate(); err != nil {
		return false, "", "", err
	}
	if record.Kind != evidence.KindCommandOutput && record.Kind != evidence.KindTestResult ||
		record.SourceType != "command" || generatedDeploymentSHA(record.Command) != execution.CommandSHA256 {
		return false, "", "", fmt.Errorf("deployment execution evidence differs from exact command digest")
	}
	succeeded, ok := record.Metadata["succeeded"].(bool)
	if !ok || record.Metadata["execution"] != true || record.Metadata["side_effect_possible"] != true {
		return false, "", "", fmt.Errorf("deployment execution evidence lacks exact execution result")
	}
	record.SourceType = generatedWorkloadDeploymentExecutionEvidenceSource
	record.SourceRef = operationID
	delete(record.Metadata, "workspace")
	record.Metadata["deployment_operation_id"] = operationID
	record.Metadata["slot"] = execution.Slot.Name
	record.Metadata["ordinal"] = execution.Slot.Ordinal
	record.Metadata["command_sha256"] = execution.CommandSHA256
	record.Metadata["workspace_sha256"] = execution.WorkspaceSHA256
	payload, err := json.Marshal(record)
	if err != nil || len(payload) > 1<<20 {
		return false, "", "", fmt.Errorf("deployment execution evidence exceeds canonical bound: %w", err)
	}
	return succeeded, string(payload), generatedDeploymentSHA(string(payload)), nil
}

func validateGeneratedDeploymentExecutionCommand(
	command GeneratedWorkloadDeploymentCommand,
	execution GeneratedWorkloadDeploymentExecutionCommand,
) error {
	if err := validateGeneratedWorkloadDeploymentCommand(command); err != nil {
		return err
	}
	if !registeredGeneratedDeploymentSlot(execution.Slot) ||
		!repositoryMutationHexDigest(execution.CommandSHA256) ||
		execution.WorkspaceSHA256 != command.WorkspaceSHA256 {
		return fmt.Errorf("deployment execution command authority is invalid")
	}
	return nil
}

func registeredGeneratedDeploymentSlot(slot GeneratedWorkloadDeploymentLifecycleSlot) bool {
	for _, registered := range generatedDeploymentLifecycleSlots() {
		if registered == slot {
			return true
		}
	}
	return false
}

func generatedDeploymentLifecycleSlots() []GeneratedWorkloadDeploymentLifecycleSlot {
	return []GeneratedWorkloadDeploymentLifecycleSlot{
		GeneratedDeploymentSlotConfig, GeneratedDeploymentSlotBuild,
		GeneratedDeploymentSlotInitialStart, GeneratedDeploymentSlotMigrate,
		GeneratedDeploymentSlotInitialObserve, GeneratedDeploymentSlotStateWrite,
		GeneratedDeploymentSlotRestart, GeneratedDeploymentSlotRestartStart,
		GeneratedDeploymentSlotFinalObserve, GeneratedDeploymentSlotStateRead,
	}
}
