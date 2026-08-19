package roleplay

import (
	"context"
	"fmt"
)

type MeterPage struct {
	Items   []MeterProjection `json:"items"`
	HasMore bool              `json:"has_more"`
}

type InventoryPage struct {
	Items   []InventoryItemProjection `json:"items"`
	HasMore bool                      `json:"has_more"`
}

type InteractionCommandPage struct {
	Items   []InteractionCommandDefinition `json:"items"`
	HasMore bool                           `json:"has_more"`
}

type ItemTemplatePage struct {
	Items   []ItemTemplateDefinition `json:"items"`
	HasMore bool                     `json:"has_more"`
}

func (s *Store) ListViewpointMetersPage(
	ctx context.Context,
	worldID, characterID string,
	limit, offset int,
) (MeterPage, error) {
	if err := s.validateContext(ctx); err != nil {
		return MeterPage{}, err
	}
	if err := validateSimulationPage(worldID, limit, offset); err != nil {
		return MeterPage{}, err
	}
	if err := validateIdentity(characterID, characterIdentity); err != nil {
		return MeterPage{}, err
	}
	items, err := loadMeterPage(ctx, s.pool, worldID, characterID, limit+1, offset)
	if err != nil {
		return MeterPage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return MeterPage{Items: items, HasMore: hasMore}, nil
}

func (s *Store) ListInventoryPage(
	ctx context.Context,
	worldID, characterID string,
	limit, offset int,
) (InventoryPage, error) {
	if err := s.validateContext(ctx); err != nil {
		return InventoryPage{}, err
	}
	if err := validateSimulationPage(worldID, limit, offset); err != nil {
		return InventoryPage{}, err
	}
	if err := validateIdentity(characterID, characterIdentity); err != nil {
		return InventoryPage{}, err
	}
	items, err := loadInventoryPage(ctx, s.pool, worldID, characterID, limit+1, offset)
	if err != nil {
		return InventoryPage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return InventoryPage{Items: items, HasMore: hasMore}, nil
}

func (s *Store) ListInteractionCommandsPage(
	ctx context.Context,
	worldID string,
	limit, offset int,
) (InteractionCommandPage, error) {
	if err := s.validateContext(ctx); err != nil {
		return InteractionCommandPage{}, err
	}
	if err := validateSimulationPage(worldID, limit, offset); err != nil {
		return InteractionCommandPage{}, err
	}
	return loadInteractionCommandsPage(ctx, s.pool, worldID, limit, offset)
}

func loadInteractionCommandsPage(
	ctx context.Context,
	query simulationQuerier,
	worldID string,
	limit, offset int,
) (InteractionCommandPage, error) {
	rows, err := query.Query(ctx, `
		SELECT id,world_id,command_key,name,description,argument_mode
		FROM roleplay_interaction_commands
		WHERE world_id=$1
		ORDER BY command_key ASC,id ASC
		LIMIT $2 OFFSET $3
	`, worldID, limit+1, offset)
	if err != nil {
		return InteractionCommandPage{}, err
	}
	defer rows.Close()
	items := make([]InteractionCommandDefinition, 0, limit+1)
	for rows.Next() {
		var item InteractionCommandDefinition
		if err := rows.Scan(&item.ID, &item.WorldID, &item.Key, &item.Name, &item.Description, &item.ArgumentMode); err != nil {
			return InteractionCommandPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return InteractionCommandPage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	for index := range items {
		items[index].Effects, err = loadCommandEffects(ctx, query, items[index].ID)
		if err != nil {
			return InteractionCommandPage{}, err
		}
	}
	return InteractionCommandPage{Items: items, HasMore: hasMore}, nil
}

func (s *Store) ListItemTemplatesPage(
	ctx context.Context,
	worldID string,
	limit, offset int,
) (ItemTemplatePage, error) {
	if err := s.validateContext(ctx); err != nil {
		return ItemTemplatePage{}, err
	}
	if err := validateSimulationPage(worldID, limit, offset); err != nil {
		return ItemTemplatePage{}, err
	}
	return loadItemTemplatesPage(ctx, s.pool, worldID, limit, offset)
}

func requirePageRowsFound(kind string, count int, offset int) error {
	if count == 0 && offset == 0 {
		return fmt.Errorf("%w: %s is absent", ErrSimulationNotConfigured, kind)
	}
	return nil
}
