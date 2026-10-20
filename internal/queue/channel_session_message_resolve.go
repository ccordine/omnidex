package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ResolveChannelSessionMessage(ctx context.Context, command ResolveChannelSessionMessageCommand) (ChannelSessionMessageResult, error) {
	if command.Kind != assemblyline.ObjectiveKindAnswer && command.Kind != assemblyline.ObjectiveKindStory && command.Kind != assemblyline.ObjectiveKindWorkspaceMutation {
		return ChannelSessionMessageResult{}, fmt.Errorf("unsupported active message meaning %q", command.Kind)
	}
	return r.resolveChannelSessionMessage(ctx, command, string(command.Kind))
}

func (r *Repository) FailChannelSessionMessage(ctx context.Context, command FailChannelSessionMessageCommand) (ChannelSessionMessageResult, error) {
	if err := validateLifecycleText("message failure", command.Error, 2048, false); err != nil {
		return ChannelSessionMessageResult{}, err
	}
	return r.resolveChannelSessionMessage(ctx, ResolveChannelSessionMessageCommand{Authority: command.Authority,
		OperationID: command.OperationID, Response: "Message could not be answered: " + command.Error}, "failed")
}

// Planning already has a durable ordinary response and a persisted ledger. It
// needs neither a second assistant reply nor a generic intent classification.
func (r *Repository) ResolvePlanningSessionMessage(ctx context.Context, authority model.StepAttemptAuthority, operationID model.LifecycleOperationID) (ChannelSessionMessageResult, error) {
	return r.resolveChannelSessionMessage(ctx, ResolveChannelSessionMessageCommand{
		Authority: authority, OperationID: operationID,
	}, string(model.ChannelSessionMessagePlanning))
}

func (r *Repository) resolveChannelSessionMessage(ctx context.Context, command ResolveChannelSessionMessageCommand, meaning string) (ChannelSessionMessageResult, error) {
	if err := validateStepAttemptAuthority(command.Authority); err != nil {
		return ChannelSessionMessageResult{}, err
	}
	if command.Kind == assemblyline.ObjectiveKindWorkspaceMutation && command.Response != "" {
		return ChannelSessionMessageResult{}, fmt.Errorf("redirect message cannot contain an answer")
	}
	if command.Kind != assemblyline.ObjectiveKindWorkspaceMutation && meaning != string(model.ChannelSessionMessagePlanning) {
		if err := model.ValidateChannelMessage(model.ChannelMessageRoleAssistant, command.Response); err != nil {
			return ChannelSessionMessageResult{}, err
		}
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ChannelSessionMessageResult{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockLifecycleOperationIdentityTx(ctx, tx, command.OperationID); err != nil {
		return ChannelSessionMessageResult{}, err
	}
	message, found, err := readChannelSessionMessageTx(ctx, tx, command.OperationID)
	if err != nil {
		return ChannelSessionMessageResult{}, err
	}
	if !found || message.AdmittedJob.ID != command.Authority.JobID || (message.AdmittedJob.CurrentGeneration != command.Authority.Generation && !(meaning == string(model.ChannelSessionMessagePlanning) && message.PlanningBinding != nil && message.AdmittedJob.CurrentGeneration < command.Authority.Generation)) {
		return ChannelSessionMessageResult{}, fmt.Errorf("active message differs from consumer authority")
	}
	if err := lockLifecycleOperationIdentityTx(ctx, tx, message.RedirectOperationID); err != nil {
		return ChannelSessionMessageResult{}, err
	}
	resolution, resolved, err := readChannelSessionMessageResolutionTx(ctx, tx, command.OperationID)
	if err != nil {
		return ChannelSessionMessageResult{}, err
	}
	if resolved {
		result, err := replayChannelSessionMessageResolutionTx(ctx, tx, message, resolution, command, meaning)
		if err != nil {
			return result, err
		}
		return result, tx.Commit(ctx)
	}
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, command.Authority)
	if err != nil {
		return ChannelSessionMessageResult{}, err
	}
	nativeWaiting := false
	if jobStatus == model.JobStatusWaiting && stepStatus == model.StepStatusWaiting {
		nativeWaiting, err = codingNativeInputWaitingTx(ctx, tx, command.Authority)
		if err != nil {
			return ChannelSessionMessageResult{}, err
		}
	}
	if !nativeWaiting && (jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning) {
		return ChannelSessionMessageResult{}, staleStepAttemptError(command.Authority, "message resolution requires running authority", nil)
	}
	job, err := scanLockedJobTx(ctx, tx, command.Authority.JobID)
	if err != nil {
		return ChannelSessionMessageResult{}, err
	}
	channel, err := lockChannelTurnAuthorityTx(ctx, tx, message.Command.ChannelID)
	if err != nil {
		return ChannelSessionMessageResult{}, err
	}
	if err := validateAssistantSessionAuthority(message.Command, channel); err != nil {
		return ChannelSessionMessageResult{}, err
	}
	if err := validateChannelSessionJobAuthority(message.Command.ChannelID, channel, message.Command.WorkspaceIdentity, job); err != nil {
		return ChannelSessionMessageResult{}, err
	}
	resolution = channelSessionMessageResolution{OperationID: command.OperationID, Kind: meaning, Job: job, Authority: &command.Authority}
	result := ChannelSessionMessageResult{Job: job, Applied: true}
	var amendmentQueued bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM evidence WHERE kind='coding_planning_amendment' AND source_ref=$1)`, command.OperationID).Scan(&amendmentQueued); err != nil {
		return ChannelSessionMessageResult{}, err
	}
	if amendmentQueued && command.Kind != assemblyline.ObjectiveKindWorkspaceMutation && meaning != string(model.ChannelSessionMessagePlanning) {
		return ChannelSessionMessageResult{}, fmt.Errorf("%w: queued amendment meaning changed", ErrLifecycleOperationConflict)
	}
	if command.Kind == assemblyline.ObjectiveKindWorkspaceMutation && message.PlanningBinding != nil {
		applied, err := queueCodingPlanningAmendmentTx(ctx, tx, message, command.Authority)
		if err != nil {
			return ChannelSessionMessageResult{}, err
		}
		return ChannelSessionMessageResult{Job: job, Applied: applied}, tx.Commit(ctx)
	}
	if meaning == string(model.ChannelSessionMessagePlanning) && message.PlanningBinding != nil {
		if err := requireCodingPlanningAmendmentTransitionTx(ctx, tx, message, command.Authority); err != nil {
			return ChannelSessionMessageResult{}, err
		}
	}
	if command.Kind == assemblyline.ObjectiveKindWorkspaceMutation {
		child := ReplanJobCommand{OperationID: message.RedirectOperationID, JobID: job.ID, Feedback: message.Command.Text, WorkspaceRoot: message.Command.WorkspaceRoot, WorkspaceIdentity: message.Command.WorkspaceIdentity}
		descriptor, err := describeLifecycleOperation(child.OperationID, LifecycleReplanJob, child)
		if err != nil {
			return ChannelSessionMessageResult{}, err
		}
		replanned, err := replanJobTx(ctx, tx, child, descriptor)
		if err != nil {
			return ChannelSessionMessageResult{}, err
		}
		resolution.Job, resolution.ControlOperationID = replanned.Job, child.OperationID
		result.Job, result.Replanned = replanned.Job, true
	} else if meaning != string(model.ChannelSessionMessagePlanning) {
		var reply model.ChannelMessage
		if err := tx.QueryRow(ctx, `INSERT INTO ai_channel_messages(channel_id,role,content) VALUES($1,'assistant',$2) RETURNING id,channel_id,role,content,created_at`, message.Command.ChannelID, command.Response).Scan(&reply.ID, &reply.ChannelID, &reply.Role, &reply.Content, &reply.CreatedAt); err != nil {
			return ChannelSessionMessageResult{}, err
		}
		resolution.ResponseMessageID, result.ResponseMessage = &reply.ID, &reply
	}
	if err := insertChannelSessionMessageResolutionTx(ctx, tx, message, command.Authority.StepID, resolution); err != nil {
		return ChannelSessionMessageResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ChannelSessionMessageResult{}, err
	}
	return result, nil
}

func insertChannelSessionMessageResolutionTx(ctx context.Context, tx pgx.Tx, intake ChannelSessionMessage, stepID int64, resolution channelSessionMessageResolution) error {
	payload, err := json.Marshal(resolution)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json) VALUES($1,$2,'channel_session_message','lifecycle_operation',$3,$4::jsonb)`, intake.AdmittedJob.ID, stepID, intake.OperationID, string(payload)); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE ai_channels SET updated_at=clock_timestamp() WHERE id=$1`, intake.Command.ChannelID)
	return err
}

func replayChannelSessionMessageResolutionTx(ctx context.Context, tx pgx.Tx, intake ChannelSessionMessage, resolution channelSessionMessageResolution, command ResolveChannelSessionMessageCommand, meaning string) (ChannelSessionMessageResult, error) {
	if resolution.Kind == "retired" {
		return ChannelSessionMessageResult{}, staleStepAttemptError(command.Authority, "queued message was retired by a control", nil)
	}
	if resolution.OperationID != command.OperationID || resolution.Kind != meaning || resolution.Authority == nil || *resolution.Authority != command.Authority {
		return ChannelSessionMessageResult{}, fmt.Errorf("%w: message resolution changed", ErrLifecycleOperationConflict)
	}
	result := ChannelSessionMessageResult{Job: resolution.Job, Replanned: resolution.Kind == string(assemblyline.ObjectiveKindWorkspaceMutation)}
	if resolution.ResponseMessageID != nil {
		var reply model.ChannelMessage
		if err := tx.QueryRow(ctx, `SELECT id,channel_id,role,content,created_at FROM ai_channel_messages WHERE id=$1 AND channel_id=$2 AND role='assistant'`, *resolution.ResponseMessageID, intake.Command.ChannelID).Scan(&reply.ID, &reply.ChannelID, &reply.Role, &reply.Content, &reply.CreatedAt); err != nil {
			return result, err
		}
		if reply.Content != command.Response {
			return result, fmt.Errorf("%w: message answer changed", ErrLifecycleOperationConflict)
		}
		result.ResponseMessage = &reply
	}
	return result, nil
}
