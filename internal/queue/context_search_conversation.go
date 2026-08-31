package queue

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
)

// SearchConversationContextRecords returns complete, completed Assistant
// exchanges. It never projects a matching user or assistant fragment without
// its exact adjacent counterpart.
func (r *Repository) SearchConversationContextRecords(
	ctx context.Context,
	job model.Job,
	terms []string,
	limit int,
) ([]ContextSearchRecord, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return nil, fmt.Errorf("conversation context search requires PostgreSQL and context")
	}
	if err := validateContextSearchRequest(terms, limit); err != nil {
		return nil, err
	}
	if len(terms) == 0 {
		return []ContextSearchRecord{}, nil
	}
	binding, exists, err := channelBindingForJob(job)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("conversation context search requires exact channel message authority")
	}
	if binding.Mode != model.ChannelModeAssistant {
		return nil, fmt.Errorf("conversation context search requires an assistant channel")
	}
	rows, err := r.pool.Query(ctx, `
		WITH query_terms AS (
			SELECT websearch_to_tsquery('simple', term) AS query
			FROM unnest($3::text[]) AS term
		), conversation_followups AS (
			SELECT channel_id,job_id,
			       string_agg(
			           context_text,''
			           ORDER BY generation ASC,phase ASC,created_at ASC,operation_id ASC
			       ) AS content
			FROM channel_conversation_followup_events
			WHERE channel_id=$1
			GROUP BY channel_id,job_id
		), completed AS (
			SELECT job.id AS job_id,job.instruction,
			       COALESCE(job.result,'') AS result,
			       job.result IS NOT NULL AS result_present,
			       message.id AS user_message_id,message.content AS user_content,
			       message.content || COALESCE(followups.content,'') AS projected_user_content,
			       COUNT(*) OVER (PARTITION BY message.id) AS completion_count
			FROM jobs AS job
			JOIN ai_channel_messages AS message
			  ON message.channel_id=$1 AND message.role='user'
			 AND message.id::text=job.metadata->>'channel_user_message_id'
			LEFT JOIN conversation_followups AS followups
			  ON followups.channel_id=message.channel_id AND followups.job_id=job.id
			WHERE job.pipeline='chat' AND job.status='completed'
			  AND job.metadata->>'channel_id'=$1 AND message.id<$2
		), exchanges AS (
			SELECT completed.*,
			       assistant.id AS assistant_message_id,
			       assistant.role AS assistant_role,
			       assistant.content AS assistant_content
			FROM completed
			LEFT JOIN LATERAL (
				SELECT message.id,message.role,message.content
				FROM ai_channel_messages AS message
				WHERE message.channel_id=$1 AND message.id>completed.user_message_id
				ORDER BY message.id ASC
				LIMIT 1
			) AS assistant ON TRUE
		), ranked AS (
			SELECT exchanges.*,
			       MAX(ts_rank_cd(
			           to_tsvector('simple',exchanges.projected_user_content || E'\n' ||
			               COALESCE(exchanges.assistant_content,'')),
			           query_terms.query
			       )) AS rank
			FROM exchanges
			JOIN query_terms ON to_tsvector(
			    'simple',exchanges.projected_user_content || E'\n' ||
			    COALESCE(exchanges.assistant_content,'')
			) @@ query_terms.query
			GROUP BY exchanges.job_id,exchanges.instruction,exchanges.result,
			         exchanges.result_present,
			         exchanges.user_message_id,exchanges.user_content,
			         exchanges.projected_user_content,
			         exchanges.completion_count,exchanges.assistant_message_id,
			         exchanges.assistant_role,exchanges.assistant_content
		)
		SELECT job_id,instruction,result,result_present,
		       user_message_id,user_content,projected_user_content,completion_count,
		       COALESCE(assistant_message_id,0),COALESCE(assistant_role,''),
		       COALESCE(assistant_content,''),rank
		FROM ranked
		ORDER BY rank DESC,user_message_id DESC
		LIMIT $4
	`, binding.ChannelID, binding.UserMessageID, terms, limit)
	if err != nil {
		return nil, fmt.Errorf("search exact channel transcript context: %w", err)
	}
	defer rows.Close()
	records := make([]ContextSearchRecord, 0, limit)
	seenUsers := make(map[int64]struct{}, limit)
	for rows.Next() {
		var (
			jobID, userID, assistantID int64
			instruction, result        string
			userContent                string
			projectedUserContent       string
			resultPresent              bool
			completionCount            int64
			assistantRole              model.ChannelMessageRole
			assistantContent           string
			rank                       float32
		)
		if err := rows.Scan(
			&jobID, &instruction, &result, &resultPresent,
			&userID, &userContent, &projectedUserContent, &completionCount,
			&assistantID, &assistantRole, &assistantContent, &rank,
		); err != nil {
			return nil, fmt.Errorf("scan searched conversation exchange: %w", err)
		}
		if jobID < 1 || completionCount != 1 {
			return nil, fmt.Errorf(
				"searched user message %d does not have one unique completed job authority", userID,
			)
		}
		if !resultPresent || instruction != userContent {
			return nil, fmt.Errorf(
				"searched user message %d differs from its exact completed job authority", userID,
			)
		}
		if _, duplicate := seenUsers[userID]; duplicate {
			return nil, fmt.Errorf("searched user message %d is duplicated", userID)
		}
		seenUsers[userID] = struct{}{}
		if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, userContent); err != nil {
			return nil, fmt.Errorf("searched user message %d is invalid: %w", userID, err)
		}
		if !utf8.ValidString(projectedUserContent) || strings.ContainsRune(projectedUserContent, '\x00') ||
			!strings.HasPrefix(projectedUserContent, userContent) {
			return nil, fmt.Errorf(
				"searched user message %d has invalid persisted follow-up projection",
				userID,
			)
		}
		if assistantID <= userID || assistantRole != model.ChannelMessageRoleAssistant ||
			assistantContent != result {
			return nil, fmt.Errorf(
				"searched user message %d has no exact adjacent assistant result authority", userID,
			)
		}
		if err := model.ValidateChannelMessage(assistantRole, assistantContent); err != nil {
			return nil, fmt.Errorf("searched assistant message %d is invalid: %w", assistantID, err)
		}
		records = append(records, ContextSearchRecord{
			Namespace: "conversation_exchange",
			SourceID: fmt.Sprintf(
				"channel-message-%d-through-%d", userID, assistantID,
			),
			Content: fmt.Sprintf(
				"user message:\n%s\nassistant response:\n%s",
				projectedUserContent,
				assistantContent,
			),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}
