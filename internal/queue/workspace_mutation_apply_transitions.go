package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) markWorkspaceMutationApplying(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command WorkspaceMutationCommand,
	identity workspaceMutationOperationIdentity,
) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin workspace mutation applying transition: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockWorkspaceMutationAuthorityTx(ctx, tx, authority, command, false); err != nil {
		return err
	}
	record, err := lockWorkspaceMutationOperationTx(ctx, tx, identity.ID)
	if err != nil {
		return err
	}
	if err := requireWorkspaceMutationIdentity(record, identity); err != nil {
		return err
	}
	var attemptCount int
	err = tx.QueryRow(ctx, `
		UPDATE workspace_mutation_operations
		SET status=$2,indeterminate_phase=NULL,last_error=NULL,
		    apply_attempt_count=apply_attempt_count+1,applying_at=clock_timestamp(),
		    current_step_attempt=$3,current_worker_id=$4,updated_at=clock_timestamp()
		WHERE id=$1 AND status IN ($5,$6,$7)
		RETURNING apply_attempt_count
	`, identity.ID, workspaceMutationApplying, authority.Attempt, authority.WorkerID,
		workspaceMutationPrepared, workspaceMutationApplying,
		workspaceMutationIndeterminateState).Scan(&attemptCount)
	if err != nil {
		return fmt.Errorf("mark workspace mutation applying: %w", err)
	}
	if attemptCount <= 0 {
		return fmt.Errorf("workspace mutation %s returned invalid apply attempt count", identity.ID)
	}
	if err := recordTelemetryJobEvent(ctx, tx, command.JobID, "workspace_mutation_applying", map[string]any{
		"operation_id": identity.ID, "workspace_id": command.Plan.WorkspaceID,
		"attempt_count": attemptCount,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit workspace mutation applying transition: %w", err)
	}
	return nil
}

func (r *Repository) recordWorkspaceMutationApplyState(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command WorkspaceMutationCommand,
	identity workspaceMutationOperationIdentity,
	status string,
	reason error,
) error {
	if status != workspaceMutationPrepared && status != workspaceMutationIndeterminateState {
		return fmt.Errorf("workspace mutation apply state %q is invalid", status)
	}
	message, err := exactWorkspaceMutationError(reason)
	if err != nil {
		return err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin workspace mutation apply-state transition: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockWorkspaceMutationAuthorityTx(ctx, tx, authority, command, false); err != nil {
		return err
	}
	record, err := lockWorkspaceMutationOperationTx(ctx, tx, identity.ID)
	if err != nil {
		return err
	}
	if err := requireWorkspaceMutationIdentity(record, identity); err != nil {
		return err
	}
	var phase any
	if status == workspaceMutationIndeterminateState {
		phase = "apply"
	}
	result, err := tx.Exec(ctx, `
		UPDATE workspace_mutation_operations
		SET status=$2,indeterminate_phase=$3,last_error=$4,
		    current_step_attempt=$5,current_worker_id=$6,updated_at=clock_timestamp()
		WHERE id=$1
	`, identity.ID, status, phase, message, authority.Attempt, authority.WorkerID)
	if err != nil {
		return fmt.Errorf("record workspace mutation apply state %q: %w", status, err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("workspace mutation %s lost apply-state authority", identity.ID)
	}
	event := "workspace_mutation_deferred"
	if status == workspaceMutationIndeterminateState {
		event = "workspace_mutation_indeterminate"
	}
	if err := recordTelemetryJobEvent(ctx, tx, command.JobID, event, map[string]any{
		"operation_id": identity.ID, "phase": "apply",
		"error_sha256": workspaceMutationFailureSHA(message),
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit workspace mutation apply state %q: %w", status, err)
	}
	return nil
}

func (r *Repository) markWorkspaceMutationApplied(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command WorkspaceMutationCommand,
	identity workspaceMutationOperationIdentity,
) (int64, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin workspace mutation applied transition: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockWorkspaceMutationAuthorityTx(ctx, tx, authority, command, false); err != nil {
		return 0, err
	}
	record, err := lockWorkspaceMutationOperationTx(ctx, tx, identity.ID)
	if err != nil {
		return 0, err
	}
	if err := requireWorkspaceMutationIdentity(record, identity); err != nil {
		return 0, err
	}
	if record.MutationEvidenceID != nil {
		if err := requireWorkspaceMutationEvidenceTx(
			ctx, tx, command, identity.ID, *record.MutationEvidenceID,
		); err != nil {
			return 0, err
		}
		if err := tx.Commit(ctx); err != nil {
			return 0, fmt.Errorf("commit workspace mutation applied replay: %w", err)
		}
		return *record.MutationEvidenceID, nil
	}
	evidenceID, err := insertWorkspaceMutationEvidenceTx(ctx, tx, command, identity.ID)
	if err != nil {
		return 0, err
	}
	result, err := tx.Exec(ctx, `
		UPDATE workspace_mutation_operations
		SET status=$2,indeterminate_phase=NULL,mutation_evidence_id=$3,
		    applied_at=clock_timestamp(),last_error=NULL,
		    current_step_attempt=$4,current_worker_id=$5,updated_at=clock_timestamp()
		WHERE id=$1 AND status IN ($6,$7,$8)
	`, identity.ID, workspaceMutationApplied, evidenceID,
		authority.Attempt, authority.WorkerID, workspaceMutationPrepared,
		workspaceMutationApplying, workspaceMutationIndeterminateState)
	if err != nil {
		return 0, fmt.Errorf("mark workspace mutation applied: %w", err)
	}
	if result.RowsAffected() != 1 {
		return 0, fmt.Errorf("workspace mutation %s lost applied authority", identity.ID)
	}
	if err := recordTelemetryJobEvent(ctx, tx, command.JobID, "workspace_mutation_applied", map[string]any{
		"operation_id": identity.ID, "evidence_id": evidenceID,
		"changed_file_count": len(command.Plan.Files),
	}); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit workspace mutation applied transition: %w", err)
	}
	return evidenceID, nil
}
