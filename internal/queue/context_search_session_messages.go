package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
)

type rankedConversationContextRecord struct {
	record    ContextSearchRecord
	rank      float32
	messageID int64
}

func (r *Repository) searchConversationSessionRecords(ctx context.Context, binding channelCompletionBinding, terms []string, limit int) ([]rankedConversationContextRecord, error) {
	rows, err := r.pool.Query(ctx, `WITH `+conversationSessionRepliesCTE+`, query_terms AS (
	 SELECT websearch_to_tsquery('simple',term) AS query FROM unnest($3::text[]) AS term
	), ranked AS (
	 SELECT reply.*,MAX(ts_rank_cd(to_tsvector('simple',
	   (reply.intake->'command'->>'text') || E'\n' || reply.content),query_terms.query)) AS rank
	 FROM session_replies AS reply JOIN query_terms ON to_tsvector('simple',
	   (reply.intake->'command'->>'text') || E'\n' || reply.content) @@ query_terms.query
	 WHERE reply.channel_id=$1 AND reply.response_message_id<$2
	 GROUP BY reply.operation_id,reply.intake,reply.resolution,reply.response_message_id,reply.channel_id,reply.content
	)
	SELECT operation_id,intake,resolution,response_message_id,channel_id,content,rank
	FROM ranked ORDER BY rank DESC,response_message_id DESC LIMIT $4`, binding.ChannelID, binding.UserMessageID, terms, limit)
	if err != nil {
		return nil, fmt.Errorf("search exact queued conversation context: %w", err)
	}
	defer rows.Close()
	records := make([]rankedConversationContextRecord, 0, limit)
	for rows.Next() {
		var operationID model.LifecycleOperationID
		var intakeRaw, resolutionRaw []byte
		var assistant model.ChannelMessage
		var rank float32
		assistant.Role = model.ChannelMessageRoleAssistant
		if err := rows.Scan(&operationID, &intakeRaw, &resolutionRaw, &assistant.ID, &assistant.ChannelID, &assistant.Content, &rank); err != nil {
			return nil, err
		}
		exchange, err := conversationSessionExchangeAuthority(operationID, intakeRaw, resolutionRaw, assistant)
		if err != nil {
			return nil, err
		}
		records = append(records, rankedConversationContextRecord{record: exchange.ContextRecord(), rank: rank, messageID: assistant.ID})
	}
	return records, rows.Err()
}
