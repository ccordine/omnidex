package roleplay

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func applyExplicitSimulationActionTx(
	ctx context.Context,
	tx pgx.Tx,
	operationID, worldID, characterID string,
	action SimulationAction,
	effects *[]SimulationEffect,
	narrativeEvents *[]string,
) error {
	switch action.Kind {
	case SimulationActionGive:
		return giveSimulationItemTx(ctx, tx, operationID, worldID, characterID, action.Argument, effects, narrativeEvents)
	case SimulationActionTake:
		return takeSimulationItemTx(ctx, tx, worldID, characterID, action.Argument, effects, narrativeEvents)
	case SimulationActionInteraction:
		return applyInteractionCommandTx(ctx, tx, worldID, characterID, action, effects, narrativeEvents)
	default:
		return fmt.Errorf("%w: unsupported simulation action", ErrSimulationIllegal)
	}
}

func applyInteractionCommandTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID, characterID string,
	action SimulationAction,
	effects *[]SimulationEffect,
	narrativeEvents *[]string,
) error {
	var commandID string
	var argumentMode CommandArgumentMode
	var description string
	err := tx.QueryRow(ctx, `
		SELECT id,argument_mode,description
		FROM roleplay_interaction_commands
		WHERE world_id=$1 AND command_key=$2
	`, worldID, action.CommandKey).Scan(&commandID, &argumentMode, &description)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("%w: interaction command %q", ErrSimulationUnknown, action.CommandKey)
	}
	if err != nil {
		return err
	}
	if argumentMode == CommandArgumentRequired && !action.HasArgument {
		return fmt.Errorf("%w: /%s requires one exact quoted argument", ErrSimulationIllegal, action.CommandKey)
	}
	if argumentMode == CommandArgumentNone && action.HasArgument {
		return fmt.Errorf("%w: /%s accepts no argument", ErrSimulationIllegal, action.CommandKey)
	}
	deltas, err := loadCommandMeterDeltasTx(ctx, tx, commandID)
	if err != nil {
		return err
	}
	for _, delta := range deltas {
		if err := applySimulationMeterDeltaTx(
			ctx, tx, worldID, characterID, commandID, delta, effects,
		); err != nil {
			return err
		}
	}
	event := description
	if action.HasArgument {
		event += "\nDetail: " + action.Argument
	}
	*narrativeEvents = append(*narrativeEvents, event)
	return nil
}

func giveSimulationItemTx(
	ctx context.Context,
	tx pgx.Tx,
	operationID, worldID, characterID, name string,
	effects *[]SimulationEffect,
	narrativeEvents *[]string,
) error {
	templates, err := loadNamedItemTemplatesTx(ctx, tx, worldID, name)
	if err != nil {
		return err
	}
	if len(templates) == 0 {
		return fmt.Errorf("%w: item %q", ErrSimulationUnknown, name)
	}
	if len(templates) != 1 {
		return fmt.Errorf("%w: item %q", ErrSimulationAmbiguous, name)
	}
	template := templates[0]
	var existing string
	err = tx.QueryRow(ctx, `
		SELECT id FROM roleplay_inventory_items
		WHERE character_id=$1 AND template_id=$2
	`, characterID, template.ID).Scan(&existing)
	if err == nil {
		return fmt.Errorf("%w: active character already holds item %q", ErrSimulationIllegal, name)
	}
	if err != pgx.ErrNoRows {
		return err
	}
	inventoryID := "rpv_" + simulationSHA([]byte(
		"inventory-item.v1\x00" + operationID + "\x00" + worldID + "\x00" + characterID + "\x00" + template.ID,
	))[:32]
	var remaining any
	if template.UsePolicy == ItemUseFinite {
		remaining = template.InitialUses
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO roleplay_inventory_items (
			id,world_id,character_id,template_id,remaining_uses
		) VALUES ($1,$2,$3,$4,$5)
	`, inventoryID, worldID, characterID, template.ID, remaining); err != nil {
		return simulationDefinitionError("inventory item", err)
	}
	appendSimulationEffect(effects, SimulationEffect{
		Kind: "inventory_added", SourceID: template.ID,
		CharacterID: characterID, InventoryItemID: inventoryID,
		RemainingUses: cloneOptionalInteger(template.remainingUses()),
	})
	*narrativeEvents = append(*narrativeEvents, "Received "+template.Name+". "+template.Description)
	return nil
}

func takeSimulationItemTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID, characterID, name string,
	effects *[]SimulationEffect,
	narrativeEvents *[]string,
) error {
	rows, err := tx.Query(ctx, `
		SELECT inventory.id,template.id,template.name,template.description
		FROM roleplay_inventory_items AS inventory
		JOIN roleplay_item_templates AS template
		  ON template.world_id=inventory.world_id AND template.id=inventory.template_id
		WHERE inventory.world_id=$1 AND inventory.character_id=$2 AND template.name=$3
		ORDER BY inventory.id ASC
		LIMIT 2
		FOR UPDATE OF inventory
	`, worldID, characterID, name)
	if err != nil {
		return err
	}
	defer rows.Close()
	type heldItem struct{ inventoryID, templateID, name, description string }
	items := make([]heldItem, 0, 2)
	for rows.Next() {
		var item heldItem
		if err := rows.Scan(&item.inventoryID, &item.templateID, &item.name, &item.description); err != nil {
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("%w: held item %q", ErrSimulationUnknown, name)
	}
	if len(items) != 1 {
		return fmt.Errorf("%w: held item %q", ErrSimulationAmbiguous, name)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM roleplay_inventory_items WHERE id=$1`, items[0].inventoryID); err != nil {
		return err
	}
	appendSimulationEffect(effects, SimulationEffect{
		Kind: "inventory_removed", SourceID: items[0].templateID,
		CharacterID: characterID, InventoryItemID: items[0].inventoryID,
	})
	*narrativeEvents = append(*narrativeEvents, "Released "+items[0].name+". "+items[0].description)
	return nil
}

func loadCommandMeterDeltasTx(ctx context.Context, tx pgx.Tx, commandID string) ([]MeterDelta, error) {
	rows, err := tx.Query(ctx, `
		SELECT meter_key,delta
		FROM roleplay_interaction_command_effects
		WHERE command_id=$1
		ORDER BY meter_key ASC
	`, commandID)
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
		return nil, fmt.Errorf("%w: interaction command effects are outside their bound", ErrSimulationNotConfigured)
	}
	return deltas, nil
}
