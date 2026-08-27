package roleplay

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
)

func AppendUserTurnCanonTx(
	ctx context.Context,
	tx pgx.Tx,
	completionOperationID string,
	preparationID string,
	channelID string,
	sourceMessageID int64,
	facts []string,
	knowledgeCharacterIDs []string,
) ([]CanonEvent, error) {
	if ctx == nil || tx == nil {
		return nil, fmt.Errorf("roleplay user canon append requires transaction authority")
	}
	if err := validateCompletionOperationID(completionOperationID); err != nil {
		return nil, err
	}
	if err := validateIdentity(preparationID, transitionIdentity); err != nil {
		return nil, err
	}
	if err := validateChannelID(channelID); err != nil {
		return nil, err
	}
	if sourceMessageID < 1 {
		return nil, fmt.Errorf("roleplay user canon requires an exact user source message")
	}
	if err := validateUserCanonFactsAndRecipients(facts, knowledgeCharacterIDs); err != nil {
		return nil, err
	}

	var worldID, personaKind string
	var actorCharacterID *string
	var participantJSON []byte
	err := tx.QueryRow(ctx, `
		SELECT preparation.world_id,user_turn.persona_kind,
		       user_turn.persona_character_id,preparation.result->'participant_character_ids'
		FROM roleplay_simulation_turn_preparations AS preparation
		JOIN roleplay_user_turns AS user_turn
		  ON user_turn.user_message_id=preparation.user_message_id
		 AND user_turn.channel_id=preparation.channel_id
		 AND user_turn.world_id=preparation.world_id
		JOIN ai_channel_messages AS message
		  ON message.id=user_turn.user_message_id
		 AND message.channel_id=user_turn.channel_id
		WHERE preparation.operation_id=$1 AND preparation.channel_id=$2
		  AND preparation.user_message_id=$3 AND message.role='user'
		  AND message.content=user_turn.exact_text
		  AND user_turn.contribution_kind<>'command'
	`, preparationID, channelID, sourceMessageID).Scan(
		&worldID, &personaKind, &actorCharacterID, &participantJSON,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("roleplay user canon source differs from frozen turn authority")
	}
	if err != nil {
		return nil, err
	}
	var participantIDs []string
	if err := json.Unmarshal(participantJSON, &participantIDs); err != nil {
		return nil, fmt.Errorf("decode roleplay user canon participant authority: %w", err)
	}
	expectedRecipients := []string{}
	if len(facts) != 0 {
		switch UserPersonaKind(personaKind) {
		case UserPersonaCharacter:
			if actorCharacterID == nil {
				return nil, fmt.Errorf("character user canon is missing its exact actor")
			}
			expectedRecipients = []string{*actorCharacterID}
		case UserPersonaNarrator:
			expectedRecipients = participantIDs
		default:
			return nil, fmt.Errorf("roleplay user canon has unsupported persona kind %q", personaKind)
		}
	}
	if !slices.Equal(knowledgeCharacterIDs, expectedRecipients) {
		return nil, fmt.Errorf("roleplay user canon recipients differ from frozen observer authority")
	}
	factsJSON, err := json.Marshal(facts)
	if err != nil {
		return nil, fmt.Errorf("encode roleplay user canon facts: %w", err)
	}
	knowledgeJSON, err := json.Marshal(knowledgeCharacterIDs)
	if err != nil {
		return nil, fmt.Errorf("encode roleplay user canon recipients: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO roleplay_user_canon_completions (
			operation_id,preparation_id,world_id,source_message_id,persona_kind,
			actor_character_id,facts,knowledge_character_ids
		) VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb)
	`, completionOperationID, preparationID, worldID, sourceMessageID, personaKind,
		actorCharacterID, string(factsJSON), string(knowledgeJSON)); err != nil {
		return nil, fmt.Errorf("record roleplay user canon completion: %w", err)
	}

	events := make([]CanonEvent, 0, len(facts))
	for _, fact := range facts {
		event, err := appendUserCanonEventTx(
			ctx, tx, worldID, sourceMessageID, fact, knowledgeCharacterIDs,
		)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func validateUserCanonFactsAndRecipients(facts, knowledgeCharacterIDs []string) error {
	if len(facts) > MaxCanonFactsPerTurn {
		return fmt.Errorf("roleplay user canon exceeds the %d-fact bound", MaxCanonFactsPerTurn)
	}
	seenFacts := make(map[string]struct{}, len(facts))
	for _, fact := range facts {
		if err := ValidateCanonFact(fact); err != nil {
			return err
		}
		if _, duplicate := seenFacts[fact]; duplicate {
			return fmt.Errorf("roleplay user canon fact is duplicated")
		}
		seenFacts[fact] = struct{}{}
	}
	if len(knowledgeCharacterIDs) > MaxKnowledgeRecipientsPerTurn {
		return fmt.Errorf(
			"roleplay user canon exceeds the %d-recipient bound",
			MaxKnowledgeRecipientsPerTurn,
		)
	}
	if len(facts) == 0 && len(knowledgeCharacterIDs) != 0 {
		return fmt.Errorf("roleplay user canon knowledge requires new facts")
	}
	if len(facts) != 0 && len(knowledgeCharacterIDs) == 0 {
		return fmt.Errorf("roleplay user canon facts require exact knowledge recipients")
	}
	seenRecipients := make(map[string]struct{}, len(knowledgeCharacterIDs))
	for _, characterID := range knowledgeCharacterIDs {
		if err := validateIdentity(characterID, characterIdentity); err != nil {
			return err
		}
		if _, duplicate := seenRecipients[characterID]; duplicate {
			return fmt.Errorf("roleplay user canon recipient is duplicated")
		}
		seenRecipients[characterID] = struct{}{}
	}
	return nil
}
