package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func validateGeneratedDeploymentExecutionAuthority(
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
) error {
	if err := validateGeneratedWorkloadDeploymentCommand(command); err != nil {
		return err
	}
	if err := validateStepAttemptAuthority(authority); err != nil {
		return err
	}
	if authority.JobID != command.Authority.JobID ||
		authority.Generation != command.Authority.Generation ||
		authority.StepID != command.Authority.StepID {
		return staleStepAttemptError(authority, "generated deployment command owner differs", nil)
	}
	return nil
}

func lockGeneratedDeploymentAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	command GeneratedWorkloadDeploymentCommand,
) error {
	if err := validateGeneratedDeploymentExecutionAuthority(authority, command); err != nil {
		return err
	}
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return staleStepAttemptError(authority, "generated deployment attempt is not running", nil)
	}
	var projectID int64
	if err := tx.QueryRow(ctx, `SELECT project_id FROM jobs WHERE id=$1`, authority.JobID).Scan(&projectID); err != nil {
		return fmt.Errorf("read generated deployment project authority: %w", err)
	}
	if projectID != command.Authority.ProjectID {
		return staleStepAttemptError(authority, "generated deployment project authority differs", nil)
	}
	return nil
}

func loadGeneratedDeploymentByGenerationTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation int64,
) (GeneratedWorkloadDeploymentRecord, bool, error) {
	record, err := scanGeneratedWorkloadDeployment(tx.QueryRow(ctx, `
		SELECT id,command_sha256,status,attempt_count,terminal_code,
		       terminal_detail_sha256,receipt_sha256,evidence_id,
		       prepared_at,updated_at,applied_at,observed_at,
		       creator_step_attempt,creator_worker_id,current_step_attempt,current_worker_id
		FROM generated_workload_deployments
		WHERE job_id=$1 AND generation=$2 FOR UPDATE
	`, jobID, generation))
	if errors.Is(err, pgx.ErrNoRows) {
		return GeneratedWorkloadDeploymentRecord{}, false, nil
	}
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, false, fmt.Errorf("load generated deployment generation: %w", err)
	}
	return record, true, nil
}

func lockGeneratedDeploymentTx(
	ctx context.Context,
	tx pgx.Tx,
	operationID string,
) (GeneratedWorkloadDeploymentRecord, error) {
	record, err := scanGeneratedWorkloadDeployment(tx.QueryRow(ctx, `
		SELECT id,command_sha256,status,attempt_count,terminal_code,
		       terminal_detail_sha256,receipt_sha256,evidence_id,
		       prepared_at,updated_at,applied_at,observed_at,
		       creator_step_attempt,creator_worker_id,current_step_attempt,current_worker_id
		FROM generated_workload_deployments WHERE id=$1 FOR UPDATE
	`, operationID))
	if err != nil {
		return GeneratedWorkloadDeploymentRecord{}, fmt.Errorf(
			"lock generated deployment %s: %w", operationID, err,
		)
	}
	return record, nil
}

func scanGeneratedWorkloadDeployment(
	row pgx.Row,
	extraDest ...any,
) (GeneratedWorkloadDeploymentRecord, error) {
	var record GeneratedWorkloadDeploymentRecord
	var terminalCode, detailSHA, receiptSHA *string
	var evidenceID *int64
	dest := []any{
		&record.OperationID, &record.CommandSHA256, &record.State, &record.AttemptCount,
		&terminalCode, &detailSHA, &receiptSHA, &evidenceID,
		&record.PreparedAt, &record.UpdatedAt, &record.AppliedAt, &record.ObservedAt,
		&record.Creator.StepAttempt, &record.Creator.WorkerID,
		&record.Current.StepAttempt, &record.Current.WorkerID,
	}
	dest = append(dest, extraDest...)
	err := row.Scan(dest...)
	if terminalCode != nil {
		record.TerminalCode = *terminalCode
	}
	if detailSHA != nil {
		record.DetailSHA256 = *detailSHA
	}
	if receiptSHA != nil {
		record.ReceiptSHA256 = *receiptSHA
	}
	if evidenceID != nil {
		record.EvidenceID = *evidenceID
	}
	return record, err
}

func requireGeneratedDeploymentIdentity(
	record GeneratedWorkloadDeploymentRecord,
	identity generatedWorkloadDeploymentIdentity,
) error {
	if record.OperationID != identity.OperationID || record.CommandSHA256 != identity.CommandSHA256 {
		return fmt.Errorf("%w: persisted generated deployment command differs", ErrGeneratedWorkloadDeploymentConflict)
	}
	return nil
}
