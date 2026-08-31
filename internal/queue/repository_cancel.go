package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CancelJob(ctx context.Context, command CancelJobCommand) (LifecycleJobResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return LifecycleJobResult{}, err
	}
	defer rollbackTx(ctx, tx, "job cancellation")

	result, err := cancelJobTx(ctx, tx, command)
	if err != nil {
		return LifecycleJobResult{}, err
	}
	if result.Applied {
		if err := terminalizeScrumJobCardTx(ctx, tx, result.Job, model.JobStatusCanceled); err != nil {
			return LifecycleJobResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return LifecycleJobResult{}, err
	}
	return result, nil
}

func cancelJobTx(ctx context.Context, tx pgx.Tx, command CancelJobCommand) (LifecycleJobResult, error) {
	command, err := normalizeCancelJobCommand(command)
	if err != nil {
		return LifecycleJobResult{}, err
	}
	descriptor, err := describeLifecycleOperation(command.OperationID, LifecycleCancelJob, command)
	if err != nil {
		return LifecycleJobResult{}, err
	}
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
	record, found, err := loadLifecycleOperationTx(ctx, tx, descriptor, command.JobID)
	if err != nil {
		return LifecycleJobResult{}, err
	}
	if found {
		if err := requireCancelJobReplayTx(ctx, tx, record, command, job); err != nil {
			return LifecycleJobResult{}, err
		}
		return LifecycleJobResult{Job: record.ResultJob}, nil
	}
	job, err = applyJobCancellationTx(ctx, tx, command, job)
	if err != nil {
		return LifecycleJobResult{}, err
	}
	if err := insertLifecycleOperationTx(ctx, tx, descriptor, lifecycleOperationRecord{
		ID: descriptor.ID, JobID: job.ID,
		ObservedGeneration: job.CurrentGeneration, ResultGeneration: job.CurrentGeneration,
		Kind: descriptor.Kind, CommandSHA256: descriptor.SHA256,
		ResultJobStatus: job.Status, ResultJob: job,
	}); err != nil {
		return LifecycleJobResult{}, err
	}
	return LifecycleJobResult{Job: job, Applied: true}, nil
}

func applyJobCancellationTx(
	ctx context.Context,
	tx pgx.Tx,
	command CancelJobCommand,
	job model.Job,
) (model.Job, error) {
	switch job.Status {
	case model.JobStatusCompleted, model.JobStatusFailed:
		return model.Job{}, fmt.Errorf("job is already %s", job.Status)
	case model.JobStatusCanceled:
		return model.Job{}, fmt.Errorf(
			"%w: job %d is already canceled under a different lifecycle operation",
			ErrStepNotWritable, job.ID,
		)
	}
	if err := requireObjectiveSessionContextAdmissionTx(
		ctx,
		tx,
		job,
		conversationFollowupCancellation,
		command.Reason,
	); err != nil {
		return model.Job{}, err
	}
	stepIDs, err := lockCurrentNonterminalStepIDsTx(ctx, tx, command.JobID, job.CurrentGeneration)
	if err != nil {
		return model.Job{}, err
	}
	if len(stepIDs) > 0 {
		if err := terminalizeStepAttemptsForAuthorityChangeTx(
			ctx, tx, command.JobID, job.CurrentGeneration, stepIDs, model.StepAttemptCanceled,
		); err != nil {
			return model.Job{}, err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2,
		    error = COALESCE(NULLIF(error, ''), $3),
		    finished_at = COALESCE(finished_at, NOW()),
		    updated_at = NOW()
		WHERE job_id = $1
		  AND superseded_at_generation IS NULL
		  AND status IN ($4, $5, $6)
	`, command.JobID, model.StepStatusCanceled, command.Reason, model.StepStatusPending, model.StepStatusRunning, model.StepStatusWaiting); err != nil {
		return model.Job{}, err
	}
	jobUpdate, err := tx.Exec(ctx, `
		UPDATE jobs
		SET status = $2, error = $3, completed_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, command.JobID, model.JobStatusCanceled, command.Reason)
	if err != nil {
		return model.Job{}, err
	}
	if jobUpdate.RowsAffected() != 1 {
		return model.Job{}, fmt.Errorf("job %d disappeared during cancellation", command.JobID)
	}
	row := tx.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata, current_generation, created_at, updated_at, completed_at
		FROM jobs
		WHERE id = $1
	`, command.JobID)

	job, err = scanJob(row)
	if err != nil {
		return model.Job{}, err
	}

	return job, nil
}
