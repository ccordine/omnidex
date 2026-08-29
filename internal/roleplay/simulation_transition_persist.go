package roleplay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func simulationRequestHash(schema string, value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(append([]byte(schema+"\x00"), payload...))
	return hex.EncodeToString(digest[:]), nil
}

func loadSimulationTransitionTx(
	ctx context.Context,
	tx pgx.Tx,
	operationID, requestHash string,
) (SimulationTransitionResult, bool, error) {
	var storedHash string
	var payload []byte
	err := tx.QueryRow(ctx, `
		SELECT request_sha256,result
		FROM roleplay_simulation_transitions
		WHERE operation_id=$1
	`, operationID).Scan(&storedHash, &payload)
	if err == pgx.ErrNoRows {
		return SimulationTransitionResult{}, false, nil
	}
	if err != nil {
		return SimulationTransitionResult{}, false, err
	}
	if storedHash != requestHash {
		return SimulationTransitionResult{}, false, fmt.Errorf("%w: transition identity was reused", ErrSimulationConflict)
	}
	result, err := decodeSimulationTransitionResult(payload)
	return result, true, err
}

func persistSimulationTransitionTx(
	ctx context.Context,
	tx pgx.Tx,
	requestHash, exactAction string,
	observerCharacterIDs []string,
	result SimulationTransitionResult,
) error {
	if err := validateSimulationTransitionObservers(
		observerCharacterIDs, result.ActorCharacterID,
	); err != nil {
		return err
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	observerPayload, err := json.Marshal(observerCharacterIDs)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO roleplay_simulation_transitions (
			operation_id,world_id,scene_id,actor_character_id,
			before_revision,after_revision,exact_action,action_kind,command_key,
			request_sha256,result,observer_character_ids,created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12::jsonb,$13)
	`, result.OperationID, result.WorldID, result.SceneID, result.ActorCharacterID,
		result.BeforeRevision, result.AfterRevision, exactAction,
		result.Action.Kind, result.Action.CommandKey, requestHash, string(payload),
		string(observerPayload), result.CreatedAt)
	return simulationDefinitionError("simulation transition", err)
}

func validateSimulationTransitionObservers(observerCharacterIDs []string, actorCharacterID string) error {
	if len(observerCharacterIDs) < 1 || len(observerCharacterIDs) > MaxSceneParticipants {
		return fmt.Errorf("simulation transition observer count is outside its bound")
	}
	seen := make(map[string]struct{}, len(observerCharacterIDs))
	actorFound := false
	for _, characterID := range observerCharacterIDs {
		if err := validateIdentity(characterID, characterIdentity); err != nil {
			return fmt.Errorf("simulation transition observer is invalid")
		}
		if _, duplicate := seen[characterID]; duplicate {
			return fmt.Errorf("simulation transition observer is duplicated")
		}
		seen[characterID] = struct{}{}
		actorFound = actorFound || characterID == actorCharacterID
	}
	if !actorFound {
		return fmt.Errorf("simulation transition actor is not an observer")
	}
	return nil
}

func decodeSimulationTransitionResult(payload []byte) (SimulationTransitionResult, error) {
	var result SimulationTransitionResult
	if err := json.Unmarshal(payload, &result); err != nil {
		return result, fmt.Errorf("decode simulation transition: %w", err)
	}
	if result.Schema != SimulationTransitionSchemaV1 ||
		validateIdentity(result.OperationID, transitionIdentity) != nil ||
		validateIdentity(result.WorldID, worldIdentity) != nil ||
		validateIdentity(result.SceneID, sceneIdentity) != nil ||
		validateIdentity(result.ActorCharacterID, characterIdentity) != nil ||
		result.BeforeRevision < 1 || result.AfterRevision != result.BeforeRevision+1 ||
		len(result.Effects) < 1 || len(result.Effects) > MaxTransitionEffects ||
		len(result.NarrativeEvents) < 1 || len(result.NarrativeEvents) > 2 || result.CreatedAt.IsZero() {
		return SimulationTransitionResult{}, fmt.Errorf("persisted simulation transition is invalid")
	}
	for _, event := range result.NarrativeEvents {
		if err := validateSimulationText(
			"transition narrative event", event, MaxSimulationTextBytes+528, true,
		); err != nil {
			return SimulationTransitionResult{}, fmt.Errorf("persisted simulation transition narrative event is invalid")
		}
	}
	for index, effect := range result.Effects {
		if effect.Sequence != index+1 || effect.Kind == "" {
			return SimulationTransitionResult{}, fmt.Errorf("persisted simulation transition effect order is invalid")
		}
	}
	return result, nil
}
