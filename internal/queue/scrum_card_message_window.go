package queue

import (
	"context"
	"fmt"
)

func loadScrumCardMessageWindow(
	ctx context.Context,
	query scrumCardMessageQueryer,
	projectID int64,
	cardID string,
	start, total int64,
) ([]ScrumCardMessage, error) {
	if start < 0 || total < start || total-start > MaxScrumChannelPageSize {
		return nil, fmt.Errorf("Scrum message window %d..%d is outside bounded authority", start, total)
	}
	rows, err := query.Query(ctx, `
		SELECT ordinal,message_id,role,content,created_at,source_created_at,
		       timestamp_origin,status,COALESCE(operation_id,'')
		FROM scrum_card_messages
		WHERE project_id=$1 AND card_id=$2 AND ordinal>$3 AND ordinal<=$4
		ORDER BY ordinal
	`, projectID, cardID, start, total)
	if err != nil {
		return nil, fmt.Errorf("load exact Scrum message window: %w", err)
	}
	defer rows.Close()
	messages := make([]ScrumCardMessage, 0, total-start)
	for rows.Next() {
		var message ScrumCardMessage
		if err := rows.Scan(
			&message.Ordinal, &message.ID, &message.Role, &message.Content,
			&message.CreatedAt, &message.SourceCreatedAt, &message.TimestampOrigin,
			&message.Status, &message.OperationID,
		); err != nil {
			return nil, fmt.Errorf("scan exact Scrum message window: %w", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read exact Scrum message window: %w", err)
	}
	if int64(len(messages)) != total-start {
		return nil, fmt.Errorf(
			"Scrum message window contains %d rows, expected %d", len(messages), total-start,
		)
	}
	contentBytes := 0
	for index, message := range messages {
		if message.Ordinal != start+int64(index)+1 {
			return nil, fmt.Errorf("Scrum message window has noncanonical ordinal %d", message.Ordinal)
		}
		contentBytes += len(message.Content)
	}
	if contentBytes > MaxScrumChannelPageBytes {
		return nil, fmt.Errorf("Scrum message window exceeds its %d-byte authority", MaxScrumChannelPageBytes)
	}
	return messages, nil
}
