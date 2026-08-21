package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
)

const (
	maxConversationCandidateAuthorities = 8
	maxConversationCandidateBytes       = 6 * 1024
)

type ConversationCandidateRole string

const (
	ConversationCandidateUser      ConversationCandidateRole = "user"
	ConversationCandidateAssistant ConversationCandidateRole = "assistant"
)

type ConversationCandidateTurn struct {
	MessageID           int64
	Role                ConversationCandidateRole
	PairedUserMessageID int64
	SpeakerName         string
	RoleplayUserTurn    *roleplay.UserTurnAuthority
	Content             string
}

type ConversationAssistantResultAuthority struct {
	UserMessageID int64
	MessageID     int64
	JobID         int64
	SpeakerName   string
	Content       string
}

type ConversationCandidateAuthoritySet struct {
	Turns            []ConversationCandidateTurn
	AssistantResults []ConversationAssistantResultAuthority
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
	`, binding.ChannelID, binding.UserMessageID, maxConversationCandidateAuthorities)
	if err != nil {
		return ConversationCandidateAuthoritySet{}, err
	}
	defer rows.Close()
	descending := make([]model.ChannelMessage, 0, maxConversationCandidateAuthorities)
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
		if totalBytes+len(message.Content) > maxConversationCandidateBytes {
			if len(descending) == 0 {
				return ConversationCandidateAuthoritySet{}, fmt.Errorf(
					"nearest conversation candidate exceeds the %d-byte bound",
					maxConversationCandidateBytes,
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
		Turns: make([]ConversationCandidateTurn, 0, len(descending)),
	}
	var pendingUserID int64
	for _, message := range descending {
		role := ConversationCandidateUser
		if message.Role == model.ChannelMessageRoleAssistant {
			role = ConversationCandidateAssistant
		}
		if message.Role == model.ChannelMessageRoleUser {
			turn := ConversationCandidateTurn{
				MessageID: message.ID, Role: role, Content: message.Content,
			}
			if binding.Mode == model.ChannelModeRoleplay {
				userTurn, err := loadConversationRoleplayUserTurn(
					ctx, tx, binding.ChannelID, message.ID, message.Content,
				)
				if err != nil {
					return ConversationCandidateAuthoritySet{}, err
				}
				turn.SpeakerName = userTurn.PersonaName
				turn.RoleplayUserTurn = &userTurn
			}
			set.Turns = append(set.Turns, turn)
			pendingUserID = message.ID
			continue
		}
		if pendingUserID < 1 {
			return ConversationCandidateAuthoritySet{}, fmt.Errorf(
				"assistant message %d has no preceding candidate user authority", message.ID,
			)
		}
		result, err := loadExactConversationAssistantResult(
			ctx, tx, binding.ChannelID, binding.Mode, pendingUserID, message,
		)
		if err != nil {
			return ConversationCandidateAuthoritySet{}, err
		}
		set.Turns = append(set.Turns, ConversationCandidateTurn{
			MessageID: message.ID, Role: role, PairedUserMessageID: pendingUserID,
			SpeakerName: result.SpeakerName, Content: message.Content,
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
	mode model.ChannelMode,
	userMessageID int64,
	message model.ChannelMessage,
) (ConversationAssistantResultAuthority, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, result
		FROM jobs
		WHERE pipeline='chat' AND status='completed'
		  AND metadata->>'channel_id'=$1
		  AND metadata->>'channel_user_message_id'=($2::bigint)::text
		ORDER BY id ASC LIMIT 2
	`, channelID, userMessageID)
	if err != nil {
		return ConversationAssistantResultAuthority{}, err
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
			return ConversationAssistantResultAuthority{}, err
		}
		completions = append(completions, value)
	}
	if err := rows.Err(); err != nil {
		return ConversationAssistantResultAuthority{}, err
	}
	if len(completions) != 1 || completions[0].result != message.Content {
		return ConversationAssistantResultAuthority{}, fmt.Errorf(
			"assistant message %d is not the unique exact result for user message %d",
			message.ID, userMessageID,
		)
	}
	speakerName, err := loadConversationAssistantSpeaker(
		ctx, tx, channelID, mode, message.ID,
	)
	if err != nil {
		return ConversationAssistantResultAuthority{}, err
	}
	return ConversationAssistantResultAuthority{
		UserMessageID: userMessageID,
		MessageID:     message.ID,
		JobID:         completions[0].jobID,
		SpeakerName:   speakerName,
		Content:       message.Content,
	}, nil
}
