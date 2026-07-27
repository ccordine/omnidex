package queue

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) beginLockedProjectTx(ctx context.Context, projectID int64, operation string) (pgx.Tx, error) {
	operation = strings.TrimSpace(operation)
	if r == nil || r.pool == nil || ctx == nil || projectID <= 0 || operation == "" {
		return nil, fmt.Errorf("PostgreSQL, context, project, and operation are required for a locked project transaction")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin %s transaction: %w", operation, err)
	}
	var lockedID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM projects WHERE id = $1 FOR UPDATE`, projectID).Scan(&lockedID); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return nil, errors.Join(err, fmt.Errorf("rollback rejected %s transaction: %w", operation, rollbackErr))
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("lock project for %s: %w", operation, err)
	}
	return tx, nil
}

func rollbackTx(ctx context.Context, tx pgx.Tx, operation string) {
	if tx == nil {
		return
	}
	rollbackContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := tx.Rollback(rollbackContext); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		log.Printf("PostgreSQL transaction rollback failed operation=%q: %v", strings.TrimSpace(operation), err)
	}
}
