package roleplay

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func loadTurnPreparationTx(
	ctx context.Context,
	tx pgx.Tx,
	operationID, requestHash string,
) (SimulationTurnAuthority, bool, error) {
	var storedHash string
	var payload []byte
	err := tx.QueryRow(ctx, `
		SELECT request_sha256,result
		FROM roleplay_simulation_turn_preparations
		WHERE operation_id=$1
	`, operationID).Scan(&storedHash, &payload)
	if err == pgx.ErrNoRows {
		return SimulationTurnAuthority{}, false, nil
	}
	if err != nil {
		return SimulationTurnAuthority{}, false, err
	}
	if storedHash != requestHash {
		return SimulationTurnAuthority{}, false, fmt.Errorf("%w: preparation identity was reused", ErrSimulationConflict)
	}
	authority, err := decodeTurnAuthority(payload)
	return authority, true, err
}

func decodeTurnAuthority(payload []byte) (SimulationTurnAuthority, error) {
	var authority SimulationTurnAuthority
	if err := json.Unmarshal(payload, &authority); err != nil {
		return authority, fmt.Errorf("decode simulation turn authority: %w", err)
	}
	if err := authority.Validate(); err != nil {
		return SimulationTurnAuthority{}, fmt.Errorf("persisted simulation turn authority is invalid: %w", err)
	}
	return authority, nil
}

func validSimulationSHA(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range []byte(value) {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}
