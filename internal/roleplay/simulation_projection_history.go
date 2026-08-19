package roleplay

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func loadVisibleCanonTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID, characterID string,
) ([]ContextFact, error) {
	rows, err := tx.Query(ctx, `
		SELECT event.id,event.content
		FROM roleplay_character_knowledge AS knowledge
		JOIN roleplay_canon_events AS event
		  ON event.world_id=knowledge.world_id AND event.id=knowledge.canon_event_id
		WHERE knowledge.world_id=$1 AND knowledge.character_id=$2
		ORDER BY event.ordinal DESC,event.id DESC LIMIT $3
	`, worldID, characterID, MaxProjectionEvents)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	facts := make([]ContextFact, 0, MaxProjectionEvents)
	for rows.Next() {
		var fact ContextFact
		if err := rows.Scan(&fact.EventID, &fact.Content); err != nil {
			return nil, err
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return reverseSimulationSlice(facts), nil
}

func loadCharacterMemoriesTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID, characterID string,
) ([]CharacterMemory, error) {
	rows, err := tx.Query(ctx, `
		SELECT id,source_event_id,content,created_at
		FROM roleplay_character_memories
		WHERE world_id=$1 AND character_id=$2
		ORDER BY ordinal DESC,id DESC LIMIT $3
	`, worldID, characterID, MaxProjectionEvents)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	memories := make([]CharacterMemory, 0, MaxProjectionEvents)
	for rows.Next() {
		memory, scanErr := scanCharacterMemory(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		memories = append(memories, memory)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return reverseSimulationSlice(memories), nil
}

func loadNarrativeEventsTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID, sceneID string,
) ([]string, []string, error) {
	rows, err := tx.Query(ctx, `
		SELECT operation_id,result
		FROM roleplay_simulation_transitions
		WHERE world_id=$1 AND scene_id=$2
		ORDER BY ordinal DESC,operation_id DESC LIMIT $3
	`, worldID, sceneID, MaxSimulationHistory)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	type persistedEvents struct {
		id     string
		events []string
	}
	records := make([]persistedEvents, 0, MaxSimulationHistory)
	for rows.Next() {
		var id string
		var payload []byte
		if err := rows.Scan(&id, &payload); err != nil {
			return nil, nil, err
		}
		result, err := decodeSimulationTransitionResult(payload)
		if err != nil {
			return nil, nil, err
		}
		if result.OperationID != id {
			return nil, nil, fmt.Errorf("persisted simulation transition identity does not match its result")
		}
		records = append(records, persistedEvents{id: id, events: result.NarrativeEvents})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	reverseSimulationSlice(records)
	type narrativeEventRow struct{ transitionID, text string }
	projected := make([]narrativeEventRow, 0, MaxSimulationHistory*2)
	for _, record := range records {
		for _, event := range record.events {
			projected = append(projected, narrativeEventRow{transitionID: record.id, text: event})
		}
	}
	if len(projected) > MaxSimulationHistory {
		projected = projected[len(projected)-MaxSimulationHistory:]
	}
	events := make([]string, len(projected))
	ids := make([]string, 0, len(records))
	lastID := ""
	for index, event := range projected {
		events[index] = event.text
		if event.transitionID != lastID {
			ids = append(ids, event.transitionID)
			lastID = event.transitionID
		}
	}
	return events, ids, nil
}

func simulationNarrativeDigest(content NarrativeSimulationProjection, authority SimulationNarrativeAuthority) (string, error) {
	authority.Fingerprint = ""
	payload, err := json.Marshal(struct {
		Content   NarrativeSimulationProjection `json:"content"`
		Authority SimulationNarrativeAuthority  `json:"authority"`
	}{Content: content, Authority: authority})
	if err != nil {
		return "", err
	}
	return simulationSHA(payload), nil
}
