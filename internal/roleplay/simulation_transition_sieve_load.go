package roleplay

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func loadSimulationInventoryCandidatesTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID, characterID string,
) ([]simulationInventoryCandidate, error) {
	rows, err := tx.Query(ctx, `
		SELECT inventory.id,inventory.template_id,template.name,template.description,
		       template.use_policy,inventory.remaining_uses,
		       template.trigger_meter_key,template.trigger_direction,template.trigger_threshold
		FROM roleplay_inventory_items AS inventory
		JOIN roleplay_item_templates AS template
		  ON template.world_id=inventory.world_id AND template.id=inventory.template_id
		WHERE inventory.world_id=$1 AND inventory.character_id=$2
		ORDER BY template.priority DESC,inventory.id ASC
		LIMIT $3
		FOR UPDATE OF inventory
	`, worldID, characterID, MaxInventoryItems+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]simulationInventoryCandidate, 0, MaxInventoryItems)
	for rows.Next() {
		var item simulationInventoryCandidate
		if err := rows.Scan(
			&item.InventoryID, &item.TemplateID, &item.Name, &item.Description,
			&item.UsePolicy, &item.RemainingUses,
			&item.TriggerKey, &item.Direction, &item.Threshold,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) > MaxInventoryItems {
		return nil, fmt.Errorf("%w: inventory exceeds %d items", ErrSimulationNotConfigured, MaxInventoryItems)
	}
	return items, nil
}

func loadItemMeterDeltasTx(ctx context.Context, tx pgx.Tx, templateID string) ([]MeterDelta, error) {
	rows, err := tx.Query(ctx, `
		SELECT meter_key,delta
		FROM roleplay_item_effects
		WHERE template_id=$1
		ORDER BY meter_key ASC
	`, templateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	deltas := make([]MeterDelta, 0, MaxDefinitionEffects)
	for rows.Next() {
		var delta MeterDelta
		if err := rows.Scan(&delta.MeterKey, &delta.Delta); err != nil {
			return nil, err
		}
		deltas = append(deltas, delta)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(deltas) < 1 || len(deltas) > MaxDefinitionEffects {
		return nil, fmt.Errorf("%w: item effects are outside their bound", ErrSimulationNotConfigured)
	}
	return deltas, nil
}
