package roleplay

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func appendUserCanonEventTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID string,
	sourceMessageID int64,
	fact string,
	knowledgeCharacterIDs []string,
) (CanonEvent, error) {
	eventID, err := newIdentity("rpe_")
	if err != nil {
		return CanonEvent{}, err
	}
	event, err := scanCanonEvent(tx.QueryRow(ctx, `
		INSERT INTO roleplay_canon_events (id,world_id,source_message_id,content)
		VALUES ($1,$2,$3,$4)
		RETURNING id,world_id,source_message_id,ordinal,content,authority_namespace,created_at
	`, eventID, worldID, sourceMessageID, fact))
	if err != nil {
		return CanonEvent{}, err
	}
	for _, characterID := range knowledgeCharacterIDs {
		knowledgeID, err := newIdentity("rpk_")
		if err != nil {
			return CanonEvent{}, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO roleplay_character_knowledge (
				id,world_id,character_id,canon_event_id
			) VALUES ($1,$2,$3,$4)
		`, knowledgeID, worldID, characterID, event.ID); err != nil {
			return CanonEvent{}, err
		}
		memoryID, err := newIdentity("rpm_")
		if err != nil {
			return CanonEvent{}, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO roleplay_character_memories (
				id,world_id,character_id,source_event_id,content
			) VALUES ($1,$2,$3,$4,$5)
		`, memoryID, worldID, characterID, event.ID, fact); err != nil {
			return CanonEvent{}, fmt.Errorf("append code-derived user canon memory: %w", err)
		}
	}
	return event, nil
}
