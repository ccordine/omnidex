package roleplay

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func projectConfiguredSimulationUI(
	ctx context.Context,
	tx pgx.Tx,
	projection SimulationUIProjection,
	page SimulationUIPageRequest,
) (SimulationUIProjection, error) {
	scene, err := projectCurrentSceneTx(ctx, tx, projection.WorldID)
	if errors.Is(err, ErrSimulationNotConfigured) {
		return projection, nil
	}
	if err != nil {
		return SimulationUIProjection{}, err
	}
	projection.Scene = &scene
	projection.AllParticipants, err = loadSceneParticipantsTx(ctx, tx, scene.ID)
	if err != nil {
		return SimulationUIProjection{}, err
	}
	for _, participant := range projection.AllParticipants {
		projection.CharacterNames[participant.CharacterID] = participant.Name
		if participant.CharacterID == scene.ActiveCharacterID {
			projection.ActiveCharacterName = participant.Name
		}
	}
	if projection.ActiveCharacterName == "" {
		return SimulationUIProjection{}, fmt.Errorf("%w: active scene character is absent from turn order", ErrSimulationNotConfigured)
	}
	projection.Participants, err = loadSceneParticipantsPage(
		ctx, tx, projection.WorldID, scene.ID, page.Limit, page.TurnOrderOffset,
	)
	if err != nil {
		return SimulationUIProjection{}, err
	}
	meters, err := loadMeterPage(
		ctx, tx, projection.WorldID, scene.ActiveCharacterID, page.Limit+1, page.MetersOffset,
	)
	if err != nil {
		return SimulationUIProjection{}, err
	}
	projection.Meters = MeterPage{Items: trimPage(meters, page.Limit), HasMore: len(meters) > page.Limit}
	inventory, err := loadInventoryPage(
		ctx, tx, projection.WorldID, scene.ActiveCharacterID, page.Limit+1, page.InventoryOffset,
	)
	if err != nil {
		return SimulationUIProjection{}, err
	}
	projection.Inventory = InventoryPage{Items: trimPage(inventory, page.Limit), HasMore: len(inventory) > page.Limit}
	projection.Interactions, err = loadInteractionCommandsPage(
		ctx, tx, projection.WorldID, page.Limit, page.InteractionsOffset,
	)
	if err != nil {
		return SimulationUIProjection{}, err
	}
	projection.ItemTemplates, err = loadItemTemplatesPage(
		ctx, tx, projection.WorldID, page.Limit, page.ItemTemplatesOffset,
	)
	if err != nil {
		return SimulationUIProjection{}, err
	}
	return projection, nil
}

func trimPage[T any](items []T, limit int) []T {
	if len(items) > limit {
		return items[:limit]
	}
	return items
}
