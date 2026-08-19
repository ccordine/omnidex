package roleplay

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type simulationQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func loadMeterPage(ctx context.Context, query simulationQuerier, worldID, characterID string, limit, offset int) ([]MeterProjection, error) {
	rows, err := query.Query(ctx, `
		SELECT meter.meter_key,definition.name,definition.minimum,definition.maximum,meter.value,meter.revision
		FROM roleplay_character_meters AS meter
		JOIN roleplay_meter_definitions AS definition
		  ON definition.world_id=meter.world_id AND definition.meter_key=meter.meter_key
		WHERE meter.world_id=$1 AND meter.character_id=$2
		ORDER BY meter.meter_key ASC LIMIT $3 OFFSET $4
	`, worldID, characterID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]MeterProjection, 0, limit)
	for rows.Next() {
		var item MeterProjection
		if err := rows.Scan(&item.Key, &item.Name, &item.Minimum, &item.Maximum, &item.Value, &item.Revision); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadInventoryPage(ctx context.Context, query simulationQuerier, worldID, characterID string, limit, offset int) ([]InventoryItemProjection, error) {
	rows, err := query.Query(ctx, `
		SELECT inventory.id,template.id,template.name,template.description,
		       template.use_policy,COALESCE(inventory.remaining_uses,0)
		FROM roleplay_inventory_items AS inventory
		JOIN roleplay_item_templates AS template
		  ON template.world_id=inventory.world_id AND template.id=inventory.template_id
		WHERE inventory.world_id=$1 AND inventory.character_id=$2
		ORDER BY inventory.id ASC LIMIT $3 OFFSET $4
	`, worldID, characterID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]InventoryItemProjection, 0, limit)
	for rows.Next() {
		var item InventoryItemProjection
		if err := rows.Scan(&item.ID, &item.TemplateID, &item.Name, &item.Description, &item.UsePolicy, &item.RemainingUses); err != nil {
			return nil, err
		}
		if item.UsePolicy != ItemUseFinite && item.UsePolicy != ItemUseInfinite {
			return nil, fmt.Errorf("persisted inventory item has invalid use policy")
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadCommandEffects(ctx context.Context, query simulationQuerier, commandID string) ([]MeterDelta, error) {
	rows, err := query.Query(ctx, `
		SELECT meter_key,delta FROM roleplay_interaction_command_effects
		WHERE command_id=$1 ORDER BY meter_key ASC
	`, commandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]MeterDelta, 0, MaxDefinitionEffects)
	for rows.Next() {
		var value MeterDelta
		if err := rows.Scan(&value.MeterKey, &value.Delta); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(values) < 1 || len(values) > MaxDefinitionEffects {
		return nil, fmt.Errorf("%w: command effects are outside their bound", ErrSimulationNotConfigured)
	}
	return values, nil
}
