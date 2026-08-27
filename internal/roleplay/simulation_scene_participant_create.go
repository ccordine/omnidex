package roleplay

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// CreateSceneParticipant atomically creates one world character and appends it
// to the current scene's authoritative initiative order.
func (s *Store) CreateSceneParticipant(
	ctx context.Context,
	worldID, name string,
) (Character, error) {
	if err := s.validateContext(ctx); err != nil {
		return Character{}, err
	}
	if err := validateIdentity(worldID, worldIdentity); err != nil {
		return Character{}, err
	}
	if err := validateName(name, "roleplay character name"); err != nil {
		return Character{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Character{}, err
	}
	defer tx.Rollback(context.Background())
	var sceneID string
	err = tx.QueryRow(ctx, `
		SELECT id FROM roleplay_current_scenes WHERE world_id=$1
	`, worldID).Scan(&sceneID)
	if err == pgx.ErrNoRows {
		return Character{}, fmt.Errorf("%w: current scene is absent", ErrSimulationNotConfigured)
	}
	if err != nil {
		return Character{}, err
	}
	locked, err := lockSimulationSceneTx(ctx, tx, worldID, sceneID)
	if err != nil {
		return Character{}, err
	}
	if len(locked.Participants) == MaxSceneParticipants {
		return Character{}, fmt.Errorf("%w: scene participant bound reached", ErrSimulationConflict)
	}
	seenNames := make(map[string]struct{}, len(locked.Participants)+1)
	for _, participant := range locked.Participants {
		seenNames[participant.Name] = struct{}{}
	}
	if err := recordDistinctSceneParticipantName(seenNames, name); err != nil {
		return Character{}, err
	}
	characterID, err := newIdentity("rpc_")
	if err != nil {
		return Character{}, err
	}
	libraryID, err := NewLibraryCharacterIdentity()
	if err != nil {
		return Character{}, err
	}
	character, err := createCharacterTx(
		ctx, tx, characterID, worldID, libraryID, name,
	)
	if err != nil {
		return Character{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO roleplay_character_profiles (
			library_character_id,summary,voice,traits,goals
		) VALUES ($1,$2,'','[]'::jsonb,'[]'::jsonb)
	`, libraryID, name); err != nil {
		return Character{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO roleplay_scene_participants (scene_id,world_id,character_id,turn_position)
		VALUES ($1,$2,$3,$4)
	`, sceneID, worldID, character.ID, len(locked.Participants)); err != nil {
		return Character{}, simulationDefinitionError("scene participant", err)
	}
	if err := ensureSimulationMetersTx(ctx, tx, worldID, character.ID); err != nil {
		return Character{}, err
	}
	commandTag, err := tx.Exec(ctx, `
		UPDATE roleplay_current_scenes
		SET revision=revision+1,updated_at=NOW()
		WHERE world_id=$1 AND id=$2 AND revision=$3
	`, worldID, sceneID, locked.Sheet.Revision)
	if err != nil {
		return Character{}, err
	}
	if commandTag.RowsAffected() != 1 {
		return Character{}, fmt.Errorf("%w: scene revision changed", ErrSimulationStaleRevision)
	}
	if err := tx.Commit(ctx); err != nil {
		return Character{}, err
	}
	return character, nil
}
