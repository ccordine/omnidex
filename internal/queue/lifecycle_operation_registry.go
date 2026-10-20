package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func reserveLifecycleOperationIdentityTx(
	ctx context.Context,
	tx pgx.Tx,
	id model.LifecycleOperationID,
	kind LifecycleOperationKind,
	commandPayload []byte,
) (bool, error) {
	if _, err := model.ParseLifecycleOperationID(string(id)); err != nil {
		return false, err
	}
	var intentExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM channel_user_intents WHERE operation_id=$1)`, id).Scan(&intentExists); err != nil {
		return false, err
	}
	if intentExists {
		return false, ErrLifecycleOperationConflict
	}
	result, err := tx.Exec(ctx, `
		INSERT INTO lifecycle_operation_registry (
			operation_id, kind, command_payload
		) VALUES ($1,$2,$3::jsonb)
		ON CONFLICT (operation_id) DO NOTHING
	`, id, kind, string(commandPayload))
	if err != nil {
		return false, fmt.Errorf("reserve lifecycle operation identity %q: %w", id, err)
	}
	if result.RowsAffected() == 1 {
		return true, nil
	}
	var persistedKind LifecycleOperationKind
	var payloadMatches bool
	if err := tx.QueryRow(ctx, `
		SELECT kind, command_payload=$2::jsonb
		FROM lifecycle_operation_registry
		WHERE operation_id=$1
	`, id, string(commandPayload)).Scan(&persistedKind, &payloadMatches); err != nil {
		return false, fmt.Errorf("read lifecycle operation identity %q: %w", id, err)
	}
	if persistedKind != kind || !payloadMatches {
		return false, fmt.Errorf(
			"%w: operation ID %q is reserved for different kind, scope, or command content",
			ErrLifecycleOperationConflict,
			id,
		)
	}
	return false, nil
}

// Admission holds the same operation lock as lifecycle writers. An intent may
// not reinterpret an identity already committed by any lifecycle entry point.
func rejectLifecycleUserIntentCollisionTx(ctx context.Context, tx pgx.Tx, id model.LifecycleOperationID) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_operation_registry WHERE operation_id=$1)`, id).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrLifecycleOperationConflict
	}
	return nil
}
