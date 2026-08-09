package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// lockLifecycleOperationIdentityTx serializes every mutation carrying the same
// global operation identity before any aggregate-specific lock is acquired.
func lockLifecycleOperationIdentityTx(
	ctx context.Context,
	tx pgx.Tx,
	id LifecycleOperationID,
) error {
	if _, err := ParseLifecycleOperationID(string(id)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))
	`, string(id)); err != nil {
		return fmt.Errorf("lock lifecycle operation identity %q: %w", id, err)
	}
	return nil
}
