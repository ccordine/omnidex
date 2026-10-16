package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
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
	if command.ExpectedInputID != nil {
		if !found || job.Status != model.JobStatusWaiting {
			return ChannelSessionTurnResult{}, nil, fmt.Errorf("%w: native request is no longer waiting", ErrChannelSessionTurnInvalid)
		}
		var matches bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM evidence request
		  WHERE id=$1 AND job_id=$2 AND kind='coding_agent' AND payload_json->>'generation'=$3::bigint::text
		    AND payload_json->>'phase'='execution_native_input'
		    AND NOT EXISTS(SELECT 1 FROM evidence later WHERE later.job_id=request.job_id AND later.step_id=request.step_id
		      AND later.id>request.id AND later.kind='coding_agent'
		      AND later.payload_json->>'phase' IN ('execution_reply','execution_native_input','execution_response','review_response','task_incomplete')))`,
			*command.ExpectedInputID, job.ID, job.CurrentGeneration).Scan(&matches); err != nil {
			return ChannelSessionTurnResult{}, nil, err
		}
		if !matches {
			return ChannelSessionTurnResult{}, nil, fmt.Errorf("%w: native request identity changed", ErrChannelSessionTurnInvalid)
		}
	}
	if !found {
		configuration, err := r.codingConfigurationForMode(ctx, tx, command.CodingMode)
		if err != nil {
			return ChannelSessionTurnResult{}, nil, err
		}
		if _, err := bindChannelCodingExecution(&authority, configuration); err != nil {
			return ChannelSessionTurnResult{}, nil, err
		}
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
			model.ChannelSessionTurnEnqueued,
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
	observedGeneration := job.CurrentGeneration

	disposition := model.ChannelSessionTurnReplanned
	var resultStepID *int64
	var err error
	switch job.Status {
	case model.JobStatusPending, model.JobStatusRunning:
		return ChannelSessionTurnResult{}, nil, fmt.Errorf("%w: active message requires the observed job and generation", ErrChannelSessionTurnInvalid)
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
			configuration, configurationErr := codingagent.ConfigurationFromMetadata(job.Metadata)
			if configurationErr != nil {
				return ChannelSessionTurnResult{}, nil, configurationErr
			}
			if configuration.Mode == codingagent.Agents {
				var planning, interruptedPlanning bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM job_steps WHERE job_id=$1 AND generation=$2
				  AND action='v3_coding_plan' AND status='waiting_input' AND superseded_at_generation IS NULL),
				  EXISTS(SELECT 1 FROM evidence event JOIN job_steps step ON step.id=event.step_id
				    WHERE event.job_id=$1 AND step.generation=$2 AND step.action='v3_coding_plan'
				      AND step.status='waiting_input' AND event.kind='coding_agent'
				      AND event.payload_json->>'phase'='planning_incomplete')`,
					job.ID, job.CurrentGeneration).Scan(&planning, &interruptedPlanning); err != nil {
					return ChannelSessionTurnResult{}, nil, err
				}
				if planning && !interruptedPlanning && command.ExpectedInputID == nil {
					job, err = applyChannelSessionReplanTx(ctx, tx, command, job)
				} else {
					disposition = model.ChannelSessionTurnFeedback
					var stepID int64
					job, stepID, err = applyChannelSessionFeedbackTx(ctx, tx, command, job)
					if err == nil {
						if job.CurrentGeneration != observedGeneration {
							disposition = model.ChannelSessionTurnReplanned
						} else {
							resultStepID = &stepID
						}
					}
				}
				break
			}
			disposition = model.ChannelSessionTurnFeedback
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
	configuration, err := codingagent.ConfigurationFromMetadata(job.Metadata)
	if err != nil {
		return model.Job{}, err
	}
	feedback, err := validateSessionReplanFeedback(command.Text)
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
	if configuration.Mode == codingagent.Agents {
		if err := tx.QueryRow(ctx, `
			SELECT ledger.current_revision FROM coding_ledgers ledger
			WHERE ledger.job_id=$1 AND (ledger.state='proposed' OR
			  (ledger.state='revising' AND EXISTS (SELECT 1 FROM job_generations generation
			    WHERE generation.job_id=$1 AND generation.generation=$2 AND generation.purpose=$3)))
			  AND EXISTS (SELECT 1 FROM job_steps step WHERE step.job_id=$1
			      AND step.generation=$2 AND step.action='v3_coding_plan'
			      AND step.status='waiting_input' AND step.superseded_at_generation IS NULL)
			FOR UPDATE OF ledger
		`, job.ID, job.CurrentGeneration, jobGenerationPurposeInterrupt).Scan(&replan.ObservedLedgerRevision); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return model.Job{}, fmt.Errorf("%w: continuation requires a proposed ledger or an interrupted ledger open for revision, with a waiting planning step", ErrChannelSessionTurnInvalid)
			}
			return model.Job{}, err
		}
	}
	// The session turn owns replay of its result; the child replan retains the
	// exact revision-bound authority consumed by the planning continuation.
	replan.OperationID, err = model.NewLifecycleOperationID()
	if err != nil {
		return model.Job{}, err
	}
	descriptor, err := describeLifecycleOperation(replan.OperationID, LifecycleReplanJob, replan)
	if err != nil {
		return model.Job{}, err
	}
	result, err := replanJobTx(ctx, tx, replan, descriptor)
	return result.Job, err
}

func applyChannelSessionFeedbackTx(
	ctx context.Context,
	tx pgx.Tx,
	command ChannelSessionTurnCommand,
	job model.Job,
) (model.Job, int64, error) {
	feedback, err := normalizeSubmitFeedbackCommand(SubmitJobFeedbackCommand{
		OperationID:        command.OperationID,
		JobID:              job.ID,
		ExpectedGeneration: &job.CurrentGeneration,
		ExpectedInputID:    command.ExpectedInputID,
		Feedback:           command.Text,
		WorkspaceRoot:      command.WorkspaceRoot,
		WorkspaceIdentity:  command.WorkspaceIdentity,
	})
	if err != nil {
		return model.Job{}, 0, fmt.Errorf("%w: %v", ErrChannelSessionTurnInvalid, err)
	}
	return applyJobFeedbackTx(ctx, tx, feedback, job)
}

func channelSessionTurnResult(
	command ChannelSessionTurnCommand,
	disposition model.ChannelSessionTurnDisposition,
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
