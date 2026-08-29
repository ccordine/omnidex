package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
)

func (r *Repository) driveWorkspaceMutationVerification(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command WorkspaceMutationCommand,
	identity workspaceMutationOperationIdentity,
	callbacks WorkspaceMutationCallbacks,
) (WorkspaceMutationResult, error) {
	observation, err := observeWorkspaceMutation(ctx, callbacks, command)
	if err != nil {
		reason := fmt.Errorf("observe applied workspace before verification: %w", err)
		markErr := r.recordWorkspaceMutationVerificationState(
			ctx, authority, command, identity, workspaceMutationIndeterminateState, reason,
		)
		return WorkspaceMutationResult{}, errors.Join(ErrWorkspaceMutationUnresolved, reason, markErr)
	}
	if observation != WorkspaceMutationPost {
		reason := fmt.Errorf("applied workspace differs from exact expected state before verification: %s", observation)
		markErr := r.recordWorkspaceMutationVerificationState(
			ctx, authority, command, identity, workspaceMutationIndeterminateState, reason,
		)
		return WorkspaceMutationResult{}, errors.Join(ErrWorkspaceMutationUnresolved, reason, markErr)
	}
	if err := r.markWorkspaceMutationVerifying(ctx, authority, command, identity); err != nil {
		return WorkspaceMutationResult{}, err
	}
	verification, verifyErr := callbacks.Verify(ctx, command)
	if verifyErr == nil {
		verifyErr = validateWorkspaceMutationVerificationResult(command, identity.ID, verification)
	}
	after, observeErr := observeWorkspaceMutation(ctx, callbacks, command)
	if observeErr != nil || after != WorkspaceMutationPost {
		reason := fmt.Errorf("workspace changed during authoritative verification")
		if observeErr != nil {
			reason = errors.Join(reason, observeErr)
		} else {
			reason = fmt.Errorf("%w: observed %s", reason, after)
		}
		if verifyErr != nil {
			reason = errors.Join(reason, verifyErr)
		}
		markErr := r.recordWorkspaceMutationVerificationState(
			ctx, authority, command, identity, workspaceMutationIndeterminateState, reason,
		)
		return WorkspaceMutationResult{}, errors.Join(ErrWorkspaceMutationUnresolved, reason, markErr)
	}
	if verifyErr != nil {
		reason := fmt.Errorf("execute workspace verification infrastructure: %w", verifyErr)
		markErr := r.recordWorkspaceMutationVerificationState(
			ctx, authority, command, identity, workspaceMutationApplied, reason,
		)
		return WorkspaceMutationResult{}, errors.Join(reason, markErr)
	}
	return r.finalizeWorkspaceMutationVerification(
		ctx, authority, command, identity, verification,
	)
}
