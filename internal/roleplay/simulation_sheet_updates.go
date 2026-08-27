package roleplay

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Store) UpdateCurrentScene(ctx context.Context, update SceneUpdate) (SceneSheet, error) {
	if err := s.validateContext(ctx); err != nil {
		return SceneSheet{}, err
	}
	setup := SceneSetup{
		ID: update.SceneID, WorldID: update.WorldID, Title: update.Title,
		Description: update.Description, ParticipantIDs: update.ParticipantIDs,
	}
	if err := validateSceneSetup(setup); err != nil {
		return SceneSheet{}, err
	}
	if update.ExpectedRevision < 1 {
		return SceneSheet{}, fmt.Errorf("scene update requires a positive expected revision")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SceneSheet{}, err
	}
	defer tx.Rollback(context.Background())
	locked, err := lockSimulationSceneTx(ctx, tx, update.WorldID, update.SceneID)
	if err != nil {
		return SceneSheet{}, err
	}
	if locked.Sheet.Revision != update.ExpectedRevision {
		return SceneSheet{}, fmt.Errorf("%w: scene revision changed", ErrSimulationStaleRevision)
	}
	if err := requireSimulationParticipantsTx(ctx, tx, update.WorldID, update.ParticipantIDs); err != nil {
		return SceneSheet{}, err
	}
	active := locked.Sheet.ActiveCharacterID
	if !containsSimulationCharacter(update.ParticipantIDs, active) {
		active = update.ParticipantIDs[0]
	}
	if _, err := tx.Exec(ctx, `DELETE FROM roleplay_scene_participants WHERE scene_id=$1`, update.SceneID); err != nil {
		return SceneSheet{}, err
	}
	for position, characterID := range update.ParticipantIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO roleplay_scene_participants (scene_id,world_id,character_id,turn_position)
			VALUES ($1,$2,$3,$4)
		`, update.SceneID, update.WorldID, characterID, position); err != nil {
			return SceneSheet{}, simulationDefinitionError("scene participant", err)
		}
		if err := ensureSimulationMetersTx(ctx, tx, update.WorldID, characterID); err != nil {
			return SceneSheet{}, err
		}
	}
	scene, err := scanSceneSheet(tx.QueryRow(ctx, `
		UPDATE roleplay_current_scenes
		SET title=$3,description=$4,current_character_id=$5,
		    revision=revision+1,updated_at=NOW()
		WHERE world_id=$1 AND id=$2 AND revision=$6
		RETURNING id,world_id,title,description,revision,current_character_id,
		          initiative_round,initiative_turn,fictional_time_tick,created_at,updated_at
	`, update.WorldID, update.SceneID, update.Title, update.Description, active, update.ExpectedRevision))
	if err == pgx.ErrNoRows {
		return SceneSheet{}, fmt.Errorf("%w: scene revision changed", ErrSimulationStaleRevision)
	}
	if err != nil {
		return SceneSheet{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SceneSheet{}, err
	}
	return scene, nil
}

func (s *Store) SetCharacterMeter(ctx context.Context, update MeterValueUpdate) (MeterProjection, error) {
	if err := s.validateContext(ctx); err != nil {
		return MeterProjection{}, err
	}
	if err := validateIdentity(update.WorldID, worldIdentity); err != nil {
		return MeterProjection{}, err
	}
	if err := validateIdentity(update.CharacterID, characterIdentity); err != nil {
		return MeterProjection{}, err
	}
	if err := validateSimulationKey("meter", update.MeterKey); err != nil {
		return MeterProjection{}, err
	}
	if update.ExpectedRevision < 1 {
		return MeterProjection{}, fmt.Errorf("meter update requires a positive expected revision")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return MeterProjection{}, err
	}
	defer tx.Rollback(context.Background())
	var projection MeterProjection
	err = tx.QueryRow(ctx, `
		SELECT meter.meter_key,definition.name,definition.minimum,definition.maximum,
		       meter.value,meter.revision
		FROM roleplay_character_meters AS meter
		JOIN roleplay_meter_definitions AS definition
		  ON definition.world_id=meter.world_id AND definition.meter_key=meter.meter_key
		WHERE meter.world_id=$1 AND meter.character_id=$2 AND meter.meter_key=$3
		FOR UPDATE OF meter
	`, update.WorldID, update.CharacterID, update.MeterKey).Scan(
		&projection.Key, &projection.Name, &projection.Minimum, &projection.Maximum,
		&projection.Value, &projection.Revision,
	)
	if err == pgx.ErrNoRows {
		return MeterProjection{}, fmt.Errorf("%w: character meter is absent", ErrSimulationNotConfigured)
	}
	if err != nil {
		return MeterProjection{}, err
	}
	if projection.Revision != update.ExpectedRevision {
		return MeterProjection{}, fmt.Errorf("%w: meter revision changed", ErrSimulationStaleRevision)
	}
	if update.Value < projection.Minimum || update.Value > projection.Maximum {
		return MeterProjection{}, fmt.Errorf("%w: meter value is outside %d..%d", ErrSimulationIllegal, projection.Minimum, projection.Maximum)
	}
	err = tx.QueryRow(ctx, `
		UPDATE roleplay_character_meters
		SET value=$4,revision=revision+1,updated_at=NOW()
		WHERE world_id=$1 AND character_id=$2 AND meter_key=$3 AND revision=$5
		RETURNING value,revision
	`, update.WorldID, update.CharacterID, update.MeterKey, update.Value, update.ExpectedRevision).Scan(
		&projection.Value, &projection.Revision,
	)
	if err == pgx.ErrNoRows {
		return MeterProjection{}, fmt.Errorf("%w: meter revision changed", ErrSimulationStaleRevision)
	}
	if err != nil {
		return MeterProjection{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return MeterProjection{}, err
	}
	return projection, nil
}

func containsSimulationCharacter(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
