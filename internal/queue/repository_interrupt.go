package queue

import (
	"context"
	"fmt"
	"math"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// InterruptJob retires the active generation and installs a new, immutable
// waiting boundary on the same job. Only an explicit replan may replace that
// boundary with runnable work.
func (r *Repository) InterruptJob(ctx context.Context, command ReplanJobCommand) (LifecycleJobResult, error) {
	command, feedbackSHA, err := normalizeInterruptJobCommand(command)
	if err != nil {
		return LifecycleJobResult{}, err
	}
	descriptor, err := describeLifecycleOperation(command.OperationID, LifecycleInterruptJob, command)
	if err != nil {
		return LifecycleJobResult{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return LifecycleJobResult{}, fmt.Errorf("begin interrupt for job %d: %w", command.JobID, err)
	}
	defer tx.Rollback(ctx)

	result, err := interruptJobTx(ctx, tx, command, feedbackSHA, descriptor)
	if err != nil {
		return LifecycleJobResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LifecycleJobResult{}, fmt.Errorf(
			"commit interrupted generation %d for job %d: %w",
			result.Job.CurrentGeneration,
			command.JobID,
			err,
		)
	}
	return result, nil
}

func interruptJobTx(
	ctx context.Context,
	tx pgx.Tx,
	command ReplanJobCommand,
	feedbackSHA string,
	descriptor lifecycleOperationDescriptor,
) (LifecycleJobResult, error) {
	if err := lockLifecycleOperationIdentityTx(ctx, tx, command.OperationID); err != nil {
		return LifecycleJobResult{}, err
	}
	job, err := lockedJobTx(ctx, tx, command.JobID)
	if err != nil {
		return LifecycleJobResult{}, err
	}
	if err := requireLifecycleWorkspaceAuthority(
		job,
		command.WorkspaceRoot,
		command.WorkspaceIdentity,
	); err != nil {
		return LifecycleJobResult{}, err
	}
	if existing, found, err := loadLifecycleOperationTx(ctx, tx, descriptor, command.JobID); err != nil {
		return LifecycleJobResult{}, err
	} else if found {
		if err := requireInterruptReplayTx(ctx, tx, existing, command, feedbackSHA); err != nil {
			return LifecycleJobResult{}, err
		}
		return LifecycleJobResult{Job: existing.ResultJob}, nil
	}
	if terminalJobStatus(job.Status) {
		return LifecycleJobResult{}, fmt.Errorf("job is already %s", job.Status)
	}
	if job.Status != model.JobStatusPending && job.Status != model.JobStatusRunning {
		return LifecycleJobResult{}, fmt.Errorf(
			"job %d status is %q; interruption requires active pending or running authority",
			job.ID,
			job.Status,
		)
	}
	if err := requireObjectiveSessionContextAdmissionTx(
		ctx,
		tx,
		job,
		conversationFollowupInterruption,
		command.Feedback,
	); err != nil {
		return LifecycleJobResult{}, err
	}
	currentGeneration, err := lockCurrentJobGenerationTx(ctx, tx, command.JobID)
	if err != nil {
		return LifecycleJobResult{}, err
	}
	if currentGeneration >= math.MaxInt64 {
		return LifecycleJobResult{}, fmt.Errorf(
			"%w: job %d exhausted generation capacity",
			ErrInvalidJobGeneration,
			command.JobID,
		)
	}
	if err := lockGenerationRecordTx(ctx, tx, command.JobID, currentGeneration); err != nil {
		return LifecycleJobResult{}, err
	}
	seeds, err := canonicalReplanStepsTx(ctx, tx, job)
	if err != nil {
		return LifecycleJobResult{}, fmt.Errorf("recompute canonical steps for job %d interruption: %w", command.JobID, err)
	}
	boundary, err := canonicalReplanTail(seeds)
	if err != nil {
		return LifecycleJobResult{}, fmt.Errorf("job %d cannot be interrupted: %w", command.JobID, err)
	}
	currentTail, err := lockCurrentReplanTailTx(ctx, tx, command.JobID, boundary.sortIndex)
	if err != nil {
		return LifecycleJobResult{}, err
	}
	retiringIDs, err := validateCurrentReplanTail(currentGeneration, boundary, currentTail)
	if err != nil {
		return LifecycleJobResult{}, fmt.Errorf("validate interrupt tail for job %d: %w", command.JobID, err)
	}
	if err := terminalizeStepAttemptsForAuthorityChangeTx(
		ctx,
		tx,
		command.JobID,
		currentGeneration,
		retiringIDs,
		model.StepAttemptSuperseded,
	); err != nil {
		return LifecycleJobResult{}, err
	}
	newGeneration := currentGeneration + 1
	if err := createInterruptGenerationTx(
		ctx,
		tx,
		command,
		feedbackSHA,
		currentGeneration,
		newGeneration,
		boundary,
	); err != nil {
		return LifecycleJobResult{}, err
	}
	if err := retireReplanTailTx(ctx, tx, command.JobID, retiringIDs, newGeneration); err != nil {
		return LifecycleJobResult{}, err
	}
	if err := insertInterruptedTailTx(ctx, tx, command.JobID, newGeneration, boundary); err != nil {
		return LifecycleJobResult{}, err
	}
	job, err = advanceInterruptedJobTx(ctx, tx, command, currentGeneration, newGeneration)
	if err != nil {
		return LifecycleJobResult{}, err
	}
	if err := insertLifecycleOperationTx(ctx, tx, descriptor, lifecycleOperationRecord{
		ID: descriptor.ID, JobID: command.JobID, ObservedGeneration: currentGeneration,
		ResultGeneration: newGeneration, Kind: descriptor.Kind, CommandSHA256: descriptor.SHA256,
		ResultJobStatus: job.Status, ResultJob: job,
	}); err != nil {
		return LifecycleJobResult{}, err
	}
	return LifecycleJobResult{Job: job, Applied: true}, nil
}
