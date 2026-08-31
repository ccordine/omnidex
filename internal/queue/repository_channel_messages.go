package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
)

type channelMessageQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func (r *Repository) ListChannelMessages(
	ctx context.Context,
	channelID model.ChannelID,
	limit int,
	beforeID *int64,
) (model.ChannelMessagePage, error) {
	if r == nil || r.pool == nil {
		return model.ChannelMessagePage{}, fmt.Errorf("channel message list requires PostgreSQL")
	}
	return listChannelMessages(ctx, r.pool, channelID, limit, beforeID)
}

func listChannelMessages(
	ctx context.Context,
	querier channelMessageQuerier,
	channelID model.ChannelID,
	limit int,
	beforeID *int64,
) (model.ChannelMessagePage, error) {
	if ctx == nil || querier == nil {
		return model.ChannelMessagePage{}, fmt.Errorf("channel message list requires PostgreSQL and context")
	}
	if err := channelID.Validate(); err != nil {
		return model.ChannelMessagePage{}, err
	}
	if limit <= 0 || limit > 200 {
		return model.ChannelMessagePage{}, fmt.Errorf("channel message limit must be between 1 and 200")
	}
	if beforeID != nil && *beforeID < 1 {
		return model.ChannelMessagePage{}, fmt.Errorf("channel message cursor must be positive")
	}
	var exists bool
	if err := querier.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM ai_channels WHERE id=$1)`, channelID).Scan(&exists); err != nil {
		return model.ChannelMessagePage{}, err
	}
	if !exists {
		return model.ChannelMessagePage{}, pgx.ErrNoRows
	}
	rows, err := querier.Query(ctx, `
		SELECT message.id,message.channel_id,message.role,message.content,message.created_at,
		       channel.mode,COALESCE(fictional_character.name,research_character.name),
		       fictional.source_message_id IS NOT NULL,research.source_message_id IS NOT NULL,
		       user_turn.authority,
		       turn_job.id,turn_job.status,turn_job.error,turn_job.updated_at,
		       turn_job.binding_count
		FROM ai_channel_messages AS message
		JOIN ai_channels AS channel ON channel.id=message.channel_id
		LEFT JOIN roleplay_turn_completions AS fictional
		  ON fictional.source_message_id=message.id
		LEFT JOIN roleplay_characters AS fictional_character
		  ON fictional_character.world_id=fictional.world_id
		 AND fictional_character.id=fictional.viewpoint_character_id
		LEFT JOIN roleplay_research_completions AS research
		  ON research.source_message_id=message.id
		LEFT JOIN roleplay_research_turns AS research_turn
		  ON research_turn.preparation_id=research.preparation_id
		LEFT JOIN roleplay_characters AS research_character
		  ON research_character.world_id=research_turn.world_id
		 AND research_character.id=research_turn.character_id
		LEFT JOIN roleplay_user_turns AS user_turn
		  ON user_turn.user_message_id=message.id AND user_turn.channel_id=message.channel_id
		LEFT JOIN LATERAL (
			SELECT candidate.id,candidate.status,candidate.error,candidate.updated_at,
			       COUNT(*) OVER () AS binding_count
			FROM jobs AS candidate
			WHERE message.role='user' AND candidate.pipeline='chat'
			  AND candidate.metadata->>'channel_id'=message.channel_id
			  AND candidate.metadata->>'channel_user_message_id'=message.id::text
			ORDER BY candidate.id ASC
			LIMIT 1
		) AS turn_job ON TRUE
		WHERE message.channel_id=$1 AND ($3::bigint IS NULL OR message.id<$3)
		ORDER BY message.id DESC LIMIT $2
	`, channelID, limit+1, beforeID)
	if err != nil {
		return model.ChannelMessagePage{}, err
	}
	defer rows.Close()
	messages := []model.ChannelMessage{}
	for rows.Next() {
		message, err := scanPresentedChannelMessage(rows)
		if err != nil {
			return model.ChannelMessagePage{}, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return model.ChannelMessagePage{}, err
	}
	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
	var nextBeforeID *int64
	if hasMore && len(messages) > 0 {
		value := messages[0].ID
		nextBeforeID = &value
	}
	return model.ChannelMessagePage{Messages: messages, NextBeforeID: nextBeforeID, HasMore: hasMore}, nil
}

func scanPresentedChannelMessage(row channelRowScanner) (model.ChannelMessage, error) {
	var message model.ChannelMessage
	var mode model.ChannelMode
	var speakerName *string
	var fictionalCompletion, researchCompletion bool
	var userTurnAuthority []byte
	var turnJobID, turnBindingCount *int64
	var turnStatus, turnError *string
	var turnUpdatedAt *time.Time
	if err := row.Scan(
		&message.ID, &message.ChannelID, &message.Role, &message.Content, &message.CreatedAt,
		&mode, &speakerName, &fictionalCompletion, &researchCompletion,
		&userTurnAuthority,
		&turnJobID, &turnStatus, &turnError, &turnUpdatedAt, &turnBindingCount,
	); err != nil {
		return model.ChannelMessage{}, err
	}
	if err := model.ValidateChannelMessage(message.Role, message.Content); err != nil {
		return model.ChannelMessage{}, fmt.Errorf("invalid stored channel message %d: %w", message.ID, err)
	}
	if err := mode.Validate(); err != nil {
		return model.ChannelMessage{}, fmt.Errorf("invalid stored channel message %d mode: %w", message.ID, err)
	}
	if message.Role == model.ChannelMessageRoleUser {
		if fictionalCompletion || researchCompletion || speakerName != nil {
			return model.ChannelMessage{}, fmt.Errorf(
				"channel user message %d carries assistant completion authority", message.ID,
			)
		}
		if err := presentChannelUserTurn(&message, mode, userTurnAuthority); err != nil {
			return model.ChannelMessage{}, err
		}
		if err := presentChannelTurnState(
			&message, turnJobID, turnStatus, turnError, turnUpdatedAt, turnBindingCount,
		); err != nil {
			return model.ChannelMessage{}, err
		}
	} else if mode == model.ChannelModeRoleplay {
		if fictionalCompletion == researchCompletion || speakerName == nil {
			return model.ChannelMessage{}, fmt.Errorf(
				"roleplay assistant message %d requires one exact persisted speaker authority", message.ID,
			)
		}
		message.SpeakerName = *speakerName
		if userTurnAuthority != nil ||
			turnJobID != nil || turnStatus != nil || turnError != nil || turnUpdatedAt != nil ||
			turnBindingCount != nil {
			return model.ChannelMessage{}, fmt.Errorf(
				"roleplay assistant message %d carries user-turn authority", message.ID,
			)
		}
	} else if fictionalCompletion || researchCompletion || speakerName != nil ||
		userTurnAuthority != nil ||
		turnJobID != nil || turnStatus != nil || turnError != nil || turnUpdatedAt != nil ||
		turnBindingCount != nil {
		return model.ChannelMessage{}, fmt.Errorf(
			"assistant channel message %d carries roleplay or user-turn presentation authority", message.ID,
		)
	}
	if err := model.ValidateChannelMessageSpeaker(message.Role, message.SpeakerName); err != nil {
		return model.ChannelMessage{}, fmt.Errorf("invalid stored channel message %d speaker: %w", message.ID, err)
	}
	return message, nil
}

func presentChannelUserTurn(
	message *model.ChannelMessage,
	mode model.ChannelMode,
	rawAuthority []byte,
) error {
	if mode == model.ChannelModeAssistant {
		if rawAuthority != nil {
			return fmt.Errorf("assistant user message %d carries fictional turn authority", message.ID)
		}
		return nil
	}
	if rawAuthority == nil {
		return fmt.Errorf("roleplay user message %d has incomplete persona authority", message.ID)
	}
	var authority roleplay.UserTurnAuthority
	if err := json.Unmarshal(rawAuthority, &authority); err != nil {
		return fmt.Errorf("decode roleplay user message %d authority: %w", message.ID, err)
	}
	if err := authority.Validate(); err != nil {
		return fmt.Errorf("roleplay user message %d persona authority: %w", message.ID, err)
	}
	if authority.ExactText != message.Content {
		return fmt.Errorf("roleplay user message %d differs from exact persona authority", message.ID)
	}
	message.SpeakerName = authority.PersonaName
	message.Roleplay = projectChannelMessageRoleplayAuthority(authority)
	return nil
}

func projectChannelMessageRoleplayAuthority(
	authority roleplay.UserTurnAuthority,
) *model.ChannelMessageRoleplayAuthority {
	projected := &model.ChannelMessageRoleplayAuthority{
		PersonaKind:      string(authority.PersonaKind),
		CharacterID:      model.RoleplayCharacterID(authority.CharacterID),
		ContributionKind: string(authority.ContributionKind),
		Parts:            make([]model.ChannelMessageRoleplayPart, len(authority.Parts)),
	}
	for index, part := range authority.Parts {
		projected.Parts[index] = model.ChannelMessageRoleplayPart{
			Kind: string(part.Kind), Text: part.Text,
		}
	}
	return projected
}

func presentChannelTurnState(
	message *model.ChannelMessage,
	jobID *int64,
	status, turnError *string,
	updatedAt *time.Time,
	bindingCount *int64,
) error {
	if jobID == nil && status == nil && turnError == nil && updatedAt == nil && bindingCount == nil {
		return nil
	}
	if jobID == nil || status == nil || updatedAt == nil || bindingCount == nil ||
		*jobID < 1 || updatedAt.IsZero() || *bindingCount != 1 {
		return fmt.Errorf("channel user message %d has contradictory job authority", message.ID)
	}
	switch *status {
	case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting,
		model.JobStatusCompleted:
		if turnError != nil && strings.TrimSpace(*turnError) != "" {
			return fmt.Errorf("channel user message %d has an error outside failed status", message.ID)
		}
	case model.JobStatusFailed, model.JobStatusCanceled:
		if turnError == nil || strings.TrimSpace(*turnError) == "" {
			return fmt.Errorf("terminal channel user message %d has no exact error", message.ID)
		}
	default:
		return fmt.Errorf("channel user message %d has unsupported job status %q", message.ID, *status)
	}
	state := &model.ChannelMessageTurnState{
		JobID: *jobID, Status: *status, UpdatedAt: *updatedAt,
	}
	if turnError != nil {
		state.Error = *turnError
	}
	message.Turn = state
	return nil
}
