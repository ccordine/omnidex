package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
)

func loadConversationRoleplayUserTurn(
	ctx context.Context,
	tx pgx.Tx,
	channelID string,
	messageID int64,
	exactText string,
) (roleplay.UserTurnAuthority, error) {
	var authority roleplay.UserTurnAuthority
	var payload []byte
	err := tx.QueryRow(ctx, `
		SELECT turn.authority
		FROM roleplay_user_turns AS turn
		JOIN roleplay_worlds AS world
		  ON world.id=turn.world_id AND world.channel_id=turn.channel_id
		WHERE turn.channel_id=$1 AND turn.user_message_id=$2 AND turn.exact_text=$3
	`, channelID, messageID, exactText).Scan(&payload)
	if err == pgx.ErrNoRows {
		return roleplay.UserTurnAuthority{}, fmt.Errorf(
			"roleplay conversation message %d has no exact user-turn authority", messageID,
		)
	}
	if err != nil {
		return roleplay.UserTurnAuthority{}, err
	}
	if err := json.Unmarshal(payload, &authority); err != nil {
		return roleplay.UserTurnAuthority{}, fmt.Errorf(
			"decode roleplay conversation message %d user-turn authority: %w", messageID, err,
		)
	}
	if err := authority.Validate(); err != nil {
		return roleplay.UserTurnAuthority{}, fmt.Errorf(
			"roleplay conversation message %d user-turn authority: %w", messageID, err,
		)
	}
	return authority, nil
}

func loadConversationAssistantSpeaker(
	ctx context.Context,
	tx pgx.Tx,
	channelID string,
	mode model.ChannelMode,
	messageID int64,
) (string, error) {
	if mode != model.ChannelModeRoleplay {
		return "", nil
	}
	var fictionalName, researchName *string
	err := tx.QueryRow(ctx, `
		SELECT fictional_character.name,research_character.name
		FROM ai_channel_messages AS assistant
		LEFT JOIN roleplay_turn_completions AS fictional
		  ON fictional.source_message_id=assistant.id
		LEFT JOIN roleplay_characters AS fictional_character
		  ON fictional_character.world_id=fictional.world_id
		 AND fictional_character.id=fictional.viewpoint_character_id
		LEFT JOIN roleplay_research_completions AS research
		  ON research.source_message_id=assistant.id
		LEFT JOIN roleplay_research_turns AS research_turn
		  ON research_turn.preparation_id=research.preparation_id
		LEFT JOIN roleplay_characters AS research_character
		  ON research_character.world_id=research_turn.world_id
		 AND research_character.id=research_turn.character_id
		WHERE assistant.id=$1 AND assistant.channel_id=$2 AND assistant.role='assistant'
	`, messageID, channelID).Scan(&fictionalName, &researchName)
	if err != nil {
		return "", err
	}
	if (fictionalName == nil) == (researchName == nil) {
		return "", fmt.Errorf(
			"roleplay assistant message %d has contradictory speaker authority", messageID,
		)
	}
	if fictionalName != nil {
		return *fictionalName, nil
	}
	return *researchName, nil
}
