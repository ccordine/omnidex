package queue

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) PauseStepForInput(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	stepOutput string,
	question string,
	extraContexts map[string]string,
) error {
	stepOutput = SanitizeUTF8Text(stepOutput)
	question = SanitizeUTF8Text(question)
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return staleStepAttemptError(authority, fmt.Sprintf(
			"input-pause writer job status %q step status %q", jobStatus, stepStatus,
		), nil)
	}
	if err := rejectUnresolvedGeneratedWorkloadDeploymentsTx(
		ctx, tx, authority.JobID,
	); err != nil {
		return err
	}
	if err := terminalizeStepAttemptTx(ctx, tx, authority, model.StepAttemptWaiting); err != nil {
		return err
	}

	stepUpdate, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2, output = $3, updated_at = NOW()
		WHERE id = $1 AND job_id=$5 AND generation=$6 AND current_attempt=$7
		  AND worker_id=$8 AND status = $4
	`, authority.StepID, model.StepStatusWaiting, stepOutput, model.StepStatusRunning,
		authority.JobID, authority.Generation, authority.Attempt, authority.WorkerID)
	if err != nil {
		return err
	}
	if stepUpdate.RowsAffected() == 0 {
		return staleStepAttemptError(authority, "input-pause target lost current authority", nil)
	}

	if strings.TrimSpace(question) != "" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO step_contexts (step_id, key, value)
			VALUES ($1, $2, $3)
		`, authority.StepID, "input_question", strings.TrimSpace(question)); err != nil {
			return err
		}
	}

	for key, value := range extraContexts {
		k := SanitizeUTF8Text(strings.TrimSpace(key))
		if k == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO step_contexts (step_id, key, value)
			VALUES ($1, $2, $3)
			`, authority.StepID, k, SanitizeUTF8Text(value)); err != nil {
			return err
		}
	}

	jobUpdate, err := tx.Exec(ctx, `
		UPDATE jobs
		SET status = $2, updated_at = NOW(), error = NULL
		WHERE id = $1 AND status IN ($3, $4, $5)
	`, authority.JobID, model.JobStatusWaiting, model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting)
	if err != nil {
		return err
	}
	if jobUpdate.RowsAffected() != 1 {
		return staleStepAttemptError(authority, "job lost input-pause authority", nil)
	}

	return tx.Commit(ctx)
}

func (r *Repository) SubmitJobFeedback(ctx context.Context, command SubmitJobFeedbackCommand) (model.Job, error) {
	command, err := normalizeSubmitFeedbackCommand(command)
	if err != nil {
		return model.Job{}, err
	}
	descriptor, err := describeLifecycleOperation(command.OperationID, LifecycleSubmitFeedback, command)
	if err != nil {
		return model.Job{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Job{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockLifecycleOperationIdentityTx(ctx, tx, command.OperationID); err != nil {
		return model.Job{}, err
	}
	job, err := submitJobFeedbackTx(ctx, tx, command, descriptor)
	if err != nil {
		return model.Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Job{}, err
	}
	return job, nil
}

func submitJobFeedbackTx(
	ctx context.Context,
	tx pgx.Tx,
	command SubmitJobFeedbackCommand,
	descriptor lifecycleOperationDescriptor,
) (model.Job, error) {
	job, err := lockedJobTx(ctx, tx, command.JobID)
	if err != nil {
		return model.Job{}, err
	}
	if existing, found, err := loadLifecycleOperationTx(ctx, tx, descriptor, command.JobID); err != nil {
		return model.Job{}, err
	} else if found {
		if err := requireSubmitFeedbackReplayTx(ctx, tx, existing, command); err != nil {
			return model.Job{}, err
		}
		return existing.ResultJob, nil
	}
	if job.Status != model.JobStatusWaiting {
		return model.Job{}, fmt.Errorf(
			"%w: job %d status is %q, expected %q",
			ErrStepNotWritable, job.ID, job.Status, model.JobStatusWaiting,
		)
	}

	var stepID int64
	err = tx.QueryRow(ctx, `
		SELECT job_steps.id
		FROM job_steps, jobs
		WHERE job_steps.job_id = $1
		  AND jobs.id = job_steps.job_id
		  AND job_steps.status = $2
		  AND job_steps.superseded_at_generation IS NULL
		  AND job_steps.generation = jobs.current_generation
		ORDER BY job_steps.sort_index ASC, job_steps.id ASC
		FOR UPDATE OF job_steps
		LIMIT 1
	`, command.JobID, model.StepStatusWaiting).Scan(&stepID)
	if err != nil {
		return model.Job{}, err
	}

	stepUpdate, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2, output = $3, finished_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, stepID, model.StepStatusCompleted, command.Feedback)
	if err != nil {
		return model.Job{}, err
	}
	if stepUpdate.RowsAffected() != 1 {
		return model.Job{}, fmt.Errorf("%w: waiting step %d disappeared", ErrStepNotWritable, stepID)
	}

	var contextID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO step_contexts (step_id, key, value)
		VALUES ($1, $2, $3)
		RETURNING id
	`, stepID, "user_feedback", command.Feedback).Scan(&contextID); err != nil {
		return model.Job{}, err
	}

	var openSteps int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_steps
		WHERE job_id = $1
		  AND superseded_at_generation IS NULL
		  AND status IN ($2, $3, $4)
	`, command.JobID, model.StepStatusPending, model.StepStatusRunning, model.StepStatusWaiting).Scan(&openSteps); err != nil {
		return model.Job{}, err
	}

	if openSteps == 0 {
		if err := transitionInitialTaskRootTx(
			ctx, tx, command.JobID, job.CurrentGeneration, stepID, taskstate.NodeDone, command.Feedback, "",
		); err != nil {
			return model.Job{}, err
		}
		jobUpdate, err := tx.Exec(ctx, `
			UPDATE jobs
			SET status = $2, result = COALESCE(NULLIF($3, ''), result), completed_at = NOW(), updated_at = NOW(), error = NULL
			WHERE id = $1 AND status IN ($4, $5, $6)
		`, command.JobID, model.JobStatusCompleted, command.Feedback, model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting)
		if err != nil {
			return model.Job{}, err
		}
		if jobUpdate.RowsAffected() != 1 {
			return model.Job{}, fmt.Errorf("%w: job %d did not accept feedback completion", ErrStepNotWritable, command.JobID)
		}
		if err := terminalizeTaskLedgerTx(
			ctx, tx, command.JobID, job.CurrentGeneration, model.JobStatusCompleted, &stepID,
			"job completed after its final waiting step received user feedback",
		); err != nil {
			return model.Job{}, err
		}
		if err := completeTelemetryRunForJob(ctx, tx, command.JobID, model.JobStatusCompleted, map[string]any{"job_id": command.JobID, "result": command.Feedback}, map[string]any{"user_feedback_step_id": stepID}); err != nil {
			return model.Job{}, err
		}
		if err := recordTelemetryJobEvent(ctx, tx, command.JobID, "run_completed", map[string]any{"job_id": command.JobID, "step_id": stepID, "source": "user_feedback"}); err != nil {
			return model.Job{}, err
		}
	} else {
		jobUpdate, err := tx.Exec(ctx, `
			UPDATE jobs
			SET status = $2, updated_at = NOW(), error = NULL
			WHERE id = $1 AND status IN ($3, $4, $5)
		`, command.JobID, model.JobStatusRunning, model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting)
		if err != nil {
			return model.Job{}, err
		}
		if jobUpdate.RowsAffected() != 1 {
			return model.Job{}, fmt.Errorf("%w: job %d did not resume after feedback", ErrStepNotWritable, command.JobID)
		}
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
	stepStatus := model.StepStatusCompleted
	if err := insertLifecycleOperationTx(ctx, tx, descriptor, lifecycleOperationRecord{
		ID: descriptor.ID, JobID: command.JobID, ObservedGeneration: job.CurrentGeneration,
		ResultGeneration: job.CurrentGeneration, StepID: &stepID, StepContextID: &contextID,
		Kind: descriptor.Kind, CommandSHA256: descriptor.SHA256,
		ResultJobStatus: job.Status, ResultStepStatus: &stepStatus, ResultJob: job,
	}); err != nil {
		return model.Job{}, err
	}
	return job, nil
}
