package roleplay

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

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
		SELECT memory.id,memory.source_event_id,memory.content,memory.created_at
		FROM roleplay_characters AS viewpoint
		JOIN roleplay_characters AS placement
		  ON placement.library_character_id=viewpoint.library_character_id
		JOIN roleplay_character_memories AS memory
		  ON memory.character_id=placement.id AND memory.world_id=placement.world_id
		WHERE viewpoint.world_id=$1 AND viewpoint.id=$2
		ORDER BY memory.ordinal DESC,memory.id DESC LIMIT $3
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
	worldID, sceneID, viewpointID string,
) ([]string, []string, error) {
	rows, err := tx.Query(ctx, `
		SELECT transition.operation_id,transition.result
		FROM roleplay_simulation_transitions AS transition
		WHERE transition.world_id=$1 AND transition.scene_id=$2
		  AND transition.observer_character_ids ? $3
		ORDER BY transition.ordinal DESC,transition.operation_id DESC LIMIT $4
	`, worldID, sceneID, viewpointID, MaxSimulationHistory)
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
	if err := content.Scene.Initiative.Validate(); err != nil {
		return "", err
	}
	if err := validateNarrativeOngoingActions(content, authority); err != nil {
		return "", err
	}
	base, err := simulationNarrativeBaseDigest(content, authority)
	if err != nil {
		return "", err
	}
	clock := content.Scene.Initiative
	return simulationSHA([]byte(
		base + ":" + strconv.FormatInt(clock.Round, 10) + ":" +
			strconv.FormatInt(clock.Turn, 10) + ":" +
			strconv.FormatInt(clock.FictionalTimeTick, 10),
	)), nil
}

func simulationNarrativeBaseDigest(
	content NarrativeSimulationProjection,
	authority SimulationNarrativeAuthority,
) (string, error) {
	authority.Fingerprint = ""
	type fingerprintScene struct {
		Title               string `json:"title"`
		Description         string `json:"description"`
		ActiveCharacterName string `json:"active_character_name"`
	}
	type fingerprintContent struct {
		Schema         string                   `json:"schema"`
		Scene          fingerprintScene         `json:"scene"`
		Participants   []string                 `json:"participants"`
		Viewpoint      NarrativePersona         `json:"viewpoint"`
		OngoingActions []NarrativeOngoingAction `json:"ongoing_actions,omitempty"`
		Meters         []NarrativeMeter         `json:"meters"`
		Inventory      []NarrativeInventoryItem `json:"inventory"`
		VisibleFacts   []string                 `json:"visible_facts"`
		Memories       []string                 `json:"memories"`
		RecentEvents   []string                 `json:"recent_events"`
	}
	payload, err := json.Marshal(struct {
		Content   fingerprintContent           `json:"content"`
		Authority SimulationNarrativeAuthority `json:"authority"`
	}{Content: fingerprintContent{
		Schema: content.Schema,
		Scene: fingerprintScene{
			Title: content.Scene.Title, Description: content.Scene.Description,
			ActiveCharacterName: content.Scene.ActiveCharacterName,
		},
		Participants: content.Participants, Viewpoint: content.Viewpoint,
		OngoingActions: content.OngoingActions,
		Meters:         content.Meters, Inventory: content.Inventory,
		VisibleFacts: content.VisibleFacts, Memories: content.Memories,
		RecentEvents: content.RecentEvents,
	}, Authority: authority})
	if err != nil {
		return "", err
	}
	return simulationSHA(payload), nil
}
