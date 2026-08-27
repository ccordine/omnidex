package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// ExecuteWorkspaceMutation is the sole durable filesystem transaction path.
// It observes reality before and after every side effect, applies only from an
// exact source state, and seals verification evidence before terminal state.
func (r *Repository) ExecuteWorkspaceMutation(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command WorkspaceMutationCommand,
	callbacks WorkspaceMutationCallbacks,
) (result WorkspaceMutationResult, resultErr error) {
	if ctx == nil {
		return WorkspaceMutationResult{}, fmt.Errorf("workspace mutation requires a context")
	}
	if err := ctx.Err(); err != nil {
		return WorkspaceMutationResult{}, fmt.Errorf("workspace mutation: %w", err)
	}
	if err := validateWorkspaceMutationCommand(command); err != nil {
		return WorkspaceMutationResult{}, err
	}
	if err := validateWorkspaceMutationExecutionAuthority(authority, command); err != nil {
		return WorkspaceMutationResult{}, err
	}
	if err := validateWorkspaceMutationCallbacks(callbacks); err != nil {
		return WorkspaceMutationResult{}, err
	}
	if r == nil || r.pool == nil {
		return WorkspaceMutationResult{}, ErrRepositoryNotConfigured
	}
	identity, err := workspaceMutationOperation(command)
	if err != nil {
		return WorkspaceMutationResult{}, err
	}
	lock, err := acquireWorkspaceMutationLock(ctx, r.pool, command)
	if err != nil {
		return WorkspaceMutationResult{}, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, releaseWorkspaceMutationLock(lock, command))
	}()
	record, err := r.prepareWorkspaceMutation(ctx, authority, command, identity)
	if err != nil {
		return WorkspaceMutationResult{}, err
	}
	switch record.Status {
	case workspaceMutationVerified, workspaceMutationVerificationFailed:
		return r.replayWorkspaceMutationTerminal(ctx, authority, command, identity)
	case workspaceMutationApplied, workspaceMutationVerifying:
		return r.driveWorkspaceMutationVerification(ctx, authority, command, identity, callbacks)
	case workspaceMutationIndeterminateState:
		if record.IndeterminatePhase != nil && *record.IndeterminatePhase == "verification" {
			return r.driveWorkspaceMutationVerification(ctx, authority, command, identity, callbacks)
		}
		return r.driveWorkspaceMutationApply(ctx, authority, command, identity, record.Status, callbacks)
	case workspaceMutationPrepared, workspaceMutationApplying:
		return r.driveWorkspaceMutationApply(ctx, authority, command, identity, record.Status, callbacks)
	default:
		return WorkspaceMutationResult{}, fmt.Errorf("workspace mutation %s has invalid status %q", identity.ID, record.Status)
	}
}

func (r *Repository) driveWorkspaceMutationApply(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command WorkspaceMutationCommand,
	identity workspaceMutationOperationIdentity,
	status string,
	callbacks WorkspaceMutationCallbacks,
) (WorkspaceMutationResult, error) {
	observation, err := observeWorkspaceMutation(ctx, callbacks, command)
	if err != nil {
		if status == workspaceMutationApplying {
			markErr := r.recordWorkspaceMutationApplyState(
				ctx, authority, command, identity, workspaceMutationIndeterminateState, err,
			)
			return WorkspaceMutationResult{}, errors.Join(ErrWorkspaceMutationUnresolved, err, markErr)
		}
		return WorkspaceMutationResult{}, err
	}
	switch observation {
	case WorkspaceMutationPost:
		if _, err := r.markWorkspaceMutationApplied(ctx, authority, command, identity); err != nil {
			return WorkspaceMutationResult{}, err
		}
		return r.driveWorkspaceMutationVerification(ctx, authority, command, identity, callbacks)
	case WorkspaceMutationIndeterminate:
		reason := fmt.Errorf("complete workspace inventory matches neither source nor expected state")
		markErr := r.recordWorkspaceMutationApplyState(
			ctx, authority, command, identity, workspaceMutationIndeterminateState, reason,
		)
		return WorkspaceMutationResult{}, errors.Join(ErrWorkspaceMutationUnresolved, reason, markErr)
	case WorkspaceMutationSource:
		return r.applyWorkspaceMutationFromSource(ctx, authority, command, identity, callbacks)
	default:
		return WorkspaceMutationResult{}, fmt.Errorf("workspace mutation observer returned invalid state %q", observation)
	}
}

func (r *Repository) applyWorkspaceMutationFromSource(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command WorkspaceMutationCommand,
	identity workspaceMutationOperationIdentity,
	callbacks WorkspaceMutationCallbacks,
) (WorkspaceMutationResult, error) {
	if err := r.markWorkspaceMutationApplying(ctx, authority, command, identity); err != nil {
		return WorkspaceMutationResult{}, err
	}
	applyErr := callbacks.Apply(ctx, command)
	observation, observeErr := observeWorkspaceMutation(ctx, callbacks, command)
	if observeErr != nil {
		reason := errors.Join(fmt.Errorf("observe workspace after apply: %w", observeErr), applyErr)
		markErr := r.recordWorkspaceMutationApplyState(
			ctx, authority, command, identity, workspaceMutationIndeterminateState, reason,
		)
		return WorkspaceMutationResult{}, errors.Join(ErrWorkspaceMutationUnresolved, reason, markErr)
	}
	switch observation {
	case WorkspaceMutationPost:
		if _, err := r.markWorkspaceMutationApplied(ctx, authority, command, identity); err != nil {
			return WorkspaceMutationResult{}, err
		}
		return r.driveWorkspaceMutationVerification(ctx, authority, command, identity, callbacks)
	case WorkspaceMutationSource:
		reason := applyErr
		if reason == nil {
			reason = fmt.Errorf("workspace apply callback returned success without producing exact expected state")
		} else {
			reason = fmt.Errorf("apply authoritative workspace mutation: %w", applyErr)
		}
		markErr := r.recordWorkspaceMutationApplyState(
			ctx, authority, command, identity, workspaceMutationPrepared, reason,
		)
		return WorkspaceMutationResult{}, errors.Join(reason, markErr)
	case WorkspaceMutationIndeterminate:
		reason := fmt.Errorf("workspace apply left a state matching neither source nor expected inventory")
		if applyErr != nil {
			reason = errors.Join(reason, fmt.Errorf("apply authoritative workspace mutation: %w", applyErr))
		}
		markErr := r.recordWorkspaceMutationApplyState(
			ctx, authority, command, identity, workspaceMutationIndeterminateState, reason,
		)
		return WorkspaceMutationResult{}, errors.Join(ErrWorkspaceMutationUnresolved, reason, markErr)
	default:
		return WorkspaceMutationResult{}, fmt.Errorf("workspace mutation observer returned invalid state %q", observation)
	}
}

func (r *Repository) replayWorkspaceMutationTerminal(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command WorkspaceMutationCommand,
	identity workspaceMutationOperationIdentity,
) (WorkspaceMutationResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return WorkspaceMutationResult{}, fmt.Errorf("begin terminal workspace mutation replay: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockWorkspaceMutationAuthorityTx(ctx, tx, authority, command, false); err != nil {
		return WorkspaceMutationResult{}, err
	}
	record, err := lockWorkspaceMutationOperationTx(ctx, tx, identity.ID)
	if err != nil {
		return WorkspaceMutationResult{}, err
	}
	if err := requireWorkspaceMutationIdentity(record, identity); err != nil {
		return WorkspaceMutationResult{}, err
	}
	if record.Status != workspaceMutationVerified && record.Status != workspaceMutationVerificationFailed {
		return WorkspaceMutationResult{}, fmt.Errorf("workspace mutation %s is not terminal", identity.ID)
	}
	return workspaceMutationTerminalResult(ctx, tx, record)
}
