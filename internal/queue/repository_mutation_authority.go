package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// ApplyRepositoryMutation serializes one code-owned filesystem mutation with
// replan and interrupt. It deliberately exposes no transaction or generic
// database callback authority.
func (r *Repository) ApplyRepositoryMutation(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command RepositoryMutationCommand,
	classify RepositoryMutationClassifier,
	apply func(context.Context) error,
) (resultErr error) {
	if ctx == nil {
		return fmt.Errorf("repository mutation requires a context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("repository mutation: %w", err)
	}
	if apply == nil {
		return fmt.Errorf("repository mutation requires one code callback")
	}
	if classify == nil {
		return fmt.Errorf("repository mutation requires one complete-inventory classifier")
	}
	if err := validateRepositoryMutationCommand(command); err != nil {
		return err
	}
	if err := validateRepositoryMutationExecutionAuthority(authority, command); err != nil {
		return err
	}
	if r == nil || r.pool == nil {
		return ErrRepositoryNotConfigured
	}
	identity, err := repositoryMutationOperation(command)
	if err != nil {
		return err
	}
	lock, err := acquireRepositoryMutationLock(ctx, r.pool, command)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, releaseRepositoryMutationLock(lock, command))
	}()
	record, err := r.prepareRepositoryMutation(ctx, authority, command, identity)
	if err != nil {
		return err
	}
	state, err := classifyRepositoryMutation(ctx, classify, command)
	if err != nil {
		return err
	}
	if record.Status == repositoryMutationApplied {
		if state != RepositoryMutationPost {
			return fmt.Errorf(
				"%w: applied operation %s no longer matches its exact post state",
				ErrRepositoryMutationUnresolved, identity.ID,
			)
		}
		return r.finalizeRepositoryMutation(ctx, authority, command, identity)
	}
	return r.driveRepositoryMutation(ctx, authority, command, identity, state, classify, apply)
}

func validateRepositoryMutationExecutionAuthority(
	authority model.StepAttemptAuthority,
	command RepositoryMutationCommand,
) error {
	if err := validateStepAttemptAuthority(authority); err != nil {
		return err
	}
	if authority.JobID != command.JobID || authority.Generation != command.Generation ||
		authority.StepID != command.StepID {
		return staleStepAttemptError(authority, "repository mutation owner disagrees with command", nil)
	}
	return nil
}

func (r *Repository) driveRepositoryMutation(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command RepositoryMutationCommand,
	identity repositoryMutationOperationIdentity,
	state RepositoryMutationState,
	classify RepositoryMutationClassifier,
	apply func(context.Context) error,
) error {
	switch state {
	case RepositoryMutationPost:
		return r.finalizeRepositoryMutation(ctx, authority, command, identity)
	case RepositoryMutationIndeterminate:
		reason := fmt.Errorf("complete repository inventory matches neither source nor exact post state")
		if err := r.recordRepositoryMutationState(
			ctx, authority, command, identity, repositoryMutationIndeterminate, reason,
		); err != nil {
			return errors.Join(ErrRepositoryMutationUnresolved, reason, err)
		}
		return fmt.Errorf("%w: %s", ErrRepositoryMutationUnresolved, reason)
	case RepositoryMutationSource:
		return r.applyPreparedRepositoryMutation(ctx, authority, command, identity, classify, apply)
	default:
		return fmt.Errorf("repository mutation classifier returned invalid state %q", state)
	}
}

func (r *Repository) applyPreparedRepositoryMutation(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	command RepositoryMutationCommand,
	identity repositoryMutationOperationIdentity,
	classify RepositoryMutationClassifier,
	apply func(context.Context) error,
) error {
	if err := r.markRepositoryMutationApplying(ctx, authority, command, identity); err != nil {
		return err
	}
	applyErr := apply(ctx)
	state, classifyErr := classifyRepositoryMutation(ctx, classify, command)
	if classifyErr != nil {
		if applyErr != nil {
			return errors.Join(fmt.Errorf("apply authoritative repository mutation: %w", applyErr), classifyErr)
		}
		return classifyErr
	}
	switch state {
	case RepositoryMutationPost:
		return r.finalizeRepositoryMutation(ctx, authority, command, identity)
	case RepositoryMutationSource:
		reason := applyErr
		if reason == nil {
			reason = fmt.Errorf("repository mutation callback returned success without producing exact post state")
		} else {
			reason = fmt.Errorf("apply authoritative repository mutation: %w", applyErr)
		}
		if err := r.recordRepositoryMutationState(
			ctx, authority, command, identity, repositoryMutationPrepared, reason,
		); err != nil {
			return errors.Join(reason, err)
		}
		return reason
	case RepositoryMutationIndeterminate:
		reason := fmt.Errorf("repository mutation left a state matching neither source nor exact post inventory")
		if applyErr != nil {
			reason = errors.Join(reason, fmt.Errorf("apply authoritative repository mutation: %w", applyErr))
		}
		if err := r.recordRepositoryMutationState(
			ctx, authority, command, identity, repositoryMutationIndeterminate, reason,
		); err != nil {
			return errors.Join(ErrRepositoryMutationUnresolved, reason, err)
		}
		return errors.Join(ErrRepositoryMutationUnresolved, reason)
	default:
		return fmt.Errorf("repository mutation classifier returned invalid state %q", state)
	}
}

func classifyRepositoryMutation(
	ctx context.Context,
	classify RepositoryMutationClassifier,
	command RepositoryMutationCommand,
) (RepositoryMutationState, error) {
	state, err := classify(ctx, command)
	if err != nil {
		return "", fmt.Errorf("classify complete repository mutation state: %w", err)
	}
	switch state {
	case RepositoryMutationSource, RepositoryMutationPost, RepositoryMutationIndeterminate:
		return state, nil
	default:
		return "", fmt.Errorf("repository mutation classifier returned invalid state %q", state)
	}
}

func lockRepositoryMutationAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	command RepositoryMutationCommand,
	requireOrigin bool,
) error {
	if err := validateRepositoryMutationExecutionAuthority(authority, command); err != nil {
		return err
	}
	if requireOrigin && authority != command.stepAttemptAuthority() {
		return staleStepAttemptError(authority, "new repository mutation origin disagrees with executor", nil)
	}
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return staleStepAttemptError(authority, "repository mutation executor is not running", nil)
	}
	return nil
}
