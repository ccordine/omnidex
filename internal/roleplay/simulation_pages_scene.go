package roleplay

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type SceneParticipantPage struct {
	Items   []SceneParticipantProjection `json:"items"`
	HasMore bool                         `json:"has_more"`
}

func (s *Store) ProjectCurrentScene(ctx context.Context, worldID string) (SceneSheet, error) {
	if err := s.validateContext(ctx); err != nil {
		return SceneSheet{}, err
	}
	if err := validateIdentity(worldID, worldIdentity); err != nil {
		return SceneSheet{}, err
	}
	scene, err := scanSceneSheet(s.pool.QueryRow(ctx, `
		SELECT id,world_id,title,description,revision,current_character_id,created_at,updated_at
		FROM roleplay_current_scenes WHERE world_id=$1
	`, worldID))
	if err == pgx.ErrNoRows {
		return SceneSheet{}, fmt.Errorf("%w: current scene is absent", ErrSimulationNotConfigured)
	}
	return scene, err
}

func (s *Store) ListSceneParticipantsPage(
	ctx context.Context,
	worldID, sceneID string,
	limit, offset int,
) (SceneParticipantPage, error) {
	if err := s.validateContext(ctx); err != nil {
		return SceneParticipantPage{}, err
	}
	if err := validateSimulationPage(worldID, limit, offset); err != nil {
		return SceneParticipantPage{}, err
	}
	if err := validateIdentity(sceneID, sceneIdentity); err != nil {
		return SceneParticipantPage{}, err
	}
	return loadSceneParticipantsPage(ctx, s.pool, worldID, sceneID, limit, offset)
}

func loadSceneParticipantsPage(
	ctx context.Context,
	query simulationQuerier,
	worldID, sceneID string,
	limit, offset int,
) (SceneParticipantPage, error) {
	rows, err := query.Query(ctx, `
		SELECT participant.character_id,character.name,participant.turn_position
		FROM roleplay_scene_participants AS participant
		JOIN roleplay_characters AS character
		  ON character.world_id=participant.world_id AND character.id=participant.character_id
		JOIN roleplay_current_scenes AS scene
		  ON scene.world_id=participant.world_id AND scene.id=participant.scene_id
		WHERE participant.world_id=$1 AND participant.scene_id=$2
		ORDER BY participant.turn_position ASC,participant.character_id ASC
		LIMIT $3 OFFSET $4
	`, worldID, sceneID, limit+1, offset)
	if err != nil {
		return SceneParticipantPage{}, err
	}
	defer rows.Close()
	items := make([]SceneParticipantProjection, 0, limit+1)
	for rows.Next() {
		var item SceneParticipantProjection
		if err := rows.Scan(&item.CharacterID, &item.Name, &item.TurnPosition); err != nil {
			return SceneParticipantPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return SceneParticipantPage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return SceneParticipantPage{Items: items, HasMore: hasMore}, nil
}
