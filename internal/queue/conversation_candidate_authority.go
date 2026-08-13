package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type ConversationCandidateAuthoritySet struct {
	Turns            []assemblyline.ConversationContextTurn
	AssistantResults []assemblyline.ConversationSelectedAssistantResult
}

// ConversationCandidateAuthorities returns one immutable, bounded suffix of
// the exact channel transcript preceding this job's bound user message.
// Recency is only this provider's candidate policy; semantic selection and
// downstream authority remain independent of the retrieval mechanism.
func (r *Repository) ConversationCandidateAuthorities(
	ctx context.Context,
	job model.Job,
) (ConversationCandidateAuthoritySet, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return ConversationCandidateAuthoritySet{}, fmt.Errorf("conversation candidates require PostgreSQL and context")
	}
	binding, exists, err := channelBindingForJob(job)
	if err != nil {
		return ConversationCandidateAuthoritySet{}, err
	}
	if !exists {
		return ConversationCandidateAuthoritySet{}, fmt.Errorf("conversation job has no channel message authority")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return ConversationCandidateAuthoritySet{}, err
	}
	defer tx.Rollback(ctx)
	var current model.ChannelMessage
	if err := tx.QueryRow(ctx, `
		SELECT id, channel_id, role, content, created_at
		FROM ai_channel_messages
		WHERE id=$1 AND channel_id=$2
	`, binding.UserMessageID, binding.ChannelID).Scan(
		&current.ID, &current.ChannelID, &current.Role, &current.Content, &current.CreatedAt,
	); err == pgx.ErrNoRows {
		return ConversationCandidateAuthoritySet{}, fmt.Errorf("current channel user message authority is absent")
	} else if err != nil {
		return ConversationCandidateAuthoritySet{}, err
	}
	if current.Role != model.ChannelMessageRoleUser || current.Content != job.Instruction {
		return ConversationCandidateAuthoritySet{}, fmt.Errorf("current channel message differs from exact job authority")
	}
	rows, err := tx.Query(ctx, `
		SELECT id, channel_id, role, content, created_at
		FROM ai_channel_messages
		WHERE channel_id=$1 AND id<$2
		ORDER BY id DESC
		LIMIT $3
	`, binding.ChannelID, binding.UserMessageID, assemblyline.MaxConversationContextCandidateAuthorities)
	if err != nil {
		return ConversationCandidateAuthoritySet{}, err
	}
	defer rows.Close()
	descending := make([]model.ChannelMessage, 0, assemblyline.MaxConversationContextCandidateAuthorities)
	totalBytes := 0
	for rows.Next() {
		var message model.ChannelMessage
		if err := rows.Scan(
			&message.ID, &message.ChannelID, &message.Role, &message.Content, &message.CreatedAt,
		); err != nil {
			return ConversationCandidateAuthoritySet{}, err
		}
		if err := model.ValidateChannelMessage(message.Role, message.Content); err != nil {
			return ConversationCandidateAuthoritySet{}, fmt.Errorf("invalid stored conversation candidate %d: %w", message.ID, err)
		}
		if totalBytes+len(message.Content) > assemblyline.MaxConversationContextCandidateBytes {
			if len(descending) == 0 {
				return ConversationCandidateAuthoritySet{}, fmt.Errorf(
					"nearest conversation candidate exceeds the %d-byte bound",
					assemblyline.MaxConversationContextCandidateBytes,
				)
			}
			break
		}
		totalBytes += len(message.Content)
		descending = append(descending, message)
	}
	if err := rows.Err(); err != nil {
		return ConversationCandidateAuthoritySet{}, err
	}
	rows.Close()
	for left, right := 0, len(descending)-1; left < right; left, right = left+1, right-1 {
		descending[left], descending[right] = descending[right], descending[left]
	}
	for len(descending) > 0 && descending[0].Role == model.ChannelMessageRoleAssistant {
		descending = descending[1:]
	}
	set := ConversationCandidateAuthoritySet{
		Turns: make([]assemblyline.ConversationContextTurn, 0, len(descending)),
	}
	var pendingUserID int64
	for _, message := range descending {
		role := assemblyline.ConversationContextUser
		if message.Role == model.ChannelMessageRoleAssistant {
			role = assemblyline.ConversationContextAssistant
		}
		if message.Role == model.ChannelMessageRoleUser {
			set.Turns = append(set.Turns, assemblyline.ConversationContextTurn{
				MessageID: message.ID, Role: role, Content: message.Content,
			})
			pendingUserID = message.ID
			continue
		}
		if pendingUserID < 1 {
			return ConversationCandidateAuthoritySet{}, fmt.Errorf(
				"assistant message %d has no preceding candidate user authority", message.ID,
			)
		}
		result, err := loadExactConversationAssistantResult(
			ctx, tx, binding.ChannelID, pendingUserID, message,
		)
		if err != nil {
			return ConversationCandidateAuthoritySet{}, err
		}
		set.Turns = append(set.Turns, assemblyline.ConversationContextTurn{
			MessageID: message.ID, Role: role, PairedUserMessageID: pendingUserID, Content: message.Content,
		})
		set.AssistantResults = append(set.AssistantResults, result)
		pendingUserID = 0
	}
	if err := tx.Commit(ctx); err != nil {
		return ConversationCandidateAuthoritySet{}, err
	}
	return set, nil
}

func loadExactConversationAssistantResult(
	ctx context.Context,
	tx pgx.Tx,
	channelID string,
	userMessageID int64,
	message model.ChannelMessage,
) (assemblyline.ConversationSelectedAssistantResult, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, result
		FROM jobs
		WHERE pipeline='chat' AND status='completed'
		  AND metadata->>'channel_id'=$1
		  AND metadata->>'channel_user_message_id'=($2::bigint)::text
		ORDER BY id ASC LIMIT 2
	`, channelID, userMessageID)
	if err != nil {
		return assemblyline.ConversationSelectedAssistantResult{}, err
	}
	defer rows.Close()
	type completion struct {
		jobID  int64
		result string
	}
	completions := make([]completion, 0, 2)
	for rows.Next() {
		var value completion
		if err := rows.Scan(&value.jobID, &value.result); err != nil {
			return assemblyline.ConversationSelectedAssistantResult{}, err
		}
		completions = append(completions, value)
	}
	if err := rows.Err(); err != nil {
		return assemblyline.ConversationSelectedAssistantResult{}, err
	}
	if len(completions) != 1 || completions[0].result != message.Content {
		return assemblyline.ConversationSelectedAssistantResult{}, fmt.Errorf(
			"assistant message %d is not the unique exact result for user message %d",
			message.ID, userMessageID,
		)
	}
	return assemblyline.ConversationSelectedAssistantResult{
		UserMessageID: userMessageID,
		MessageID:     message.ID,
		JobID:         completions[0].jobID,
		Content:       message.Content,
	}, nil
}
