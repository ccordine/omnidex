package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func insertChannelMessageForTest(
	t *testing.T,
	repository *Repository,
	channelID model.ChannelID,
	role model.ChannelMessageRole,
	content string,
) (model.ChannelMessage, error) {
	t.Helper()
	var message model.ChannelMessage
	err := repository.pool.QueryRow(t.Context(), `
		INSERT INTO ai_channel_messages (channel_id,role,content)
		VALUES ($1,$2,$3)
		RETURNING id,channel_id,role,content,created_at
	`, channelID, role, content).Scan(
		&message.ID, &message.ChannelID, &message.Role, &message.Content, &message.CreatedAt,
	)
	return message, err
}
