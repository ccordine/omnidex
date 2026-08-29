package roleplay

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type simulationItemTemplate struct {
	ID               string
	Name             string
	Description      string
	UsePolicy        ItemUsePolicy
	InitialUses      int
	TriggerMeterKey  *string
	TriggerDirection *ThresholdDirection
	TriggerThreshold *int
}

type simulationInventoryCandidate struct {
	InventoryID   string
	TemplateID    string
	Name          string
	Description   string
	UsePolicy     ItemUsePolicy
	RemainingUses *int
	TriggerKey    *string
	Direction     *ThresholdDirection
	Threshold     *int
}

func applySimulationAutoUseTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID, characterID string,
	effects *[]SimulationEffect,
	narrativeEvents *[]string,
) (bool, error) {
	candidates, err := loadSimulationInventoryCandidatesTx(ctx, tx, worldID, characterID)
	if err != nil {
		return false, err
	}
	for _, candidate := range candidates {
		eligible, err := simulationItemEligibleTx(ctx, tx, worldID, characterID, candidate)
		if err != nil {
			return false, err
		}
		if !eligible {
			continue
		}
		deltas, err := loadItemMeterDeltasTx(ctx, tx, candidate.TemplateID)
		if err != nil {
			return false, err
		}
		for _, delta := range deltas {
			if err := applySimulationMeterDeltaTx(
				ctx, tx, worldID, characterID, candidate.TemplateID, delta, effects,
			); err != nil {
				return false, err
			}
		}
		remaining, exhausted, err := consumeSimulationInventoryTx(ctx, tx, candidate)
		if err != nil {
			return false, err
		}
		kind := "item_auto_used"
		if exhausted {
			kind = "item_auto_used_and_exhausted"
		}
		appendSimulationEffect(effects, SimulationEffect{
			Kind: kind, SourceID: candidate.TemplateID, CharacterID: characterID,
			InventoryItemID: candidate.InventoryID, RemainingUses: remaining,
		})
		*narrativeEvents = append(*narrativeEvents, "Used "+candidate.Name+". "+candidate.Description)
		return true, nil
	}
	return false, nil
}

func simulationItemEligibleTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID, characterID string,
	candidate simulationInventoryCandidate,
) (bool, error) {
	if candidate.TriggerKey == nil {
		return false, nil
	}
	if candidate.Direction == nil || candidate.Threshold == nil {
		return false, fmt.Errorf("%w: item trigger is incomplete", ErrSimulationNotConfigured)
	}
	value, err := loadSimulationMeterValueTx(ctx, tx, worldID, characterID, *candidate.TriggerKey)
	if err != nil {
		return false, err
	}
	switch *candidate.Direction {
	case ThresholdAtOrBelow:
		return value <= *candidate.Threshold, nil
	case ThresholdAtOrAbove:
		return value >= *candidate.Threshold, nil
	default:
		return false, fmt.Errorf("%w: item trigger direction is invalid", ErrSimulationNotConfigured)
	}
}

func consumeSimulationInventoryTx(
	ctx context.Context,
	tx pgx.Tx,
	candidate simulationInventoryCandidate,
) (*int, bool, error) {
	if candidate.UsePolicy == ItemUseInfinite {
		if candidate.RemainingUses != nil {
			return nil, false, fmt.Errorf("%w: infinite inventory item carries finite uses", ErrSimulationNotConfigured)
		}
		return nil, false, nil
	}
	if candidate.UsePolicy != ItemUseFinite || candidate.RemainingUses == nil || *candidate.RemainingUses < 1 {
		return nil, false, fmt.Errorf("%w: finite inventory item has invalid uses", ErrSimulationNotConfigured)
	}
	next := *candidate.RemainingUses - 1
	if next == 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM roleplay_inventory_items WHERE id=$1`, candidate.InventoryID); err != nil {
			return nil, false, err
		}
		return nil, true, nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE roleplay_inventory_items SET remaining_uses=$2 WHERE id=$1
	`, candidate.InventoryID, next); err != nil {
		return nil, false, err
	}
	return &next, false, nil
}

func loadNamedItemTemplatesTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID, name string,
) ([]simulationItemTemplate, error) {
	rows, err := tx.Query(ctx, `
		SELECT id,name,description,use_policy,COALESCE(initial_uses,0),
		       trigger_meter_key,trigger_direction,trigger_threshold
		FROM roleplay_item_templates
		WHERE world_id=$1 AND name=$2
		ORDER BY id ASC LIMIT 2
	`, worldID, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]simulationItemTemplate, 0, 2)
	for rows.Next() {
		var item simulationItemTemplate
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Description, &item.UsePolicy, &item.InitialUses,
			&item.TriggerMeterKey, &item.TriggerDirection, &item.TriggerThreshold,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (item simulationItemTemplate) remainingUses() *int {
	if item.UsePolicy != ItemUseFinite {
		return nil
	}
	value := item.InitialUses
	return &value
}

func cloneOptionalInteger(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
