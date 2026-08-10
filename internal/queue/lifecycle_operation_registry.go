package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func reserveLifecycleOperationIdentityTx(
	ctx context.Context,
	tx pgx.Tx,
	id LifecycleOperationID,
	kind LifecycleOperationKind,
	commandSHA256 string,
	commandPayload []byte,
) (bool, error) {
	if _, err := ParseLifecycleOperationID(string(id)); err != nil {
		return false, err
	}
	result, err := tx.Exec(ctx, `
		INSERT INTO lifecycle_operation_registry (
			operation_id, kind, command_sha256, command_payload
		) VALUES ($1,$2,$3,$4::jsonb)
		ON CONFLICT (operation_id) DO NOTHING
	`, id, kind, commandSHA256, string(commandPayload))
	if err != nil {
		return false, fmt.Errorf("reserve lifecycle operation identity %q: %w", id, err)
	}
	if result.RowsAffected() == 1 {
		return true, nil
	}
	var persistedKind LifecycleOperationKind
	var persistedSHA string
	var payloadMatches bool
	if err := tx.QueryRow(ctx, `
		SELECT kind, command_sha256, command_payload=$2::jsonb
		FROM lifecycle_operation_registry
		WHERE operation_id=$1
	`, id, string(commandPayload)).Scan(&persistedKind, &persistedSHA, &payloadMatches); err != nil {
		return false, fmt.Errorf("read lifecycle operation identity %q: %w", id, err)
	}
	if persistedKind != kind || persistedSHA != commandSHA256 || !payloadMatches {
		return false, fmt.Errorf(
			"%w: operation ID %q is reserved for different kind, scope, or command content",
			ErrLifecycleOperationConflict,
			id,
		)
	}
	return false, nil
}
