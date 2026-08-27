package roleplay

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

func loadTurnAdvanceTx(
	ctx context.Context,
	tx pgx.Tx,
	operationID, requestHash string,
) (SimulationTurnAdvanceResult, bool, error) {
	type advanceRow struct {
		preparationID, worldID, sceneID        string
		previousCharacterID, activeCharacterID string
		jobID, beforeRevision, afterRevision   int64
		beforeInitiative, afterInitiative      SimulationInitiativeClock
		requestHash                            string
		participantPayload                     []byte
		userTurnPayload                        []byte
		narrativeFingerprint                   string
		payload                                []byte
		createdAt                              time.Time
	}
	var row advanceRow
	err := tx.QueryRow(ctx, `
		SELECT advance.preparation_id,advance.job_id,advance.world_id,advance.scene_id,
		       advance.before_revision,advance.after_revision,
		       advance.previous_character_id,advance.active_character_id,
		       advance.participant_character_ids,
		       advance.before_initiative_round,advance.before_initiative_turn,
		       advance.before_fictional_time_tick,advance.after_initiative_round,
		       advance.after_initiative_turn,advance.after_fictional_time_tick,
		       advance.narrative_fingerprint,advance.request_sha256,advance.result,
		       preparation.result->'user_turn',advance.created_at
		FROM roleplay_simulation_turn_advances AS advance
		JOIN roleplay_simulation_turn_preparations AS preparation
		  ON preparation.operation_id=advance.preparation_id
		WHERE advance.operation_id=$1
	`, operationID).Scan(
		&row.preparationID, &row.jobID, &row.worldID, &row.sceneID,
		&row.beforeRevision, &row.afterRevision, &row.previousCharacterID,
		&row.activeCharacterID, &row.participantPayload,
		&row.beforeInitiative.Round, &row.beforeInitiative.Turn,
		&row.beforeInitiative.FictionalTimeTick, &row.afterInitiative.Round,
		&row.afterInitiative.Turn, &row.afterInitiative.FictionalTimeTick,
		&row.narrativeFingerprint,
		&row.requestHash, &row.payload, &row.userTurnPayload, &row.createdAt,
	)
	if err == pgx.ErrNoRows {
		return SimulationTurnAdvanceResult{}, false, nil
	}
	if err != nil {
		return SimulationTurnAdvanceResult{}, false, err
	}
	if row.requestHash != requestHash {
		return SimulationTurnAdvanceResult{}, false, fmt.Errorf("%w: turn advance identity was reused", ErrSimulationConflict)
	}
	var result SimulationTurnAdvanceResult
	if err := json.Unmarshal(row.payload, &result); err != nil {
		return result, false, fmt.Errorf("decode simulation turn advance: %w", err)
	}
	var participantIDs []string
	if err := json.Unmarshal(row.participantPayload, &participantIDs); err != nil {
		return SimulationTurnAdvanceResult{}, false, fmt.Errorf("decode simulation turn advance participants: %w", err)
	}
	var userTurn UserTurnAuthority
	if err := json.Unmarshal(row.userTurnPayload, &userTurn); err != nil {
		return SimulationTurnAdvanceResult{}, false, fmt.Errorf("decode simulation turn advance user authority: %w", err)
	}
	if err := userTurn.Validate(); err != nil {
		return SimulationTurnAdvanceResult{}, false, fmt.Errorf("persisted simulation turn advance user authority: %w", err)
	}
	if result.OperationID != operationID || result.PreparationID != row.preparationID ||
		result.WorldID != row.worldID || result.SceneID != row.sceneID ||
		result.PreviousCharacterID != row.previousCharacterID || result.ActiveCharacterID != row.activeCharacterID ||
		result.BeforeRevision != row.beforeRevision || result.AfterRevision != row.afterRevision ||
		result.BeforeInitiative != row.beforeInitiative || result.AfterInitiative != row.afterInitiative ||
		!slices.Equal(result.ParticipantCharacterIDs, participantIDs) ||
		result.NarrativeFingerprint != row.narrativeFingerprint ||
		!result.CreatedAt.Equal(row.createdAt) {
		return SimulationTurnAdvanceResult{}, false, fmt.Errorf("persisted simulation turn advance does not match its row authority")
	}
	if err := validateAdvanceReplayResult(result, userTurn); err != nil {
		return SimulationTurnAdvanceResult{}, false, err
	}
	return result, true, nil
}

func validateAdvanceReplayResult(result SimulationTurnAdvanceResult, userTurn UserTurnAuthority) error {
	if validateIdentity(result.OperationID, transitionIdentity) != nil ||
		validateIdentity(result.PreparationID, transitionIdentity) != nil ||
		validateIdentity(result.WorldID, worldIdentity) != nil ||
		validateIdentity(result.SceneID, sceneIdentity) != nil ||
		validateIdentity(result.PreviousCharacterID, characterIdentity) != nil ||
		validateIdentity(result.ActiveCharacterID, characterIdentity) != nil ||
		result.BeforeRevision < 1 || result.AfterRevision != result.BeforeRevision+1 ||
		!validSimulationSHA(result.NarrativeFingerprint) || result.CreatedAt.IsZero() {
		return fmt.Errorf("persisted simulation turn advance is invalid")
	}
	if len(result.ParticipantCharacterIDs) < 1 || len(result.ParticipantCharacterIDs) > MaxSceneParticipants {
		return fmt.Errorf("persisted simulation turn advance participant count is invalid")
	}
	seen := make(map[string]struct{}, len(result.ParticipantCharacterIDs))
	previousFound := false
	activeFound := false
	for _, id := range result.ParticipantCharacterIDs {
		if validateIdentity(id, characterIdentity) != nil {
			return fmt.Errorf("persisted simulation turn advance participant is invalid")
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("persisted simulation turn advance participant is duplicated")
		}
		seen[id] = struct{}{}
		previousFound = previousFound || id == result.PreviousCharacterID
		activeFound = activeFound || id == result.ActiveCharacterID
	}
	if !previousFound || !activeFound {
		return fmt.Errorf("persisted simulation turn advance character is not a participant")
	}
	excludedCharacterID := ""
	if userTurn.IsCharacter() {
		excludedCharacterID = userTurn.CharacterID
	}
	if err := validateSimulationInitiativeAdvance(
		result.BeforeInitiative, result.AfterInitiative,
		result.PreviousCharacterID, result.ActiveCharacterID, result.ParticipantCharacterIDs,
		excludedCharacterID,
	); err != nil {
		return fmt.Errorf("persisted simulation turn advance initiative: %w", err)
	}
	return nil
}
