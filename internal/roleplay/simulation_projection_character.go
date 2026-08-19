package roleplay

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func loadMeterProjectionsTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID, characterID string,
	limit, offset int,
) ([]MeterProjection, error) {
	rows, err := tx.Query(ctx, `
		SELECT meter.meter_key,definition.name,definition.minimum,definition.maximum,
		       meter.value,meter.revision
		FROM roleplay_character_meters AS meter
		JOIN roleplay_meter_definitions AS definition
		  ON definition.world_id=meter.world_id AND definition.meter_key=meter.meter_key
		WHERE meter.world_id=$1 AND meter.character_id=$2
		ORDER BY meter.meter_key ASC
		LIMIT $3 OFFSET $4
	`, worldID, characterID, limit+1, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	meters := make([]MeterProjection, 0, limit+1)
	for rows.Next() {
		var meter MeterProjection
		if err := rows.Scan(
			&meter.Key, &meter.Name, &meter.Minimum, &meter.Maximum,
			&meter.Value, &meter.Revision,
		); err != nil {
			return nil, err
		}
		meters = append(meters, meter)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(meters) > limit {
		return nil, fmt.Errorf("bounded meter projection has more rows")
	}
	return meters, nil
}

func loadInventoryProjectionsTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID, characterID string,
	limit, offset int,
) ([]InventoryItemProjection, error) {
	rows, err := tx.Query(ctx, `
		SELECT inventory.id,template.id,template.name,template.description,
		       template.use_policy,COALESCE(inventory.remaining_uses,0)
		FROM roleplay_inventory_items AS inventory
		JOIN roleplay_item_templates AS template
		  ON template.world_id=inventory.world_id AND template.id=inventory.template_id
		WHERE inventory.world_id=$1 AND inventory.character_id=$2
		ORDER BY inventory.id ASC
		LIMIT $3 OFFSET $4
	`, worldID, characterID, limit+1, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]InventoryItemProjection, 0, limit+1)
	for rows.Next() {
		var item InventoryItemProjection
		if err := rows.Scan(
			&item.ID, &item.TemplateID, &item.Name, &item.Description,
			&item.UsePolicy, &item.RemainingUses,
		); err != nil {
			return nil, err
		}
		if item.UsePolicy != ItemUseFinite && item.UsePolicy != ItemUseInfinite {
			return nil, fmt.Errorf("persisted inventory item has invalid use policy")
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) > limit {
		return nil, fmt.Errorf("bounded inventory projection has more rows")
	}
	return items, nil
}
