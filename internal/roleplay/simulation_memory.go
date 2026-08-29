package roleplay

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Store) AppendCharacterMemory(
	ctx context.Context,
	characterID, sourceEventID, content string,
) (CharacterMemory, error) {
	if err := s.validateContext(ctx); err != nil {
		return CharacterMemory{}, err
	}
	if err := validateIdentity(characterID, characterIdentity); err != nil {
		return CharacterMemory{}, err
	}
	if err := validateIdentity(sourceEventID, eventIdentity); err != nil {
		return CharacterMemory{}, err
	}
	if err := validateSimulationText("character memory", content, MaxSimulationTextBytes, true); err != nil {
		return CharacterMemory{}, err
	}
	id, err := newIdentity("rpm_")
	if err != nil {
		return CharacterMemory{}, err
	}
	memory, err := scanCharacterMemory(s.pool.QueryRow(ctx, `
		INSERT INTO roleplay_character_memories (
			id,world_id,character_id,source_event_id,content
		)
		SELECT $3,knowledge.world_id,knowledge.character_id,knowledge.canon_event_id,$4
		FROM roleplay_character_knowledge AS knowledge
		WHERE knowledge.character_id=$1 AND knowledge.canon_event_id=$2
		RETURNING id,source_event_id,content,created_at
	`, characterID, sourceEventID, id, content))
	if err == pgx.ErrNoRows {
		return CharacterMemory{}, fmt.Errorf("%w: memory source is not visible to the character", ErrSimulationIllegal)
	}
	if err != nil {
		return CharacterMemory{}, simulationDefinitionError("character memory", err)
	}
	return memory, nil
}

func scanCharacterMemory(row rowScanner) (CharacterMemory, error) {
	var memory CharacterMemory
	if err := row.Scan(&memory.ID, &memory.SourceEventID, &memory.Content, &memory.CreatedAt); err != nil {
		return CharacterMemory{}, err
	}
	if err := validateIdentity(memory.ID, memoryIdentity); err != nil {
		return CharacterMemory{}, err
	}
	return memory, nil
}
