package roleplay

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/jackc/pgx/v5"
)

func loadTurnPreparationTx(
	ctx context.Context,
	tx pgx.Tx,
	request SimulationTurnPreparationRequest,
	userTurn UserTurnAuthority,
) (SimulationTurnAuthority, bool, error) {
	var payload []byte
	err := tx.QueryRow(ctx, `
		SELECT result
		FROM roleplay_simulation_turn_preparations
		WHERE operation_id=$1
	`, request.OperationID).Scan(&payload)
	if err == pgx.ErrNoRows {
		return SimulationTurnAuthority{}, false, nil
	}
	if err != nil {
		return SimulationTurnAuthority{}, false, err
	}
	authority, err := decodeTurnAuthority(payload)
	if err != nil {
		return SimulationTurnAuthority{}, false, err
	}
	if authority.PreparationID != request.OperationID || authority.ChannelID != request.ChannelID ||
		authority.UserMessageID != request.UserMessageID || authority.InputKind != request.InputKind ||
		!authority.UserTurn.Equal(userTurn) {
		return SimulationTurnAuthority{}, false, fmt.Errorf("%w: preparation identity was reused", ErrSimulationConflict)
	}
	return authority, true, nil
}

func persistTurnPreparationTx(ctx context.Context, tx pgx.Tx, authority SimulationTurnAuthority) error {
	payload, err := json.Marshal(authority)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO roleplay_simulation_turn_preparations (
			operation_id,channel_id,user_message_id,world_id,scene_id,
			base_scene_revision,scene_revision,active_character_id,input_kind,explicit_action,
			pending_transition_id,result,created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13)
	`, authority.PreparationID, authority.ChannelID, authority.UserMessageID,
		authority.WorldID, authority.SceneID, authority.BaseSceneRevision,
		authority.SceneRevision, authority.ActiveCharacterID, authority.InputKind, authority.ExplicitAction,
		pendingTransitionID(authority.PendingTransition), string(payload), authority.CreatedAt)
	return simulationDefinitionError("simulation turn preparation", err)
}

func pendingTransitionID(transition *SimulationTransitionResult) any {
	if transition == nil {
		return nil
	}
	return transition.OperationID
}

func decodeTurnAuthority(payload []byte) (SimulationTurnAuthority, error) {
	var authority SimulationTurnAuthority
	if err := exactjson.ValidateObject(payload, authority, "simulation turn"); err != nil {
		return authority, err
	}
	if err := json.Unmarshal(payload, &authority); err != nil {
		return authority, fmt.Errorf("decode simulation turn authority: %w", err)
	}
	if err := authority.Validate(); err != nil {
		return SimulationTurnAuthority{}, fmt.Errorf("persisted simulation turn authority is invalid: %w", err)
	}
	return authority, nil
}
