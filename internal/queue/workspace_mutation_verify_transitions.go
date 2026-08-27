package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) markWorkspaceMutationVerifying(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command WorkspaceMutationCommand,
	identity workspaceMutationOperationIdentity,
) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin workspace mutation verifying transition: %w", err)
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
		    verification_attempt_count=verification_attempt_count+1,
		    verifying_at=clock_timestamp(),current_step_attempt=$3,
		    current_worker_id=$4,updated_at=clock_timestamp()
		WHERE id=$1 AND status IN ($5,$6,$7)
		RETURNING verification_attempt_count
	`, identity.ID, workspaceMutationVerifying, authority.Attempt, authority.WorkerID,
		workspaceMutationApplied, workspaceMutationVerifying,
		workspaceMutationIndeterminateState).Scan(&attemptCount)
	if err != nil {
		return fmt.Errorf("mark workspace mutation verifying: %w", err)
	}
	if attemptCount <= 0 {
		return fmt.Errorf("workspace mutation %s returned invalid verification attempt count", identity.ID)
	}
	if err := recordTelemetryJobEvent(ctx, tx, command.JobID, "workspace_mutation_verifying", map[string]any{
		"operation_id": identity.ID, "attempt_count": attemptCount,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit workspace mutation verifying transition: %w", err)
	}
	return nil
}

func (r *Repository) recordWorkspaceMutationVerificationState(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command WorkspaceMutationCommand,
	identity workspaceMutationOperationIdentity,
	status string,
	reason error,
) error {
	if status != workspaceMutationApplied && status != workspaceMutationIndeterminateState {
		return fmt.Errorf("workspace mutation verification state %q is invalid", status)
	}
	message, err := exactWorkspaceMutationError(reason)
	if err != nil {
		return err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin workspace mutation verification-state transition: %w", err)
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
		phase = "verification"
	}
	result, err := tx.Exec(ctx, `
		UPDATE workspace_mutation_operations
		SET status=$2,indeterminate_phase=$3,last_error=$4,
		    current_step_attempt=$5,current_worker_id=$6,updated_at=clock_timestamp()
		WHERE id=$1
	`, identity.ID, status, phase, message, authority.Attempt, authority.WorkerID)
	if err != nil {
		return fmt.Errorf("record workspace mutation verification state %q: %w", status, err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("workspace mutation %s lost verification-state authority", identity.ID)
	}
	event := "workspace_mutation_verification_deferred"
	if status == workspaceMutationIndeterminateState {
		event = "workspace_mutation_indeterminate"
	}
	if err := recordTelemetryJobEvent(ctx, tx, command.JobID, event, map[string]any{
		"operation_id": identity.ID, "phase": "verification",
		"error_sha256": workspaceMutationFailureSHA(message),
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit workspace mutation verification state %q: %w", status, err)
	}
	return nil
}
