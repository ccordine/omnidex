package roleplay

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (s *Store) RegisterItemTemplate(ctx context.Context, definition ItemTemplateDefinition) error {
	if err := s.validateContext(ctx); err != nil {
		return err
	}
	if err := validateItemDefinition(definition); err != nil {
		return err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err := requireDefinitionCapacityTx(ctx, tx, "roleplay_item_templates", definition.WorldID, MaxWorldItemTemplates); err != nil {
		return err
	}
	if err := requireMeterEffectsTx(ctx, tx, definition.WorldID, definition.Effects); err != nil {
		return err
	}
	var triggerKey any
	var triggerDirection any
	var triggerThreshold any
	if definition.Trigger != nil {
		if err := requireMeterEffectsTx(ctx, tx, definition.WorldID, []MeterDelta{{
			MeterKey: definition.Trigger.MeterKey, Delta: 1,
		}}); err != nil {
			return err
		}
		triggerKey = definition.Trigger.MeterKey
		triggerDirection = definition.Trigger.Direction
		triggerThreshold = definition.Trigger.Threshold
	}
	var initialUses any
	if definition.UsePolicy == ItemUseFinite {
		initialUses = definition.InitialUses
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO roleplay_item_templates (
			id,world_id,name,description,use_policy,initial_uses,
			trigger_meter_key,trigger_direction,trigger_threshold,priority
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, definition.ID, definition.WorldID, definition.Name, definition.Description,
		definition.UsePolicy, initialUses, triggerKey, triggerDirection, triggerThreshold, definition.Priority); err != nil {
		return simulationDefinitionError("item template", err)
	}
	for _, effect := range definition.Effects {
		if _, err := tx.Exec(ctx, `
			INSERT INTO roleplay_item_effects (template_id,world_id,meter_key,delta)
			VALUES ($1,$2,$3,$4)
		`, definition.ID, definition.WorldID, effect.MeterKey, effect.Delta); err != nil {
			return simulationDefinitionError("item effect", err)
		}
	}
	return tx.Commit(ctx)
}
