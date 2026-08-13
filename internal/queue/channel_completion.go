package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type channelCompletionBinding struct {
	ChannelID     string
	UserMessageID int64
	ProjectID     int64
}

func channelBindingForJob(job model.Job) (channelCompletionBinding, bool, error) {
	if len(job.Metadata) == 0 {
		return channelCompletionBinding{}, false, nil
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(job.Metadata, &metadata); err != nil {
		return channelCompletionBinding{}, false, fmt.Errorf("decode job channel completion metadata: %w", err)
	}
	_, hasChannel := metadata["channel_id"]
	_, hasMessage := metadata["channel_user_message_id"]
	if !hasChannel && !hasMessage {
		return channelCompletionBinding{}, false, nil
	}
	if !hasChannel || !hasMessage {
		return channelCompletionBinding{}, false, fmt.Errorf("channel completion metadata is incomplete")
	}
	var metadataBinding channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &metadataBinding); err != nil {
		return channelCompletionBinding{}, false, fmt.Errorf("decode exact channel completion binding: %w", err)
	}
	if err := validateChannelTurnMetadata(metadataBinding); err != nil {
		return channelCompletionBinding{}, false, fmt.Errorf("channel completion metadata: %w", err)
	}
	if job.Pipeline != model.PipelineChat {
		return channelCompletionBinding{}, false, fmt.Errorf("channel completion metadata requires chat pipeline")
	}
	binding := channelCompletionBinding{
		ChannelID: string(metadataBinding.ChannelID), UserMessageID: metadataBinding.ChannelUserMessageID,
		ProjectID: metadataBinding.ProjectID,
	}
	return binding, true, nil
}

func materializeChannelCompletionTx(
	ctx context.Context,
	tx pgx.Tx,
	job model.Job,
	output string,
) error {
	binding, exists, err := channelBindingForJob(job)
	if err != nil || !exists {
		return err
	}
	if strings.TrimSpace(output) == "" {
		return fmt.Errorf("channel completion output is required")
	}
	if err := model.ValidateChannelMessage(model.ChannelMessageRoleAssistant, output); err != nil {
		return fmt.Errorf("channel completion output: %w", err)
	}
	var userContent string
	err = tx.QueryRow(ctx, `
		SELECT content FROM ai_channel_messages
		WHERE id=$1 AND channel_id=$2 AND role='user'
	`, binding.UserMessageID, binding.ChannelID).Scan(&userContent)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("channel completion user message authority is absent")
	}
	if err != nil {
		return err
	}
	if userContent != job.Instruction {
		return fmt.Errorf("channel completion user message differs from exact job authority")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_channel_messages (channel_id, role, content)
		VALUES ($1, 'assistant', $2)
	`, binding.ChannelID, output); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE ai_channels SET updated_at=NOW() WHERE id=$1`, binding.ChannelID)
	return err
}
