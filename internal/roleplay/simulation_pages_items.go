package roleplay

import (
	"context"
	"fmt"
)

func loadItemTemplatesPage(
	ctx context.Context,
	query simulationQuerier,
	worldID string,
	limit, offset int,
) (ItemTemplatePage, error) {
	rows, err := query.Query(ctx, `
		SELECT id,world_id,name,description,use_policy,COALESCE(initial_uses,0),
		       trigger_meter_key,trigger_direction,trigger_threshold,priority
		FROM roleplay_item_templates
		WHERE world_id=$1
		ORDER BY name ASC,id ASC
		LIMIT $2 OFFSET $3
	`, worldID, limit+1, offset)
	if err != nil {
		return ItemTemplatePage{}, err
	}
	defer rows.Close()
	items := make([]ItemTemplateDefinition, 0, limit+1)
	for rows.Next() {
		var item ItemTemplateDefinition
		var triggerKey *string
		var direction *ThresholdDirection
		var threshold *int
		if err := rows.Scan(
			&item.ID, &item.WorldID, &item.Name, &item.Description, &item.UsePolicy,
			&item.InitialUses, &triggerKey, &direction, &threshold, &item.Priority,
		); err != nil {
			return ItemTemplatePage{}, err
		}
		if triggerKey != nil || direction != nil || threshold != nil {
			if triggerKey == nil || direction == nil || threshold == nil {
				return ItemTemplatePage{}, fmt.Errorf("%w: item trigger is incomplete", ErrSimulationNotConfigured)
			}
			item.Trigger = &ItemTrigger{MeterKey: *triggerKey, Direction: *direction, Threshold: *threshold}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ItemTemplatePage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	for index := range items {
		items[index].Effects, err = loadItemEffects(ctx, query, items[index].ID)
		if err != nil {
			return ItemTemplatePage{}, err
		}
		if err := validateItemDefinition(items[index]); err != nil {
			return ItemTemplatePage{}, fmt.Errorf("persisted item definition is invalid: %w", err)
		}
	}
	return ItemTemplatePage{Items: items, HasMore: hasMore}, nil
}
