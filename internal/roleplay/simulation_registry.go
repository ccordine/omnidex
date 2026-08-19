package roleplay

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Store) RegisterMeter(ctx context.Context, definition MeterDefinition) error {
	if err := s.validateContext(ctx); err != nil {
		return err
	}
	if err := validateMeterDefinition(definition); err != nil {
		return err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err := requireDefinitionCapacityTx(ctx, tx, "roleplay_meter_definitions", definition.WorldID, MaxSimulationMeters); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO roleplay_meter_definitions (
			world_id,meter_key,name,minimum,maximum,initial_value
		) VALUES ($1,$2,$3,$4,$5,$6)
	`, definition.WorldID, definition.Key, definition.Name, definition.Minimum, definition.Maximum, definition.InitialValue); err != nil {
		return simulationDefinitionError("meter", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO roleplay_character_meters (world_id,character_id,meter_key,value)
		SELECT $1,id,$2,$3 FROM roleplay_characters WHERE world_id=$1
	`, definition.WorldID, definition.Key, definition.InitialValue); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RegisterInteractionCommand(
	ctx context.Context,
	definition InteractionCommandDefinition,
) error {
	if err := s.validateContext(ctx); err != nil {
		return err
	}
	if err := validateCommandDefinition(definition); err != nil {
		return err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err := requireDefinitionCapacityTx(ctx, tx, "roleplay_interaction_commands", definition.WorldID, MaxInteractionCommands); err != nil {
		return err
	}
	if err := requireMeterEffectsTx(ctx, tx, definition.WorldID, definition.Effects); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO roleplay_interaction_commands (
			id,world_id,command_key,name,description,argument_mode
		) VALUES ($1,$2,$3,$4,$5,$6)
	`, definition.ID, definition.WorldID, definition.Key, definition.Name,
		definition.Description, definition.ArgumentMode); err != nil {
		return simulationDefinitionError("interaction command", err)
	}
	for _, effect := range definition.Effects {
		if _, err := tx.Exec(ctx, `
			INSERT INTO roleplay_interaction_command_effects (
				command_id,world_id,meter_key,delta
			) VALUES ($1,$2,$3,$4)
		`, definition.ID, definition.WorldID, effect.MeterKey, effect.Delta); err != nil {
			return simulationDefinitionError("interaction command effect", err)
		}
	}
	return tx.Commit(ctx)
}

func requireDefinitionCapacityTx(
	ctx context.Context,
	tx pgx.Tx,
	table, worldID string,
	maximum int,
) error {
	if err := lockSimulationWorldTx(ctx, tx, worldID); err != nil {
		return err
	}
	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE world_id=$1", pgx.Identifier{table}.Sanitize())
	if err := tx.QueryRow(ctx, query, worldID).Scan(&count); err != nil {
		return err
	}
	if count >= maximum {
		return fmt.Errorf("%w: %s reached its %d-definition bound", ErrSimulationConflict, table, maximum)
	}
	return nil
}

func lockSimulationWorldTx(ctx context.Context, tx pgx.Tx, worldID string) error {
	var found string
	if err := tx.QueryRow(ctx, `
		SELECT id FROM roleplay_worlds WHERE id=$1 FOR UPDATE
	`, worldID).Scan(&found); err == pgx.ErrNoRows {
		return fmt.Errorf("%w: fictional world is absent", ErrSimulationNotConfigured)
	} else if err != nil {
		return err
	}
	return nil
}

func requireMeterEffectsTx(ctx context.Context, tx pgx.Tx, worldID string, effects []MeterDelta) error {
	for _, effect := range effects {
		var found string
		if err := tx.QueryRow(ctx, `
			SELECT meter_key FROM roleplay_meter_definitions
			WHERE world_id=$1 AND meter_key=$2
		`, worldID, effect.MeterKey).Scan(&found); err == pgx.ErrNoRows {
			return fmt.Errorf("%w: meter %q is not registered", ErrSimulationNotConfigured, effect.MeterKey)
		} else if err != nil {
			return err
		}
	}
	return nil
}

func ensureSimulationMetersTx(ctx context.Context, tx pgx.Tx, worldID, characterID string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO roleplay_character_meters (world_id,character_id,meter_key,value)
		SELECT definition.world_id,$2,definition.meter_key,definition.initial_value
		FROM roleplay_meter_definitions AS definition
		WHERE definition.world_id=$1
		ON CONFLICT (character_id,meter_key) DO NOTHING
	`, worldID, characterID)
	return err
}

func simulationDefinitionError(name string, err error) error {
	var postgres *pgconn.PgError
	if errors.As(err, &postgres) && (postgres.Code == "23505" || postgres.Code == "23503" || postgres.Code == "23514") {
		return fmt.Errorf("%w: invalid or duplicate %s", ErrSimulationConflict, name)
	}
	return err
}
