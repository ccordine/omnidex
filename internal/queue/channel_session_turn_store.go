package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func channelSessionTurnOperationExistsTx(
	ctx context.Context,
	tx pgx.Tx,
	operationID LifecycleOperationID,
) (bool, error) {
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM channel_session_turn_operations WHERE operation_id=$1
		)
	`, operationID).Scan(&exists); err != nil {
		return false, fmt.Errorf(
			"check channel session operation %q: %w",
			operationID,
			err,
		)
	}
	return exists, nil
}

func loadChannelSessionTurnOperationTx(
	ctx context.Context,
	tx pgx.Tx,
	command ChannelSessionTurnCommand,
	authority lockedChannelTurnAuthority,
) (ChannelSessionTurnResult, bool, error) {
	var disposition ChannelSessionTurnDisposition
	var jobID, resultGeneration int64
	var messageID, stepID *int64
	var resultJobJSON []byte
	err := tx.QueryRow(ctx, `
		SELECT disposition, job_id, result_generation, user_message_id,
		       result_step_id, result_job
		FROM channel_session_turn_operations
		WHERE operation_id=$1
	`, command.OperationID).Scan(
		&disposition,
		&jobID,
		&resultGeneration,
		&messageID,
		&stepID,
		&resultJobJSON,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChannelSessionTurnResult{}, false, nil
	}
	if err != nil {
		return ChannelSessionTurnResult{}, false, fmt.Errorf(
			"read channel session operation %q: %w",
			command.OperationID,
			err,
		)
	}
	if err := validateChannelSessionTurnDisposition(disposition); err != nil {
		return ChannelSessionTurnResult{}, false, err
	}
	var job model.Job
	if err := json.Unmarshal(resultJobJSON, &job); err != nil {
		return ChannelSessionTurnResult{}, false, fmt.Errorf(
			"decode channel session operation %q result job: %w",
			command.OperationID,
			err,
		)
	}
	if job.ID != jobID || job.CurrentGeneration != resultGeneration {
		return ChannelSessionTurnResult{}, false, fmt.Errorf(
			"channel session operation %q contains inconsistent result authority",
			command.OperationID,
		)
	}
	if err := validateChannelSessionJobAuthority(
		command.ChannelID,
		authority,
		command.WorkspaceIdentity,
		job,
	); err != nil {
		return ChannelSessionTurnResult{}, false, err
	}
	if disposition == ChannelSessionTurnFeedback && stepID == nil ||
		disposition != ChannelSessionTurnFeedback && stepID != nil {
		return ChannelSessionTurnResult{}, false, fmt.Errorf(
			"channel session operation %q has contradictory step authority",
			command.OperationID,
		)
	}
	var message *model.ChannelMessage
	if disposition == ChannelSessionTurnEnqueued {
		if messageID == nil {
			return ChannelSessionTurnResult{}, false, fmt.Errorf(
				"channel session enqueue %q has no user message",
				command.OperationID,
			)
		}
		persisted, err := channelSessionUserMessageTx(
			ctx,
			tx,
			command.ChannelID,
			*messageID,
			command.Text,
		)
		if err != nil {
			return ChannelSessionTurnResult{}, false, err
		}
		message = &persisted
	} else if messageID != nil {
		return ChannelSessionTurnResult{}, false, fmt.Errorf(
			"channel session control %q unexpectedly owns a user message",
			command.OperationID,
		)
	}
	if err := requireChannelSessionTurnEffectTx(
		ctx,
		tx,
		command,
		disposition,
		job,
		messageID,
		stepID,
	); err != nil {
		return ChannelSessionTurnResult{}, false, err
	}
	return channelSessionTurnResult(command, disposition, job, message, false), true, nil
}

func insertChannelSessionTurnOperationTx(
	ctx context.Context,
	tx pgx.Tx,
	descriptor lifecycleOperationDescriptor,
	result ChannelSessionTurnResult,
	stepID *int64,
) error {
	if err := validateChannelSessionTurnDisposition(result.Disposition); err != nil {
		return err
	}
	var messageID *int64
	if result.UserMessage != nil {
		id := result.UserMessage.ID
		messageID = &id
	}
	resultJobJSON, err := json.Marshal(result.Job)
	if err != nil {
		return fmt.Errorf("encode channel session operation result: %w", err)
	}
	insert, err := tx.Exec(ctx, `
		INSERT INTO channel_session_turn_operations (
			operation_id, kind, command_sha256, channel_id, job_id,
			result_generation, disposition, user_message_id, result_step_id,
			result_job
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb)
	`, descriptor.ID, descriptor.Kind, descriptor.SHA256, result.ChannelID, result.Job.ID,
		result.Job.CurrentGeneration, result.Disposition, messageID, stepID,
		string(resultJobJSON))
	if err != nil {
		return fmt.Errorf("record channel session operation %q: %w", descriptor.ID, err)
	}
	if insert.RowsAffected() != 1 {
		return fmt.Errorf("channel session operation %q was not recorded", descriptor.ID)
	}
	return nil
}

func channelSessionUserMessageTx(
	ctx context.Context,
	tx pgx.Tx,
	channelID model.ChannelID,
	messageID int64,
	exactText string,
) (model.ChannelMessage, error) {
	var message model.ChannelMessage
	if err := tx.QueryRow(ctx, `
		SELECT id, channel_id, role, content, created_at
		FROM ai_channel_messages
		WHERE id=$1 AND channel_id=$2
	`, messageID, channelID).Scan(
		&message.ID,
		&message.ChannelID,
		&message.Role,
		&message.Content,
		&message.CreatedAt,
	); err != nil {
		return model.ChannelMessage{}, fmt.Errorf(
			"read channel %q session message %d: %w",
			channelID,
			messageID,
			err,
		)
	}
	if message.Role != model.ChannelMessageRoleUser || message.Content != exactText {
		return model.ChannelMessage{}, fmt.Errorf(
			"channel %q session message %d differs from exact turn authority",
			channelID,
			messageID,
		)
	}
	return message, nil
}
