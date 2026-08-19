package roleplay

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func loadMeterDefinitionsTx(ctx context.Context, tx pgx.Tx, worldID string) ([]MeterDefinition, error) {
	rows, err := tx.Query(ctx, `
		SELECT world_id,meter_key,name,minimum,maximum,initial_value
		FROM roleplay_meter_definitions WHERE world_id=$1
		ORDER BY meter_key ASC LIMIT $2
	`, worldID, MaxSimulationMeters+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]MeterDefinition, 0, MaxSimulationMeters)
	for rows.Next() {
		var value MeterDefinition
		if err := rows.Scan(&value.WorldID, &value.Key, &value.Name, &value.Minimum, &value.Maximum, &value.InitialValue); err != nil {
			return nil, err
		}
		if err := validateMeterDefinition(value); err != nil {
			return nil, fmt.Errorf("persisted meter definition is invalid: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(values) > MaxSimulationMeters {
		return nil, fmt.Errorf("%w: meter definitions exceed their bound", ErrSimulationNotConfigured)
	}
	return values, nil
}

func loadInteractionDefinitionsTx(ctx context.Context, tx pgx.Tx, worldID string) ([]InteractionCommandDefinition, error) {
	rows, err := tx.Query(ctx, `
		SELECT id,world_id,command_key,name,description,argument_mode
		FROM roleplay_interaction_commands WHERE world_id=$1
		ORDER BY command_key ASC,id ASC LIMIT $2
	`, worldID, MaxInteractionCommands+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]InteractionCommandDefinition, 0, MaxInteractionCommands)
	for rows.Next() {
		var value InteractionCommandDefinition
		if err := rows.Scan(&value.ID, &value.WorldID, &value.Key, &value.Name, &value.Description, &value.ArgumentMode); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(values) > MaxInteractionCommands {
		return nil, fmt.Errorf("%w: interaction definitions exceed their bound", ErrSimulationNotConfigured)
	}
	for index := range values {
		values[index].Effects, err = loadCommandEffects(ctx, tx, values[index].ID)
		if err != nil {
			return nil, err
		}
		if err := validateCommandDefinition(values[index]); err != nil {
			return nil, fmt.Errorf("persisted interaction definition is invalid: %w", err)
		}
	}
	return values, nil
}

func loadItemDefinitionsTx(ctx context.Context, tx pgx.Tx, worldID string) ([]ItemTemplateDefinition, error) {
	rows, err := tx.Query(ctx, `
		SELECT id,world_id,name,description,use_policy,COALESCE(initial_uses,0),
		       trigger_meter_key,trigger_direction,trigger_threshold,priority
		FROM roleplay_item_templates WHERE world_id=$1
		ORDER BY name ASC,id ASC LIMIT $2
	`, worldID, MaxWorldItemTemplates+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]ItemTemplateDefinition, 0, MaxWorldItemTemplates)
	for rows.Next() {
		var value ItemTemplateDefinition
		var triggerKey *string
		var direction *ThresholdDirection
		var threshold *int
		if err := rows.Scan(
			&value.ID, &value.WorldID, &value.Name, &value.Description,
			&value.UsePolicy, &value.InitialUses, &triggerKey, &direction, &threshold, &value.Priority,
		); err != nil {
			return nil, err
		}
		if triggerKey != nil || direction != nil || threshold != nil {
			if triggerKey == nil || direction == nil || threshold == nil {
				return nil, fmt.Errorf("%w: item trigger is incomplete", ErrSimulationNotConfigured)
			}
			value.Trigger = &ItemTrigger{MeterKey: *triggerKey, Direction: *direction, Threshold: *threshold}
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(values) > MaxWorldItemTemplates {
		return nil, fmt.Errorf("%w: item definitions exceed their bound", ErrSimulationNotConfigured)
	}
	for index := range values {
		values[index].Effects, err = loadItemEffects(ctx, tx, values[index].ID)
		if err != nil {
			return nil, err
		}
		if err := validateItemDefinition(values[index]); err != nil {
			return nil, fmt.Errorf("persisted item definition is invalid: %w", err)
		}
	}
	return values, nil
}

func loadItemEffects(ctx context.Context, query simulationQuerier, templateID string) ([]MeterDelta, error) {
	rows, err := query.Query(ctx, `
		SELECT meter_key,delta FROM roleplay_item_effects
		WHERE template_id=$1 ORDER BY meter_key ASC
	`, templateID)
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
		return nil, fmt.Errorf("%w: item effects are outside their bound", ErrSimulationNotConfigured)
	}
	return values, nil
}
