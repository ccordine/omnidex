package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) SubmitJobFeedback(ctx context.Context, command SubmitJobFeedbackCommand) (LifecycleJobResult, error) {
	command, err := normalizeSubmitFeedbackCommand(command)
	if err != nil {
		return LifecycleJobResult{}, err
	}
	descriptor, err := describeLifecycleOperation(command.OperationID, LifecycleSubmitFeedback, command)
	if err != nil {
		return LifecycleJobResult{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return LifecycleJobResult{}, err
	}
	defer tx.Rollback(ctx)
	result, err := submitJobFeedbackTx(ctx, tx, command, descriptor)
	if err != nil {
		return LifecycleJobResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LifecycleJobResult{}, err
	}
	return result, nil
}

func submitJobFeedbackTx(
	ctx context.Context,
	tx pgx.Tx,
	command SubmitJobFeedbackCommand,
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
		if err := requireSubmitFeedbackReplayTx(ctx, tx, existing, command); err != nil {
			return LifecycleJobResult{}, err
		}
		return LifecycleJobResult{Job: existing.ResultJob}, nil
	}
	job, stepID, err := applyJobFeedbackTx(ctx, tx, command, job)
	if err != nil {
		return LifecycleJobResult{}, err
	}
	stepStatus := model.StepStatusCompleted
	if err := insertLifecycleOperationTx(ctx, tx, descriptor, lifecycleOperationRecord{
		ID: descriptor.ID, JobID: command.JobID, ObservedGeneration: job.CurrentGeneration,
		ResultGeneration: job.CurrentGeneration, StepID: &stepID,
		Kind: descriptor.Kind, CommandSHA256: descriptor.SHA256,
		ResultJobStatus: job.Status, ResultStepStatus: &stepStatus, ResultJob: job,
	}); err != nil {
		return LifecycleJobResult{}, err
	}
	return LifecycleJobResult{Job: job, Applied: true}, nil
}

func applyJobFeedbackTx(
	ctx context.Context,
	tx pgx.Tx,
	command SubmitJobFeedbackCommand,
	job model.Job,
) (model.Job, int64, error) {
	if job.Status != model.JobStatusWaiting {
		return model.Job{}, 0, fmt.Errorf(
			"%w: job %d status is %q, expected %q",
			ErrStepNotWritable, job.ID, job.Status, model.JobStatusWaiting,
		)
	}
	var generationPurpose string
	if err := tx.QueryRow(ctx, `
		SELECT purpose
		FROM job_generations
		WHERE job_id=$1 AND generation=$2
		FOR SHARE
		`, command.JobID, job.CurrentGeneration).Scan(&generationPurpose); err != nil {
		return model.Job{}, 0, fmt.Errorf("read waiting job %d generation purpose: %w", command.JobID, err)
	}
	switch generationPurpose {
	case jobGenerationPurposeInitial, jobGenerationPurposeReplan:
	case jobGenerationPurposeInterrupt:
		return model.Job{}, 0, fmt.Errorf(
			"%w: %w: job %d generation %d cannot accept generic feedback",
			ErrStepNotWritable,
			ErrInterruptedJobRequiresReplan,
			job.ID,
			job.CurrentGeneration,
		)
	default:
		return model.Job{}, 0, fmt.Errorf(
			"%w: job %d generation %d has unregistered purpose %q",
			ErrInvalidJobGeneration,
			job.ID,
			job.CurrentGeneration,
			generationPurpose,
		)
	}
	if err := requireObjectiveSessionContextAdmissionTx(
		ctx,
		tx,
		job,
		conversationFollowupUserTurn,
		command.Feedback,
	); err != nil {
		return model.Job{}, 0, err
	}

	var stepID int64
	var stepAction string
	err := tx.QueryRow(ctx, `
		SELECT job_steps.id,job_steps.action
		FROM job_steps, jobs
		WHERE job_steps.job_id = $1
		  AND jobs.id = job_steps.job_id
		  AND job_steps.status = $2
		  AND job_steps.superseded_at_generation IS NULL
		  AND job_steps.generation = jobs.current_generation
		ORDER BY job_steps.sort_index ASC, job_steps.id ASC
		FOR UPDATE OF job_steps
		LIMIT 1
	`, command.JobID, model.StepStatusWaiting).Scan(&stepID, &stepAction)
	if err != nil {
		return model.Job{}, 0, err
	}
	if stepAction == "v3_coding_plan" {
		return model.Job{}, 0, fmt.Errorf(
			"%w: coding plan review accepts only persisted plan decisions, freeze, or same-job replan",
			ErrStepNotWritable,
		)
	}

	stepUpdate, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2, output = $3, finished_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, stepID, model.StepStatusCompleted, command.Feedback)
	if err != nil {
		return model.Job{}, 0, err
	}
	if stepUpdate.RowsAffected() != 1 {
		return model.Job{}, 0, fmt.Errorf("%w: waiting step %d disappeared", ErrStepNotWritable, stepID)
	}

	var openSteps int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_steps
		WHERE job_id = $1
		  AND superseded_at_generation IS NULL
		  AND status IN ($2, $3, $4)
	`, command.JobID, model.StepStatusPending, model.StepStatusRunning, model.StepStatusWaiting).Scan(&openSteps); err != nil {
		return model.Job{}, 0, err
	}

	if openSteps == 0 {
		jobUpdate, err := tx.Exec(ctx, `
			UPDATE jobs
			SET status = $2, result = COALESCE(NULLIF($3, ''), result), completed_at = NOW(), updated_at = NOW(), error = NULL
			WHERE id = $1 AND status IN ($4, $5, $6)
			`, command.JobID, model.JobStatusCompleted, command.Feedback, model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting)
		if err != nil {
			return model.Job{}, 0, err
		}
		if jobUpdate.RowsAffected() != 1 {
			return model.Job{}, 0, fmt.Errorf("%w: job %d did not accept feedback completion", ErrStepNotWritable, command.JobID)
		}
	} else {
		jobUpdate, err := tx.Exec(ctx, `
			UPDATE jobs
			SET status = $2, updated_at = NOW(), error = NULL
			WHERE id = $1 AND status IN ($3, $4, $5)
			`, command.JobID, model.JobStatusRunning, model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting)
		if err != nil {
			return model.Job{}, 0, err
		}
		if jobUpdate.RowsAffected() != 1 {
			return model.Job{}, 0, fmt.Errorf("%w: job %d did not resume after feedback", ErrStepNotWritable, command.JobID)
		}
	}

	row := tx.QueryRow(ctx, `
		SELECT id, instruction, pipeline, status, result, error, metadata, current_generation, created_at, updated_at, completed_at
		FROM jobs
		WHERE id = $1
	`, command.JobID)

	job, err = scanJob(row)
	if err != nil {
		return model.Job{}, 0, err
	}
	return job, stepID, nil
}
