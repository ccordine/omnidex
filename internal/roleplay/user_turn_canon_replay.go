package roleplay

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
)

func RequireUserTurnCanonReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	completionOperationID string,
	preparationID string,
	channelID string,
	sourceMessageID int64,
	facts []string,
	knowledgeCharacterIDs []string,
) error {
	if ctx == nil || tx == nil {
		return fmt.Errorf("roleplay user canon replay requires transaction authority")
	}
	if err := validateCompletionOperationID(completionOperationID); err != nil {
		return err
	}
	if err := validateUserCanonFactsAndRecipients(facts, knowledgeCharacterIDs); err != nil {
		return err
	}
	var worldID string
	var storedFactsJSON, storedKnowledgeJSON []byte
	err := tx.QueryRow(ctx, `
		SELECT completion.world_id,completion.facts,completion.knowledge_character_ids
		FROM roleplay_user_canon_completions AS completion
		JOIN roleplay_simulation_turn_preparations AS preparation
		  ON preparation.operation_id=completion.preparation_id
		JOIN ai_channel_messages AS message
		  ON message.id=completion.source_message_id
		 AND message.channel_id=preparation.channel_id
		WHERE completion.operation_id=$1 AND completion.preparation_id=$2
		  AND preparation.channel_id=$3 AND completion.source_message_id=$4
		  AND preparation.world_id=completion.world_id
		  AND message.role='user' AND preparation.user_message_id=message.id
	`, completionOperationID, preparationID, channelID, sourceMessageID).Scan(
		&worldID, &storedFactsJSON, &storedKnowledgeJSON,
	)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("roleplay user canon completion receipt is absent")
	}
	if err != nil {
		return err
	}
	var storedFacts, storedKnowledge []string
	if json.Unmarshal(storedFactsJSON, &storedFacts) != nil ||
		json.Unmarshal(storedKnowledgeJSON, &storedKnowledge) != nil ||
		!slices.Equal(storedFacts, facts) ||
		!slices.Equal(storedKnowledge, knowledgeCharacterIDs) {
		return fmt.Errorf("roleplay user canon completion receipt differs from exact command")
	}
	var eventFacts []string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(array_agg(content ORDER BY ordinal),ARRAY[]::text[])
		FROM roleplay_canon_events
		WHERE world_id=$1 AND source_message_id=$2
	`, worldID, sourceMessageID).Scan(&eventFacts); err != nil {
		return err
	}
	if !slices.Equal(eventFacts, facts) {
		return fmt.Errorf("roleplay user canon events differ from exact completion receipt")
	}
	var knowledgeCount, memoryCount int
	if err := tx.QueryRow(ctx, `
		SELECT
		  (SELECT COUNT(*)
		   FROM roleplay_canon_events AS event
		   JOIN roleplay_character_knowledge AS knowledge
		     ON knowledge.world_id=event.world_id AND knowledge.canon_event_id=event.id
		   WHERE event.world_id=$1 AND event.source_message_id=$2),
		  (SELECT COUNT(*)
		   FROM roleplay_canon_events AS event
		   JOIN roleplay_character_memories AS memory
		     ON memory.world_id=event.world_id AND memory.source_event_id=event.id
		   WHERE event.world_id=$1 AND event.source_message_id=$2)
	`, worldID, sourceMessageID).Scan(&knowledgeCount, &memoryCount); err != nil {
		return err
	}
	expectedCount := len(facts) * len(knowledgeCharacterIDs)
	if knowledgeCount != expectedCount || memoryCount != expectedCount {
		return fmt.Errorf("roleplay user canon visibility cardinality differs from exact recipients")
	}
	for _, characterID := range knowledgeCharacterIDs {
		if err := requireUserCanonRecipientReplayTx(
			ctx, tx, worldID, sourceMessageID, characterID, facts,
		); err != nil {
			return err
		}
	}
	return nil
}

func requireUserCanonRecipientReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID string,
	sourceMessageID int64,
	characterID string,
	facts []string,
) error {
	var knowledgeFacts, memoryFacts []string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(array_agg(event.content ORDER BY event.ordinal),ARRAY[]::text[])
		FROM roleplay_canon_events AS event
		JOIN roleplay_character_knowledge AS knowledge
		  ON knowledge.world_id=event.world_id AND knowledge.canon_event_id=event.id
		WHERE event.world_id=$1 AND event.source_message_id=$2
		  AND knowledge.character_id=$3
	`, worldID, sourceMessageID, characterID).Scan(&knowledgeFacts); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(array_agg(memory.content ORDER BY event.ordinal),ARRAY[]::text[])
		FROM roleplay_canon_events AS event
		JOIN roleplay_character_memories AS memory
		  ON memory.world_id=event.world_id AND memory.source_event_id=event.id
		WHERE event.world_id=$1 AND event.source_message_id=$2
		  AND memory.character_id=$3
	`, worldID, sourceMessageID, characterID).Scan(&memoryFacts); err != nil {
		return err
	}
	if !slices.Equal(knowledgeFacts, facts) || !slices.Equal(memoryFacts, facts) {
		return fmt.Errorf("roleplay user canon recipient projection differs from exact facts")
	}
	return nil
}
