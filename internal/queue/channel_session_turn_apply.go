package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) applyChannelSessionTurnTx(
	ctx context.Context,
	tx pgx.Tx,
	command ChannelSessionTurnCommand,
	authority lockedChannelTurnAuthority,
	job model.Job,
	found bool,
) (ChannelSessionTurnResult, *int64, error) {
	if !found {
		message, job, err := r.persistChannelTurnTx(
			ctx,
			tx,
			command.ChannelID,
			authority,
			command.Text,
			"",
			nil,
			command.WorkspaceIdentity,
		)
		if err != nil {
			return ChannelSessionTurnResult{}, nil, err
		}
		return channelSessionTurnResult(
			command,
			ChannelSessionTurnEnqueued,
			job,
			&message,
			true,
		), nil, nil
	}
	if err := validateChannelSessionJobAuthority(
		command.ChannelID,
		authority,
		command.WorkspaceIdentity,
		job,
	); err != nil {
		return ChannelSessionTurnResult{}, nil, err
	}
	authoritativeJobID := job.ID

	disposition := ChannelSessionTurnReplanned
	var resultStepID *int64
	var err error
	switch job.Status {
	case model.JobStatusPending, model.JobStatusRunning:
		job, err = applyChannelSessionReplanTx(ctx, tx, command, job)
	case model.JobStatusWaiting:
		var generationPurpose string
		if err = tx.QueryRow(ctx, `
			SELECT purpose
			FROM job_generations
			WHERE job_id=$1 AND generation=$2
			FOR SHARE
		`, job.ID, job.CurrentGeneration).Scan(&generationPurpose); err != nil {
			return ChannelSessionTurnResult{}, nil, fmt.Errorf(
				"read channel %q waiting job %d generation purpose: %w",
				command.ChannelID,
				job.ID,
				err,
			)
		}
		switch generationPurpose {
		case jobGenerationPurposeInterrupt:
			job, err = applyChannelSessionReplanTx(ctx, tx, command, job)
		case jobGenerationPurposeInitial, jobGenerationPurposeReplan:
			disposition = ChannelSessionTurnFeedback
			var stepID int64
			job, stepID, err = applyChannelSessionFeedbackTx(ctx, tx, command, job)
			if err == nil {
				resultStepID = &stepID
			}
		default:
			return ChannelSessionTurnResult{}, nil, fmt.Errorf(
				"%w: job %d generation %d has unregistered purpose %q",
				ErrInvalidJobGeneration,
				job.ID,
				job.CurrentGeneration,
				generationPurpose,
			)
		}
	default:
		return ChannelSessionTurnResult{}, nil, fmt.Errorf(
			"channel %q active job %d changed to unsupported status %q",
			command.ChannelID,
			job.ID,
			job.Status,
		)
	}
	if err != nil {
		return ChannelSessionTurnResult{}, nil, err
	}
	if job.ID != authoritativeJobID {
		return ChannelSessionTurnResult{}, nil, fmt.Errorf(
			"channel %q session control changed job identity from %d to %d",
			command.ChannelID,
			authoritativeJobID,
			job.ID,
		)
	}
	if _, err := tx.Exec(
		ctx,
		`UPDATE ai_channels SET updated_at=NOW() WHERE id=$1`,
		command.ChannelID,
	); err != nil {
		return ChannelSessionTurnResult{}, nil, err
	}
	return channelSessionTurnResult(command, disposition, job, nil, true), resultStepID, nil
}

func applyChannelSessionReplanTx(
	ctx context.Context,
	tx pgx.Tx,
	command ChannelSessionTurnCommand,
	job model.Job,
) (model.Job, error) {
	feedback, feedbackSHA, err := validateSessionReplanFeedback(command.Text)
	if err != nil {
		return model.Job{}, fmt.Errorf("%w: %v", ErrChannelSessionTurnInvalid, err)
	}
	replan := ReplanJobCommand{
		OperationID:       command.OperationID,
		JobID:             job.ID,
		Feedback:          feedback,
		WorkspaceRoot:     command.WorkspaceRoot,
		WorkspaceIdentity: command.WorkspaceIdentity,
	}
	result, _, err := applyReplanJobTx(ctx, tx, replan, feedbackSHA, job)
	return result, err
}

func applyChannelSessionFeedbackTx(
	ctx context.Context,
	tx pgx.Tx,
	command ChannelSessionTurnCommand,
	job model.Job,
) (model.Job, int64, error) {
	feedback, err := normalizeSubmitFeedbackCommand(SubmitJobFeedbackCommand{
		OperationID:       command.OperationID,
		JobID:             job.ID,
		Feedback:          command.Text,
		WorkspaceRoot:     command.WorkspaceRoot,
		WorkspaceIdentity: command.WorkspaceIdentity,
	})
	if err != nil {
		return model.Job{}, 0, fmt.Errorf("%w: %v", ErrChannelSessionTurnInvalid, err)
	}
	return applyJobFeedbackTx(ctx, tx, feedback, job)
}

func channelSessionTurnResult(
	command ChannelSessionTurnCommand,
	disposition ChannelSessionTurnDisposition,
	job model.Job,
	message *model.ChannelMessage,
	applied bool,
) ChannelSessionTurnResult {
	return ChannelSessionTurnResult{
		OperationID:       command.OperationID,
		Disposition:       disposition,
		ChannelID:         command.ChannelID,
		WorkspaceRoot:     command.WorkspaceRoot,
		WorkspaceIdentity: command.WorkspaceIdentity,
		Job:               job,
		UserMessage:       message,
		Applied:           applied,
	}
}
