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
	record, err := r.prepareRepositoryMutation(ctx, command, identity)
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
		return r.finalizeRepositoryMutation(ctx, command, identity)
	}
	return r.driveRepositoryMutation(ctx, command, identity, state, classify, apply)
}

func (r *Repository) driveRepositoryMutation(
	ctx context.Context,
	command RepositoryMutationCommand,
	identity repositoryMutationOperationIdentity,
	state RepositoryMutationState,
	classify RepositoryMutationClassifier,
	apply func(context.Context) error,
) error {
	switch state {
	case RepositoryMutationPost:
		return r.finalizeRepositoryMutation(ctx, command, identity)
	case RepositoryMutationIndeterminate:
		reason := fmt.Errorf("complete repository inventory matches neither source nor exact post state")
		if err := r.recordRepositoryMutationState(
			ctx, command, identity, repositoryMutationIndeterminate, reason,
		); err != nil {
			return errors.Join(ErrRepositoryMutationUnresolved, reason, err)
		}
		return fmt.Errorf("%w: %s", ErrRepositoryMutationUnresolved, reason)
	case RepositoryMutationSource:
		return r.applyPreparedRepositoryMutation(ctx, command, identity, classify, apply)
	default:
		return fmt.Errorf("repository mutation classifier returned invalid state %q", state)
	}
}

func (r *Repository) applyPreparedRepositoryMutation(
	ctx context.Context,
	command RepositoryMutationCommand,
	identity repositoryMutationOperationIdentity,
	classify RepositoryMutationClassifier,
	apply func(context.Context) error,
) error {
	if err := r.markRepositoryMutationApplying(ctx, command, identity); err != nil {
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
		return r.finalizeRepositoryMutation(ctx, command, identity)
	case RepositoryMutationSource:
		reason := applyErr
		if reason == nil {
			reason = fmt.Errorf("repository mutation callback returned success without producing exact post state")
		} else {
			reason = fmt.Errorf("apply authoritative repository mutation: %w", applyErr)
		}
		if err := r.recordRepositoryMutationState(
			ctx, command, identity, repositoryMutationPrepared, reason,
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
			ctx, command, identity, repositoryMutationIndeterminate, reason,
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
	command RepositoryMutationCommand,
) error {
	var jobStatus string
	var currentGeneration int64
	if err := tx.QueryRow(ctx, `
		SELECT status, current_generation
		FROM jobs WHERE id=$1
		FOR UPDATE
	`, command.JobID).Scan(&jobStatus, &currentGeneration); err != nil {
		return fmt.Errorf("lock repository mutation job %d: %w", command.JobID, err)
	}
	if currentGeneration != command.Generation {
		return fmt.Errorf(
			"%w: repository mutation step %d generation %d is not job %d generation %d",
			ErrStaleJobGeneration, command.StepID, command.Generation, command.JobID, currentGeneration,
		)
	}
	if jobStatus != model.JobStatusRunning {
		return fmt.Errorf(
			"%w: repository mutation job %d status is %q, expected %q",
			ErrStepNotWritable, command.JobID, jobStatus, model.JobStatusRunning,
		)
	}
	var stepStatus, workerID string
	var stepGeneration int64
	var supersededAt *int64
	err := tx.QueryRow(ctx, `
		SELECT status, generation, superseded_at_generation, COALESCE(worker_id, '')
		FROM job_steps
		WHERE id=$1 AND job_id=$2
		FOR UPDATE
	`, command.StepID, command.JobID).Scan(
		&stepStatus, &stepGeneration, &supersededAt, &workerID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf(
			"%w: repository mutation step %d does not belong to job %d",
			ErrStaleJobGeneration, command.StepID, command.JobID,
		)
	}
	if err != nil {
		return fmt.Errorf("lock repository mutation step %d: %w", command.StepID, err)
	}
	if supersededAt != nil || stepGeneration != currentGeneration {
		return fmt.Errorf(
			"%w: repository mutation step %d generation %d is not current generation %d",
			ErrStaleJobGeneration, command.StepID, stepGeneration, currentGeneration,
		)
	}
	if stepStatus != model.StepStatusRunning {
		return fmt.Errorf(
			"%w: repository mutation step %d status is %q, expected %q",
			ErrStepNotWritable, command.StepID, stepStatus, model.StepStatusRunning,
		)
	}
	if workerID != command.WorkerID {
		return fmt.Errorf(
			"%w: repository mutation step %d belongs to worker %q, not %q",
			ErrStepNotWritable, command.StepID, workerID, command.WorkerID,
		)
	}
	return nil
}
