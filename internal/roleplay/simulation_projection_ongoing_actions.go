package roleplay

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func loadCurrentOngoingActionsTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID, sceneID string,
) ([]NarrativeOngoingAction, []string, []string, error) {
	rows, err := tx.Query(ctx, `
		SELECT latest.id,character.id,character.name,latest.action_text
		FROM roleplay_scene_participants AS participant
		JOIN roleplay_characters AS character
		  ON character.world_id=participant.world_id
		 AND character.id=participant.character_id
		JOIN LATERAL (
			SELECT state.id,state.action_text
			FROM roleplay_ongoing_action_states AS state
			WHERE state.world_id=$1 AND state.character_id=participant.character_id
			ORDER BY state.ordinal DESC,state.id DESC
			LIMIT 1
		) AS latest ON latest.action_text IS NOT NULL
		WHERE participant.world_id=$1 AND participant.scene_id=$2
		ORDER BY participant.turn_position,participant.character_id
		LIMIT $3
	`, worldID, sceneID, MaxSceneParticipants+1)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	actions := make([]NarrativeOngoingAction, 0, MaxSceneParticipants)
	ids := make([]string, 0, MaxSceneParticipants)
	characterIDs := make([]string, 0, MaxSceneParticipants)
	for rows.Next() {
		var id, characterID string
		var action NarrativeOngoingAction
		if err := rows.Scan(&id, &characterID, &action.CharacterName, &action.Action); err != nil {
			return nil, nil, nil, err
		}
		if validateIdentity(id, ongoingActionStateIdentity) != nil {
			return nil, nil, nil, fmt.Errorf("projected ongoing-action state identity is invalid")
		}
		if validateIdentity(characterID, characterIdentity) != nil {
			return nil, nil, nil, fmt.Errorf("projected ongoing-action character identity is invalid")
		}
		if err := validateName(action.CharacterName, "ongoing-action character name"); err != nil {
			return nil, nil, nil, err
		}
		if err := ValidateOngoingActionText(action.Action); err != nil {
			return nil, nil, nil, err
		}
		ids = append(ids, id)
		characterIDs = append(characterIDs, characterID)
		actions = append(actions, action)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}
	if len(actions) > MaxSceneParticipants {
		return nil, nil, nil, fmt.Errorf("%w: ongoing-action projection exceeds its participant bound", ErrSimulationNotConfigured)
	}
	if len(actions) == 0 {
		return nil, nil, nil, nil
	}
	return actions, ids, characterIDs, nil
}
