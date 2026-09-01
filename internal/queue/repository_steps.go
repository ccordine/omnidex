package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CompleteStep(ctx context.Context, command CompleteStepCommand) error {
	command, err := normalizeCompleteStepCommand(command)
	if err != nil {
		return err
	}
	descriptor, err := describeLifecycleOperation(
		command.OperationID, LifecycleCompleteStep, command,
	)
	if err != nil {
		return err
	}
	return r.completeStep(ctx, command, &descriptor, nil)
}

func (r *Repository) completeStep(
	ctx context.Context,
	command CompleteStepCommand,
	descriptor *lifecycleOperationDescriptor,
	objectiveEvidencePayloads [][]byte,
) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if descriptor != nil {
		if err := lockLifecycleOperationIdentityTx(ctx, tx, command.OperationID); err != nil {
			return err
		}
	}
	lockedAttempt, err := lockStepAttemptAuthorityTx(ctx, tx, command.Authority)
	if err != nil {
		return err
	}
	jobID := command.Authority.JobID
	job, err := scanLockedJobTx(ctx, tx, jobID)
	if err != nil {
		return err
	}
	if descriptor != nil {
		if existing, found, err := loadLifecycleOperationTx(ctx, tx, *descriptor, jobID); err != nil {
			return err
		} else if found {
			if err := requireCompleteStepReplayTx(ctx, tx, existing, command); err != nil {
				return err
			}
			researchHandled, err := requireRoleplayResearchCompletionReplayTx(
				ctx, tx, job, existing, command,
			)
			if err != nil {
				return err
			}
			if !researchHandled {
				if err := requireRoleplayCompletionReplayTx(ctx, tx, job, existing, command); err != nil {
					return err
				}
			}
			if objectiveEvidencePayloads != nil {
				return requireObjectiveCompletionEvidenceReplayTx(
					ctx, tx, command.OperationID, objectiveEvidencePayloads,
				)
			}
			return nil
		}
	}
	if err := requireRoleplayCompletionJobAuthority(job, command); err != nil {
		return err
	}
	if err := requireLockedStepAttemptActiveTx(ctx, tx, command.Authority, lockedAttempt); err != nil {
		return err
	}
	if err := requireNewRoleplayCompletionPayload(job, command); err != nil {
		return err
	}
	if lockedAttempt.StepStatus != model.StepStatusRunning || !jobAcceptsStepTerminal(lockedAttempt.JobStatus) {
		return staleStepAttemptError(command.Authority, fmt.Sprintf(
			"completion writer job status %q step status %q",
			lockedAttempt.JobStatus, lockedAttempt.StepStatus,
		), nil)
	}
	if objectiveEvidencePayloads != nil {
		if err := insertObjectiveCompletionEvidenceTx(ctx, tx, command, objectiveEvidencePayloads); err != nil {
			return err
		}
	}
	generation := command.Authority.Generation
	if err := terminalizeStepAttemptTx(ctx, tx, command.Authority, model.StepAttemptCompleted); err != nil {
		return err
	}

	stepUpdate, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2, output = $3, finished_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND job_id=$4 AND generation=$5 AND current_attempt=$6
		  AND worker_id=$7 AND status=$8
	`, command.StepID, model.StepStatusCompleted, command.Output, jobID, generation,
		command.Authority.Attempt, command.Authority.WorkerID, model.StepStatusRunning)
	if err != nil {
		return err
	}
	if stepUpdate.RowsAffected() == 0 {
		return staleStepAttemptError(command.Authority, "completion target lost current authority", nil)
	}
	if command.ContextKey == "objective_result" {
		if err := cancelTerminalObjectiveCodingTailTx(ctx, tx, job, command); err != nil {
			return err
		}
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
		if descriptor != nil {
			if err := materializeChannelCompletionTx(ctx, tx, job, command); err != nil {
				return err
			}
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
			return staleStepAttemptError(command.Authority, "job lost terminal-completion authority", nil)
		}
		if err := terminalizeScrumJobCardTx(
			ctx, tx, job, model.JobStatusCompleted,
		); err != nil {
			return err
		}
	} else {
		if hasRoleplayCompletionPayload(command) {
			return fmt.Errorf("roleplay facts require the terminal current-generation step")
		}
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
	if descriptor != nil {
		stepStatus := model.StepStatusCompleted
		if err := insertLifecycleOperationTx(ctx, tx, *descriptor, lifecycleOperationRecord{
			ID: descriptor.ID, JobID: jobID, ObservedGeneration: generation,
			ResultGeneration: job.CurrentGeneration, StepID: &command.StepID,
			Kind: descriptor.Kind, CommandSHA256: descriptor.SHA256,
			ResultJobStatus: job.Status, ResultStepStatus: &stepStatus, ResultJob: job,
		}); err != nil {
			return err
		}
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
	lockedAttempt, err := lockStepAttemptAuthorityTx(ctx, tx, command.Authority)
	if err != nil {
		return err
	}
	jobID := command.Authority.JobID
	job, err := scanLockedJobTx(ctx, tx, jobID)
	if err != nil {
		return err
	}
	if existing, found, err := loadLifecycleOperationTx(ctx, tx, descriptor, jobID); err != nil {
		return err
	} else if found {
		return requireFailStepReplayTx(ctx, tx, existing, command)
	}
	if err := requireLockedStepAttemptActiveTx(ctx, tx, command.Authority, lockedAttempt); err != nil {
		return err
	}
	if lockedAttempt.StepStatus != model.StepStatusRunning || !jobAcceptsStepTerminal(lockedAttempt.JobStatus) {
		return staleStepAttemptError(command.Authority, fmt.Sprintf(
			"failure writer job status %q step status %q",
			lockedAttempt.JobStatus, lockedAttempt.StepStatus,
		), nil)
	}
	generation := command.Authority.Generation
	if err := terminalizeStepAttemptTx(ctx, tx, command.Authority, model.StepAttemptFailed); err != nil {
		return err
	}

	stepUpdate, err := tx.Exec(ctx, `
		UPDATE job_steps
		SET status = $2, error = $3, finished_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND job_id=$4 AND generation=$5 AND current_attempt=$6
		  AND worker_id=$7 AND status=$8
	`, command.StepID, model.StepStatusFailed, command.Error, jobID, generation,
		command.Authority.Attempt, command.Authority.WorkerID, model.StepStatusRunning)
	if err != nil {
		return err
	}
	if stepUpdate.RowsAffected() == 0 {
		return staleStepAttemptError(command.Authority, "failure target lost current authority", nil)
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
		return staleStepAttemptError(command.Authority, "job lost terminal-failure authority", nil)
	}
	if err := terminalizeScrumJobCardTx(ctx, tx, job, model.JobStatusFailed); err != nil {
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
