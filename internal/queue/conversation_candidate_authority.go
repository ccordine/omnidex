package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
)

const maxConversationCandidateExchanges = 6

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
// complete assistant exchanges preceding this job's bound user message.
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
	if binding.Mode != model.ChannelModeAssistant {
		return ConversationCandidateAuthoritySet{}, fmt.Errorf(
			"assistant conversation candidates require an assistant channel",
		)
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
		WHERE channel_id=$1 AND id<$2 AND role='assistant'
		ORDER BY id DESC
		LIMIT $3
	`, binding.ChannelID, binding.UserMessageID, maxConversationCandidateExchanges)
	if err != nil {
		return ConversationCandidateAuthoritySet{}, err
	}
	defer rows.Close()
	descendingAssistants := make([]model.ChannelMessage, 0, maxConversationCandidateExchanges)
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
		if message.Role != model.ChannelMessageRoleAssistant {
			return ConversationCandidateAuthoritySet{}, fmt.Errorf(
				"conversation candidate %d is not an assistant message", message.ID,
			)
		}
		descendingAssistants = append(descendingAssistants, message)
	}
	if err := rows.Err(); err != nil {
		return ConversationCandidateAuthoritySet{}, err
	}
	rows.Close()
	type completeExchange struct {
		user      model.ChannelMessage
		assistant model.ChannelMessage
		result    ConversationAssistantResultAuthority
		followups []conversationFollowup
	}
	descendingExchanges := make([]completeExchange, 0, len(descendingAssistants))
	for _, assistant := range descendingAssistants {
		user, err := loadImmediatelyPrecedingConversationUser(
			ctx, tx, binding.ChannelID, assistant.ID,
		)
		if err != nil {
			return ConversationCandidateAuthoritySet{}, err
		}
		result, err := loadExactConversationAssistantResult(
			ctx, tx, binding.ChannelID, binding.Mode, user, assistant,
		)
		if err != nil {
			return ConversationCandidateAuthoritySet{}, err
		}
		followups, err := loadConversationSessionFollowupsTx(
			ctx,
			tx,
			binding.ChannelID,
			result.JobID,
		)
		if err != nil {
			return ConversationCandidateAuthoritySet{}, err
		}
		descendingExchanges = append(descendingExchanges, completeExchange{
			user: user, assistant: assistant, result: result, followups: followups,
		})
	}
	set := ConversationCandidateAuthoritySet{
		Turns:            make([]ConversationCandidateTurn, 0, len(descendingExchanges)*2),
		AssistantResults: make([]ConversationAssistantResultAuthority, 0, len(descendingExchanges)),
	}
	for index := len(descendingExchanges) - 1; index >= 0; index-- {
		exchange := descendingExchanges[index]
		set.Turns = append(set.Turns, ConversationCandidateTurn{
			MessageID: exchange.user.ID, Role: ConversationCandidateUser,
			Content: projectConversationSessionTurns(exchange.user.Content, exchange.followups),
		})
		set.Turns = append(set.Turns, ConversationCandidateTurn{
			MessageID: exchange.assistant.ID, Role: ConversationCandidateAssistant,
			PairedUserMessageID: exchange.user.ID,
			SpeakerName:         exchange.result.SpeakerName, Content: exchange.assistant.Content,
		})
		set.AssistantResults = append(set.AssistantResults, exchange.result)
	}
	if err := tx.Commit(ctx); err != nil {
		return ConversationCandidateAuthoritySet{}, err
	}
	return set, nil
}

func loadImmediatelyPrecedingConversationUser(
	ctx context.Context,
	tx pgx.Tx,
	channelID string,
	assistantMessageID int64,
) (model.ChannelMessage, error) {
	var user model.ChannelMessage
	err := tx.QueryRow(ctx, `
		SELECT id, channel_id, role, content, created_at
		FROM ai_channel_messages
		WHERE channel_id=$1 AND id<$2
		ORDER BY id DESC
		LIMIT 1
	`, channelID, assistantMessageID).Scan(
		&user.ID, &user.ChannelID, &user.Role, &user.Content, &user.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return model.ChannelMessage{}, fmt.Errorf(
			"assistant message %d has no immediately preceding user authority",
			assistantMessageID,
		)
	}
	if err != nil {
		return model.ChannelMessage{}, err
	}
	if err := model.ValidateChannelMessage(user.Role, user.Content); err != nil {
		return model.ChannelMessage{}, fmt.Errorf(
			"invalid stored conversation candidate %d: %w", user.ID, err,
		)
	}
	if user.Role != model.ChannelMessageRoleUser {
		return model.ChannelMessage{}, fmt.Errorf(
			"assistant message %d is not immediately preceded by a user authority",
			assistantMessageID,
		)
	}
	return user, nil
}

func loadExactConversationAssistantResult(
	ctx context.Context,
	tx pgx.Tx,
	channelID string,
	mode model.ChannelMode,
	user model.ChannelMessage,
	message model.ChannelMessage,
) (ConversationAssistantResultAuthority, error) {
	rows, err := tx.Query(ctx, `
			SELECT id,instruction,result
			FROM jobs
			WHERE pipeline='chat' AND status='completed'
			  AND metadata->>'channel_id'=$1
			  AND metadata->>'channel_user_message_id'=($2::bigint)::text
			ORDER BY id ASC LIMIT 2
		`, channelID, user.ID)
	if err != nil {
		return ConversationAssistantResultAuthority{}, err
	}
	defer rows.Close()
	type completion struct {
		jobID       int64
		instruction string
		result      string
	}
	completions := make([]completion, 0, 2)
	for rows.Next() {
		var value completion
		if err := rows.Scan(&value.jobID, &value.instruction, &value.result); err != nil {
			return ConversationAssistantResultAuthority{}, err
		}
		completions = append(completions, value)
	}
	if err := rows.Err(); err != nil {
		return ConversationAssistantResultAuthority{}, err
	}
	if len(completions) != 1 || completions[0].instruction != user.Content ||
		completions[0].result != message.Content {
		return ConversationAssistantResultAuthority{}, fmt.Errorf(
			"assistant message %d is not the unique exact result for user message %d",
			message.ID, user.ID,
		)
	}
	speakerName, err := loadConversationAssistantSpeaker(
		ctx, tx, channelID, mode, message.ID,
	)
	if err != nil {
		return ConversationAssistantResultAuthority{}, err
	}
	return ConversationAssistantResultAuthority{
		UserMessageID: user.ID,
		MessageID:     message.ID,
		JobID:         completions[0].jobID,
		SpeakerName:   speakerName,
		Content:       message.Content,
	}, nil
}
