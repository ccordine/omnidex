package roleplay

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func AppendTurnCanonTx(
	ctx context.Context,
	tx pgx.Tx,
	completionOperationID string,
	responsePosition int,
	channelID string,
	sourceMessageID int64,
	viewpointID string,
	facts []string,
	knowledgeCharacterIDs []string,
) ([]CanonEvent, error) {
	if ctx == nil || tx == nil {
		return nil, fmt.Errorf("roleplay turn canon append requires transaction authority")
	}
	if err := validateCompletionOperationID(completionOperationID); err != nil {
		return nil, err
	}
	if responsePosition < 0 || responsePosition >= MaxSceneParticipants {
		return nil, fmt.Errorf("roleplay turn canon requires a bounded response position")
	}
	if err := validateChannelID(channelID); err != nil {
		return nil, err
	}
	if sourceMessageID < 1 {
		return nil, fmt.Errorf("roleplay turn canon requires an assistant source message")
	}
	if err := validateIdentity(viewpointID, characterIdentity); err != nil {
		return nil, err
	}
	if len(facts) > MaxCanonFactsPerTurn {
		return nil, fmt.Errorf("roleplay turn canon exceeds the %d-fact bound", MaxCanonFactsPerTurn)
	}
	seen := make(map[string]struct{}, len(facts))
	for _, fact := range facts {
		if err := ValidateCanonFact(fact); err != nil {
			return nil, err
		}
		if _, duplicate := seen[fact]; duplicate {
			return nil, fmt.Errorf("roleplay turn canon fact is duplicated")
		}
		seen[fact] = struct{}{}
	}
	if len(knowledgeCharacterIDs) > MaxKnowledgeRecipientsPerTurn {
		return nil, fmt.Errorf(
			"roleplay turn knowledge exceeds the %d-character bound", MaxKnowledgeRecipientsPerTurn,
		)
	}
	if len(facts) == 0 && len(knowledgeCharacterIDs) != 0 {
		return nil, fmt.Errorf("roleplay turn knowledge requires new canon facts")
	}
	if len(facts) != 0 && (len(knowledgeCharacterIDs) != 1 || knowledgeCharacterIDs[0] != viewpointID) {
		return nil, fmt.Errorf("roleplay turn knowledge must bind exactly to the active viewpoint character")
	}
	seenCharacters := make(map[string]struct{}, len(knowledgeCharacterIDs))
	for _, characterID := range knowledgeCharacterIDs {
		if err := validateIdentity(characterID, characterIdentity); err != nil {
			return nil, err
		}
		if _, duplicate := seenCharacters[characterID]; duplicate {
			return nil, fmt.Errorf("roleplay turn knowledge character is duplicated")
		}
		seenCharacters[characterID] = struct{}{}
	}
	var worldID string
	if err := tx.QueryRow(ctx, `
		SELECT world.id
		FROM roleplay_worlds AS world
		JOIN roleplay_characters AS character ON character.world_id=world.id
		WHERE world.channel_id=$1 AND character.id=$2
	`, channelID, viewpointID).Scan(&worldID); err != nil {
		return nil, fmt.Errorf("resolve roleplay turn world and viewpoint: %w", err)
	}
	for _, characterID := range knowledgeCharacterIDs {
		var belongs bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM roleplay_characters WHERE world_id=$1 AND id=$2
			)
		`, worldID, characterID).Scan(&belongs); err != nil {
			return nil, err
		}
		if !belongs {
			return nil, fmt.Errorf("roleplay knowledge recipient does not belong to the turn world")
		}
	}
	factsJSON, err := json.Marshal(facts)
	if err != nil {
		return nil, fmt.Errorf("encode roleplay turn canon facts: %w", err)
	}
	knowledgeJSON, err := json.Marshal(knowledgeCharacterIDs)
	if err != nil {
		return nil, fmt.Errorf("encode roleplay turn knowledge recipients: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO roleplay_turn_completions (
			operation_id,response_position,world_id,viewpoint_character_id,source_message_id,
			facts,knowledge_character_ids
		) VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7::jsonb)
	`, completionOperationID, responsePosition, worldID, viewpointID, sourceMessageID,
		string(factsJSON), string(knowledgeJSON)); err != nil {
		return nil, fmt.Errorf("record roleplay turn completion: %w", err)
	}
	events := make([]CanonEvent, 0, len(facts))
	for _, fact := range facts {
		eventID, err := newIdentity("rpe_")
		if err != nil {
			return nil, err
		}
		event, err := scanCanonEvent(tx.QueryRow(ctx, `
			INSERT INTO roleplay_canon_events (id,world_id,source_message_id,content)
			VALUES ($1,$2,$3,$4)
			RETURNING id,world_id,source_message_id,ordinal,content,authority_namespace,created_at
		`, eventID, worldID, sourceMessageID, fact))
		if err != nil {
			return nil, err
		}
		for _, characterID := range knowledgeCharacterIDs {
			knowledgeID, err := newIdentity("rpk_")
			if err != nil {
				return nil, err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO roleplay_character_knowledge (
					id,world_id,character_id,canon_event_id
				) VALUES ($1,$2,$3,$4)
			`, knowledgeID, worldID, characterID, event.ID); err != nil {
				return nil, err
			}
			memoryID, err := newIdentity("rpm_")
			if err != nil {
				return nil, err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO roleplay_character_memories (
					id,world_id,character_id,source_event_id,content
				) VALUES ($1,$2,$3,$4,$5)
			`, memoryID, worldID, characterID, event.ID, fact); err != nil {
				return nil, fmt.Errorf("append code-derived character memory: %w", err)
			}
		}
		events = append(events, event)
	}
	return events, nil
}

func validateCompletionOperationID(value string) error {
	const prefix = "lifecycle_operation_"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+64 {
		return fmt.Errorf("roleplay turn canon requires an exact lifecycle operation identity")
	}
	for _, character := range []byte(value[len(prefix):]) {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return fmt.Errorf("roleplay turn canon requires an exact lifecycle operation identity")
		}
	}
	return nil
}
