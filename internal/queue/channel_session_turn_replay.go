package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func requireChannelSessionTurnEffectTx(
	ctx context.Context,
	tx pgx.Tx,
	command ChannelSessionTurnCommand,
	disposition ChannelSessionTurnDisposition,
	job model.Job,
	messageID *int64,
	stepID *int64,
) error {
	var jobExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM jobs
			WHERE id=$1 AND pipeline='chat'
			  AND metadata->>'channel_id'=$2
			  AND metadata->>'client_cwd'=$3
			  AND metadata->>'client_workspace_identity'=$4
		)
	`, job.ID, command.ChannelID, command.WorkspaceRoot, command.WorkspaceIdentity).Scan(&jobExists); err != nil {
		return err
	}
	if !jobExists {
		return channelSessionReplayError(command.OperationID, "same-channel job")
	}
	switch disposition {
	case ChannelSessionTurnEnqueued:
		if messageID == nil || stepID != nil || job.CurrentGeneration != 1 ||
			job.Instruction != command.Text {
			return channelSessionReplayError(command.OperationID, "initial turn shape")
		}
		var valid bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM job_generations AS generation
				JOIN jobs AS persisted_job ON persisted_job.id=generation.job_id
				JOIN ai_channel_messages AS message
				  ON message.id=$3 AND message.channel_id=$2
				WHERE generation.job_id=$1 AND generation.generation=1
				  AND generation.purpose='initial'
				  AND message.role='user' AND message.content=$4
				  AND persisted_job.metadata->>'channel_user_message_id'=message.id::TEXT
			)
		`, job.ID, command.ChannelID, *messageID, command.Text).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return channelSessionReplayError(command.OperationID, "initial message and generation")
		}
	case ChannelSessionTurnReplanned:
		if messageID != nil || stepID != nil || job.CurrentGeneration < 2 {
			return channelSessionReplayError(command.OperationID, "replan shape")
		}
		var valid bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM job_generations
				WHERE job_id=$1 AND generation=$2
				  AND purpose='replan' AND feedback=$3
			)
		`, job.ID, job.CurrentGeneration, command.Text).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return channelSessionReplayError(command.OperationID, "replan generation")
		}
	case ChannelSessionTurnFeedback:
		if messageID != nil || stepID == nil {
			return channelSessionReplayError(command.OperationID, "feedback shape")
		}
		var valid bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM job_steps
				WHERE id=$1 AND job_id=$2 AND generation=$3
				  AND status='completed' AND output=$4
			)
		`, *stepID, job.ID, job.CurrentGeneration, command.Text).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return channelSessionReplayError(command.OperationID, "completed input step")
		}
	default:
		return channelSessionReplayError(command.OperationID, "registered disposition")
	}
	return nil
}

func channelSessionReplayError(operationID LifecycleOperationID, subject string) error {
	return fmt.Errorf(
		"channel session operation %q has no exact persisted %s authority",
		operationID,
		subject,
	)
}
