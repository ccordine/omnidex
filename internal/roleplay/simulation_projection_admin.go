package roleplay

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (s *Store) ProjectSimulationAdmin(ctx context.Context, worldID string) (SimulationAdminProjection, error) {
	if err := s.validateContext(ctx); err != nil {
		return SimulationAdminProjection{}, err
	}
	if err := validateIdentity(worldID, worldIdentity); err != nil {
		return SimulationAdminProjection{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return SimulationAdminProjection{}, err
	}
	defer tx.Rollback(context.Background())
	scene, err := projectCurrentSceneTx(ctx, tx, worldID)
	if err != nil {
		return SimulationAdminProjection{}, err
	}
	participants, err := loadSceneParticipantsTx(ctx, tx, scene.ID)
	if err != nil {
		return SimulationAdminProjection{}, err
	}
	meters, err := loadMeterDefinitionsTx(ctx, tx, worldID)
	if err != nil {
		return SimulationAdminProjection{}, err
	}
	commands, err := loadInteractionDefinitionsTx(ctx, tx, worldID)
	if err != nil {
		return SimulationAdminProjection{}, err
	}
	items, err := loadItemDefinitionsTx(ctx, tx, worldID)
	if err != nil {
		return SimulationAdminProjection{}, err
	}
	projection := SimulationAdminProjection{
		Schema: SimulationAdminProjectionSchemaV1, WorldID: worldID,
		Scene: scene, Participants: participants, Meters: meters,
		Commands: commands, Items: items,
	}
	if err := tx.Commit(ctx); err != nil {
		return SimulationAdminProjection{}, err
	}
	return projection, nil
}
