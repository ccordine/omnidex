package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func loadGeneratedDeploymentExecutionTx(
	ctx context.Context,
	querier generatedDeploymentExecutionQuerier,
	operationID string,
	ordinal int,
	lock bool,
) (GeneratedWorkloadDeploymentExecutionRecord, bool, error) {
	query := `
		SELECT operation_id,slot_name,slot_ordinal,command_sha256,workspace_sha256,
		       status,succeeded,evidence_id,result_sha256,step_attempt,worker_id,
		       started_at,completed_at
		FROM generated_workload_deployment_executions
		WHERE operation_id=$1 AND slot_ordinal=$2`
	if lock {
		query += ` FOR UPDATE`
	}
	var record GeneratedWorkloadDeploymentExecutionRecord
	var succeeded *bool
	var evidenceID *int64
	var resultSHA *string
	err := querier.QueryRow(ctx, query, operationID, ordinal).Scan(
		&record.OperationID, &record.Slot.Name, &record.Slot.Ordinal,
		&record.CommandSHA256, &record.WorkspaceSHA256, &record.Status,
		&succeeded, &evidenceID, &resultSHA, &record.StepAttempt, &record.WorkerID,
		&record.StartedAt, &record.CompletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return GeneratedWorkloadDeploymentExecutionRecord{}, false, nil
	}
	if err != nil {
		return GeneratedWorkloadDeploymentExecutionRecord{}, false, fmt.Errorf("load deployment execution: %w", err)
	}
	record.Succeeded = succeeded
	if evidenceID != nil {
		record.EvidenceID = *evidenceID
	}
	if resultSHA != nil {
		record.ResultSHA256 = *resultSHA
	}
	if !registeredGeneratedDeploymentSlot(record.Slot) ||
		!validSHA256Digest(record.CommandSHA256) ||
		!validSHA256Digest(record.WorkspaceSHA256) ||
		(record.Status != GeneratedWorkloadDeploymentExecutionStarted &&
			record.Status != GeneratedWorkloadDeploymentExecutionCompleted) {
		return GeneratedWorkloadDeploymentExecutionRecord{}, false, fmt.Errorf("durable deployment execution is invalid")
	}
	return record, true, nil
}

type generatedDeploymentExecutionQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func requireGeneratedDeploymentExecutionIdentity(
	record GeneratedWorkloadDeploymentExecutionRecord,
	execution GeneratedWorkloadDeploymentExecutionCommand,
) error {
	if record.Slot != execution.Slot || record.CommandSHA256 != execution.CommandSHA256 ||
		record.WorkspaceSHA256 != execution.WorkspaceSHA256 {
		return fmt.Errorf("%w: deployment execution slot identity differs", ErrGeneratedWorkloadDeploymentConflict)
	}
	return nil
}
