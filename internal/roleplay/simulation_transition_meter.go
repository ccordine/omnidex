package roleplay

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func applySimulationMeterDeltaTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID, characterID, sourceID string,
	delta MeterDelta,
	effects *[]SimulationEffect,
) error {
	var minimum, maximum, before int
	err := tx.QueryRow(ctx, `
		SELECT definition.minimum,definition.maximum,meter.value
		FROM roleplay_character_meters AS meter
		JOIN roleplay_meter_definitions AS definition
		  ON definition.world_id=meter.world_id AND definition.meter_key=meter.meter_key
		WHERE meter.world_id=$1 AND meter.character_id=$2 AND meter.meter_key=$3
		FOR UPDATE OF meter
	`, worldID, characterID, delta.MeterKey).Scan(&minimum, &maximum, &before)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("%w: meter %q is unavailable for the active character", ErrSimulationNotConfigured, delta.MeterKey)
	}
	if err != nil {
		return err
	}
	after := clampMeterValue(before, delta.Delta, minimum, maximum)
	if _, err := tx.Exec(ctx, `
		UPDATE roleplay_character_meters
		SET value=$4,revision=revision+1,updated_at=NOW()
		WHERE world_id=$1 AND character_id=$2 AND meter_key=$3
	`, worldID, characterID, delta.MeterKey, after); err != nil {
		return err
	}
	appendSimulationEffect(effects, SimulationEffect{
		Kind: "meter_changed", SourceID: sourceID, CharacterID: characterID,
		MeterKey: delta.MeterKey, RequestedDelta: delta.Delta,
		BeforeValue: before, AfterValue: after,
	})
	return nil
}

func loadSimulationMeterValueTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID, characterID, meterKey string,
) (int, error) {
	var value int
	err := tx.QueryRow(ctx, `
		SELECT value FROM roleplay_character_meters
		WHERE world_id=$1 AND character_id=$2 AND meter_key=$3
		FOR UPDATE
	`, worldID, characterID, meterKey).Scan(&value)
	if err == pgx.ErrNoRows {
		return 0, fmt.Errorf("%w: trigger meter %q is unavailable", ErrSimulationNotConfigured, meterKey)
	}
	return value, err
}

func clampMeterValue(before, delta, minimum, maximum int) int {
	value := int64(before) + int64(delta)
	if value < int64(minimum) {
		return minimum
	}
	if value > int64(maximum) {
		return maximum
	}
	return int(value)
}

func appendSimulationEffect(effects *[]SimulationEffect, effect SimulationEffect) {
	effect.Sequence = len(*effects) + 1
	*effects = append(*effects, effect)
}
