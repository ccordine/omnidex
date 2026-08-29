package roleplay

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// PersistUserTurnAuthorityTx snapshots who the user controls and what kind of
// contribution the exact message contains. It must run in the same transaction
// that inserts that message and prepares the simulation turn.
func PersistUserTurnAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	channelID string,
	userMessageID int64,
	exactText string,
	request UserTurnRequest,
) (UserTurnAuthority, error) {
	if ctx == nil || tx == nil {
		return UserTurnAuthority{}, fmt.Errorf("roleplay user turn requires transaction authority")
	}
	if err := validateChannelID(channelID); err != nil {
		return UserTurnAuthority{}, err
	}
	if userMessageID < 1 {
		return UserTurnAuthority{}, fmt.Errorf("roleplay user turn requires a positive message identity")
	}
	if err := request.ValidateForExactText(exactText); err != nil {
		return UserTurnAuthority{}, err
	}
	authority := UserTurnAuthority{
		PersonaKind: request.PersonaKind, CharacterID: request.CharacterID,
		ContributionKind: request.ContributionKind,
		Parts:            append([]UserTurnPart{}, request.Parts...),
		ExactText:        exactText,
	}
	var worldID string
	switch request.PersonaKind {
	case UserPersonaCharacter:
		err := tx.QueryRow(ctx, `
			SELECT world.id,character.name,COALESCE(profile.summary,'')
			FROM ai_channels AS channel
			JOIN roleplay_worlds AS world ON world.channel_id=channel.id
			JOIN roleplay_current_scenes AS scene ON scene.world_id=world.id
			JOIN roleplay_scene_participants AS participant
			  ON participant.world_id=scene.world_id AND participant.scene_id=scene.id
			JOIN roleplay_characters AS character
			  ON character.world_id=participant.world_id
			 AND character.id=participant.character_id
			JOIN roleplay_character_profiles AS profile
			  ON profile.library_character_id=character.library_character_id
			JOIN ai_channel_messages AS message
			  ON message.channel_id=channel.id AND message.id=$2
			WHERE channel.id=$1 AND channel.mode='roleplay' AND message.role='user'
			  AND message.content=$3 AND character.id=$4
			FOR UPDATE OF scene
		`, channelID, userMessageID, exactText, request.CharacterID).Scan(
			&worldID, &authority.PersonaName, &authority.PersonaSummary,
		)
		if err == pgx.ErrNoRows {
			return UserTurnAuthority{}, fmt.Errorf(
				"%w: selected user persona must be a current scene participant",
				ErrSimulationIllegal,
			)
		}
		if err != nil {
			return UserTurnAuthority{}, err
		}
	case UserPersonaNarrator:
		authority.PersonaName = NarratorPersonaName
		err := tx.QueryRow(ctx, `
			SELECT world.id
			FROM ai_channels AS channel
			JOIN roleplay_worlds AS world ON world.channel_id=channel.id
			JOIN roleplay_current_scenes AS scene ON scene.world_id=world.id
			JOIN ai_channel_messages AS message
			  ON message.channel_id=channel.id AND message.id=$2
			WHERE channel.id=$1 AND channel.mode='roleplay'
			  AND message.role='user' AND message.content=$3
		`, channelID, userMessageID, exactText).Scan(&worldID)
		if err == pgx.ErrNoRows {
			return UserTurnAuthority{}, fmt.Errorf("%w: channel message has no current roleplay scene", ErrSimulationNotConfigured)
		}
		if err != nil {
			return UserTurnAuthority{}, err
		}
	default:
		return UserTurnAuthority{}, fmt.Errorf("new roleplay user turn has unsupported persona kind %q", request.PersonaKind)
	}
	if err := authority.Validate(); err != nil {
		return UserTurnAuthority{}, err
	}
	partsJSON, err := json.Marshal(authority.Parts)
	if err != nil {
		return UserTurnAuthority{}, fmt.Errorf("encode roleplay user turn parts: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO roleplay_user_turns (
			user_message_id,channel_id,world_id,persona_kind,persona_character_id,
			persona_name,persona_summary,contribution_kind,parts,exact_text
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10)
	`, userMessageID, channelID, worldID, authority.PersonaKind,
		nullableUserPersonaCharacter(authority.CharacterID), authority.PersonaName,
		authority.PersonaSummary, authority.ContributionKind, string(partsJSON), authority.ExactText)
	if err != nil {
		return UserTurnAuthority{}, simulationDefinitionError("roleplay user turn", err)
	}
	return authority, nil
}

func loadUserTurnAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	channelID string,
	userMessageID int64,
) (UserTurnAuthority, string, error) {
	var authority UserTurnAuthority
	var worldID string
	var payload []byte
	err := tx.QueryRow(ctx, `
		SELECT turn.world_id,turn.authority
		FROM roleplay_user_turns AS turn
		JOIN ai_channel_messages AS message
		  ON message.id=turn.user_message_id AND message.channel_id=turn.channel_id
		WHERE turn.channel_id=$1 AND turn.user_message_id=$2
		  AND message.role='user' AND message.content=turn.exact_text
	`, channelID, userMessageID).Scan(&worldID, &payload)
	if err == pgx.ErrNoRows {
		return UserTurnAuthority{}, "", fmt.Errorf("%w: roleplay user turn authority is absent", ErrSimulationIllegal)
	}
	if err != nil {
		return UserTurnAuthority{}, "", err
	}
	if err := json.Unmarshal(payload, &authority); err != nil {
		return UserTurnAuthority{}, "", fmt.Errorf("decode roleplay user turn authority: %w", err)
	}
	if err := authority.Validate(); err != nil {
		return UserTurnAuthority{}, "", fmt.Errorf("persisted roleplay user turn is invalid: %w", err)
	}
	return authority, worldID, nil
}

func nullableUserPersonaCharacter(characterID string) any {
	if characterID == "" {
		return nil
	}
	return characterID
}
