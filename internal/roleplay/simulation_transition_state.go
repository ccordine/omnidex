package roleplay

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type lockedSimulationScene struct {
	Sheet        SceneSheet
	Participants []SceneParticipantProjection
}

func lockSimulationSceneTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID, sceneID string,
) (lockedSimulationScene, error) {
	scene, err := scanSceneSheet(tx.QueryRow(ctx, `
		SELECT id,world_id,title,description,revision,current_character_id,created_at,updated_at
		FROM roleplay_current_scenes
		WHERE world_id=$1 AND id=$2
		FOR UPDATE
	`, worldID, sceneID))
	if err == pgx.ErrNoRows {
		return lockedSimulationScene{}, fmt.Errorf("%w: current scene is absent", ErrSimulationNotConfigured)
	}
	if err != nil {
		return lockedSimulationScene{}, err
	}
	participants, err := loadSceneParticipantsTx(ctx, tx, scene.ID)
	if err != nil {
		return lockedSimulationScene{}, err
	}
	activeFound := false
	for _, participant := range participants {
		if participant.CharacterID == scene.ActiveCharacterID {
			activeFound = true
		}
	}
	if !activeFound {
		return lockedSimulationScene{}, fmt.Errorf("%w: active character is not a scene participant", ErrSimulationNotConfigured)
	}
	return lockedSimulationScene{Sheet: scene, Participants: participants}, nil
}

func loadSceneParticipantsTx(
	ctx context.Context,
	tx pgx.Tx,
	sceneID string,
) ([]SceneParticipantProjection, error) {
	rows, err := tx.Query(ctx, `
		SELECT participant.character_id,character.name,participant.turn_position
		FROM roleplay_scene_participants AS participant
		JOIN roleplay_characters AS character
		  ON character.world_id=participant.world_id AND character.id=participant.character_id
		WHERE participant.scene_id=$1
		ORDER BY participant.turn_position ASC,participant.character_id ASC
		LIMIT $2
	`, sceneID, MaxSceneParticipants+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	participants := make([]SceneParticipantProjection, 0, MaxSceneParticipants)
	for rows.Next() {
		var participant SceneParticipantProjection
		if err := rows.Scan(&participant.CharacterID, &participant.Name, &participant.TurnPosition); err != nil {
			return nil, err
		}
		participants = append(participants, participant)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(participants) < 1 || len(participants) > MaxSceneParticipants {
		return nil, fmt.Errorf("%w: scene participant count is outside its bound", ErrSimulationNotConfigured)
	}
	for position, participant := range participants {
		if participant.TurnPosition != position {
			return nil, fmt.Errorf("%w: scene turn positions are not contiguous", ErrSimulationNotConfigured)
		}
	}
	return participants, nil
}

func nextSceneCharacter(scene lockedSimulationScene) (string, error) {
	return nextSceneCharacterExcept(scene, "")
}

func nextSceneCharacterExcept(scene lockedSimulationScene, excludedCharacterID string) (string, error) {
	for index, participant := range scene.Participants {
		if participant.CharacterID == scene.Sheet.ActiveCharacterID {
			for offset := 1; offset <= len(scene.Participants); offset++ {
				candidate := scene.Participants[(index+offset)%len(scene.Participants)].CharacterID
				if candidate != excludedCharacterID {
					return candidate, nil
				}
			}
			return "", fmt.Errorf(
				"%w: no responding character remains after excluding the user persona",
				ErrSimulationNotConfigured,
			)
		}
	}
	return "", fmt.Errorf("%w: active character is not a scene participant", ErrSimulationNotConfigured)
}

func updateSceneRevisionTx(
	ctx context.Context,
	tx pgx.Tx,
	sceneID string,
	expected int64,
	activeCharacterID string,
) (int64, error) {
	var revision int64
	err := tx.QueryRow(ctx, `
		UPDATE roleplay_current_scenes
		SET revision=revision+1,current_character_id=$3,updated_at=NOW()
		WHERE id=$1 AND revision=$2
		RETURNING revision
	`, sceneID, expected, activeCharacterID).Scan(&revision)
	if err == pgx.ErrNoRows {
		return 0, fmt.Errorf("%w: scene revision changed", ErrSimulationStaleRevision)
	}
	return revision, err
}
