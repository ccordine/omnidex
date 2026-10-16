package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) submitChannelSessionMessage(ctx context.Context, command ChannelSessionTurnCommand) (ChannelSessionTurnResult, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ChannelSessionTurnResult{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockLifecycleOperationIdentityTx(ctx, tx, command.OperationID); err != nil {
		return ChannelSessionTurnResult{}, err
	}
	intake, found, err := readChannelSessionMessageTx(ctx, tx, command.OperationID)
	if err != nil {
		return ChannelSessionTurnResult{}, err
	}
	if found {
		if !reflect.DeepEqual(intake.Command, command) {
			return ChannelSessionTurnResult{}, fmt.Errorf("%w: active message command changed", ErrLifecycleOperationConflict)
		}
		authority, err := lockChannelTurnAuthorityTx(ctx, tx, command.ChannelID)
		if err != nil {
			return ChannelSessionTurnResult{}, err
		}
		if err := validateAssistantSessionAuthority(command, authority); err != nil {
			return ChannelSessionTurnResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return ChannelSessionTurnResult{}, err
		}
		return channelSessionTurnResult(command, model.ChannelSessionTurnQueued, intake.AdmittedJob, nil, false), nil
	}
	job, err := lockedJobTx(ctx, tx, *command.ExpectedJobID)
	if err != nil {
		return ChannelSessionTurnResult{}, err
	}
	if job.CurrentGeneration != *command.ExpectedGeneration ||
		(job.Status != model.JobStatusRunning && job.Status != model.JobStatusPending) {
		return ChannelSessionTurnResult{}, fmt.Errorf("%w: observed message job is no longer active at generation %d", ErrStaleJobGeneration, *command.ExpectedGeneration)
	}
	configuration, err := codingagent.ConfigurationFromMetadata(job.Metadata)
	if err != nil {
		return ChannelSessionTurnResult{}, err
	}
	if configuration.Mode == codingagent.Agents {
		var planning bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM job_steps
			WHERE job_id=$1 AND generation=$2 AND action='v3_coding_plan'
			AND status IN ('pending','running'))`, job.ID, job.CurrentGeneration).Scan(&planning); err != nil {
			return ChannelSessionTurnResult{}, err
		}
		if !planning {
			return ChannelSessionTurnResult{}, fmt.Errorf("%w: execution replies require the active input request", ErrChannelSessionTurnInvalid)
		}
	}
	authority, err := lockChannelTurnAuthorityTx(ctx, tx, command.ChannelID)
	if err != nil {
		return ChannelSessionTurnResult{}, err
	}
	if err := validateAssistantSessionAuthority(command, authority); err != nil {
		return ChannelSessionTurnResult{}, err
	}
	if err := validateChannelSessionJobAuthority(command.ChannelID, authority, command.WorkspaceIdentity, job); err != nil {
		return ChannelSessionTurnResult{}, err
	}
	current, exists, err := activeChannelJobTx(ctx, tx, command.ChannelID)
	if err != nil {
		return ChannelSessionTurnResult{}, err
	}
	if !exists || current.ID != job.ID {
		return ChannelSessionTurnResult{}, fmt.Errorf("%w: active message target changed", ErrStaleJobGeneration)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM lifecycle_operation_registry WHERE kind='channel_session_message' AND message_job_id=$1`, job.ID).Scan(&count); err != nil {
		return ChannelSessionTurnResult{}, err
	}
	if count >= model.MaxChannelSessionTurns {
		return ChannelSessionTurnResult{}, fmt.Errorf("%w: active message history is full", ErrObjectiveSessionContextCapacity)
	}
	redirectID, err := model.NewLifecycleOperationID()
	if err != nil {
		return ChannelSessionTurnResult{}, err
	}
	intake = ChannelSessionMessage{OperationID: command.OperationID, Command: command, AdmittedJob: job, RedirectOperationID: redirectID}
	payload, err := json.Marshal(intake)
	if err != nil {
		return ChannelSessionTurnResult{}, err
	}
	if _, err := reserveLifecycleOperationIdentityTx(ctx, tx, command.OperationID, LifecycleChannelSessionMessage, payload); err != nil {
		return ChannelSessionTurnResult{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE ai_channels SET updated_at=clock_timestamp() WHERE id=$1`, command.ChannelID); err != nil {
		return ChannelSessionTurnResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ChannelSessionTurnResult{}, err
	}
	return channelSessionTurnResult(command, model.ChannelSessionTurnQueued, job, nil, true), nil
}

func readChannelSessionMessageTx(ctx context.Context, tx pgx.Tx, operationID model.LifecycleOperationID) (ChannelSessionMessage, bool, error) {
	var kind LifecycleOperationKind
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT kind,command_payload FROM lifecycle_operation_registry WHERE operation_id=$1`, operationID).Scan(&kind, &raw)
	if err == pgx.ErrNoRows {
		return ChannelSessionMessage{}, false, nil
	}
	if err != nil {
		return ChannelSessionMessage{}, false, err
	}
	if kind != LifecycleChannelSessionMessage {
		return ChannelSessionMessage{}, false, fmt.Errorf("%w: active message identity belongs to %s", ErrLifecycleOperationConflict, kind)
	}
	var message ChannelSessionMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		return ChannelSessionMessage{}, false, err
	}
	if message.OperationID != operationID || message.Command.OperationID != operationID ||
		message.Command.ExpectedJobID == nil || message.Command.ExpectedGeneration == nil ||
		message.AdmittedJob.ID != *message.Command.ExpectedJobID || message.AdmittedJob.CurrentGeneration != *message.Command.ExpectedGeneration {
		return ChannelSessionMessage{}, false, fmt.Errorf("active message intake lacks exact admitted authority")
	}
	if _, err := model.ParseLifecycleOperationID(string(message.RedirectOperationID)); err != nil {
		return ChannelSessionMessage{}, false, err
	}
	return message, true, nil
}
