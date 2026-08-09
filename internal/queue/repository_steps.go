package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CompleteStep(ctx context.Context, command CompleteStepCommand) error {
	command, err := normalizeCompleteStepCommand(command)
	if err != nil {
		return err
	}
	descriptor, err := describeLifecycleOperation(command.OperationID, LifecycleCompleteStep, command)
	if err != nil {
		return err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockLifecycleOperationIdentityTx(ctx, tx, command.OperationID); err != nil {
		return err
	}

	jobID, err := stepJobIDTx(ctx, tx, command.StepID)
	if err != nil {
		return err
	}
	job, err := lockedJobTx(ctx, tx, jobID)
	if err != nil {
		return err
	}
	if existing, found, err := loadLifecycleOperationTx(ctx, tx, descriptor, jobID); err != nil {
		return err
	} else if found {
		return requireCompleteStepReplayTx(ctx, tx, existing, command)
	}
	jobStatus, err := lockCurrentGenerationStep(ctx, tx, jobID, command.StepID)
	if err != nil {
		return err
	}
	if !jobAcceptsStepTerminal(jobStatus) {
		return fmt.Errorf("%w: job %d status is %q", ErrStepNotWritable, jobID, jobStatus)
	}
	generation := job.CurrentGeneration

	stepUpdate, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2, output = $3, finished_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status IN ($4, $5)
	`, command.StepID, model.StepStatusCompleted, command.Output, model.StepStatusRunning, model.StepStatusWaiting)
	if err != nil {
		return err
	}
	if stepUpdate.RowsAffected() == 0 {
		return fmt.Errorf("%w: step %d is neither running nor waiting", ErrStepNotWritable, command.StepID)
	}

	var contextID *int64
	if command.ContextKey != "" {
		var insertedID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO step_contexts (step_id, key, value)
			VALUES ($1, $2, $3)
			RETURNING id
		`, command.StepID, command.ContextKey, command.ContextValue).Scan(&insertedID); err != nil {
			return err
		}
		contextID = &insertedID
	}

	var openSteps int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM job_steps
		WHERE job_id = $1
		  AND superseded_at_generation IS NULL
		  AND status IN ($2, $3, $4)
	`, jobID, model.StepStatusPending, model.StepStatusRunning, model.StepStatusWaiting).Scan(&openSteps); err != nil {
		return err
	}

	if openSteps == 0 {
		if err := transitionInitialTaskRootTx(
			ctx, tx, jobID, generation, command.StepID, taskstate.NodeDone, command.Output, "",
		); err != nil {
			return err
		}
		jobUpdate, err := tx.Exec(ctx, `
			UPDATE jobs
			SET status = $2, result = COALESCE(NULLIF($3, ''), result), completed_at = NOW(), updated_at = NOW()
			WHERE id = $1 AND status IN ($4, $5, $6)
		`, jobID, model.JobStatusCompleted, command.Output, model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting)
		if err != nil {
			return err
		}
		if jobUpdate.RowsAffected() != 1 {
			return fmt.Errorf("%w: job %d did not accept terminal completion", ErrStepNotWritable, jobID)
		}
		if err := terminalizeTaskLedgerTx(
			ctx, tx, jobID, generation, model.JobStatusCompleted, &command.StepID,
			"job completed after every current-generation step completed",
		); err != nil {
			return err
		}
		if err := completeTelemetryRunForJob(ctx, tx, jobID, model.JobStatusCompleted, map[string]any{"job_id": jobID, "result": command.Output}, map[string]any{"terminal_step_id": command.StepID, "context_key": command.ContextKey}); err != nil {
			return err
		}
		if err := recordTelemetryJobEvent(ctx, tx, jobID, "run_completed", map[string]any{"job_id": jobID, "step_id": command.StepID}); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE jobs
			SET updated_at = NOW()
			WHERE id = $1 AND status <> $2
		`, jobID, model.JobStatusCanceled); err != nil {
			return err
		}
	}
	job, err = scanLockedJobTx(ctx, tx, jobID)
	if err != nil {
		return err
	}
	stepStatus := model.StepStatusCompleted
	if err := insertLifecycleOperationTx(ctx, tx, descriptor, lifecycleOperationRecord{
		ID: descriptor.ID, JobID: jobID, ObservedGeneration: generation,
		ResultGeneration: job.CurrentGeneration, StepID: &command.StepID,
		StepContextID: contextID, Kind: descriptor.Kind, CommandSHA256: descriptor.SHA256,
		ResultJobStatus: job.Status, ResultStepStatus: &stepStatus, ResultJob: job,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) FailStep(ctx context.Context, command FailStepCommand) error {
	command, err := normalizeFailStepCommand(command)
	if err != nil {
		return err
	}
	descriptor, err := describeLifecycleOperation(command.OperationID, LifecycleFailStep, command)
	if err != nil {
		return err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockLifecycleOperationIdentityTx(ctx, tx, command.OperationID); err != nil {
		return err
	}

	jobID, err := stepJobIDTx(ctx, tx, command.StepID)
	if err != nil {
		return err
	}
	job, err := lockedJobTx(ctx, tx, jobID)
	if err != nil {
		return err
	}
	if existing, found, err := loadLifecycleOperationTx(ctx, tx, descriptor, jobID); err != nil {
		return err
	} else if found {
		return requireFailStepReplayTx(ctx, tx, existing, command)
	}
	jobStatus, err := lockCurrentGenerationStep(ctx, tx, jobID, command.StepID)
	if err != nil {
		return err
	}
	if !jobAcceptsStepTerminal(jobStatus) {
		return fmt.Errorf("%w: job %d status is %q", ErrStepNotWritable, jobID, jobStatus)
	}
	generation := job.CurrentGeneration

	stepUpdate, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2, error = $3, finished_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status IN ($4, $5)
	`, command.StepID, model.StepStatusFailed, command.Error, model.StepStatusRunning, model.StepStatusWaiting)
	if err != nil {
		return err
	}
	if stepUpdate.RowsAffected() == 0 {
		return fmt.Errorf("%w: step %d is neither running nor waiting", ErrStepNotWritable, command.StepID)
	}
	if err := transitionInitialTaskRootTx(
		ctx, tx, jobID, generation, command.StepID, taskstate.NodeFailed, "", command.Error,
	); err != nil {
		return err
	}

	jobUpdate, err := tx.Exec(ctx, `
		UPDATE jobs
		SET status = $2, error = $3, completed_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status IN ($4, $5, $6)
	`, jobID, model.JobStatusFailed, command.Error, model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting)
	if err != nil {
		return err
	}
	if jobUpdate.RowsAffected() != 1 {
		return fmt.Errorf("%w: job %d did not accept terminal failure", ErrStepNotWritable, jobID)
	}
	if err := terminalizeTaskLedgerTx(
		ctx, tx, jobID, generation, model.JobStatusFailed, &command.StepID,
		"job failed after its current-generation step failed",
	); err != nil {
		return err
	}
	if err := completeTelemetryRunForJob(ctx, tx, jobID, model.JobStatusFailed, map[string]any{"job_id": jobID, "error": command.Error}, map[string]any{"failed_step_id": command.StepID}); err != nil {
		return err
	}
	if err := recordTelemetryJobEvent(ctx, tx, jobID, "run_failed", map[string]any{"job_id": jobID, "step_id": command.StepID, "error": command.Error}); err != nil {
		return err
	}
	job, err = scanLockedJobTx(ctx, tx, jobID)
	if err != nil {
		return err
	}
	stepStatus := model.StepStatusFailed
	if err := insertLifecycleOperationTx(ctx, tx, descriptor, lifecycleOperationRecord{
		ID: descriptor.ID, JobID: jobID, ObservedGeneration: generation,
		ResultGeneration: job.CurrentGeneration, StepID: &command.StepID,
		Kind: descriptor.Kind, CommandSHA256: descriptor.SHA256,
		ResultJobStatus: job.Status, ResultStepStatus: &stepStatus, ResultJob: job,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func jobAcceptsStepTerminal(status string) bool {
	return status == model.JobStatusRunning || status == model.JobStatusWaiting
}
