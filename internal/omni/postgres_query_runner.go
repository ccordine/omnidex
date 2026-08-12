package omni

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const readOnlyQueryRollbackTimeout = 5 * time.Second

type MemorySQLRunner interface {
	Query(ctx context.Context, sql string, args ...any) ([]MemorySQLRow, error)
}

type MemorySQLRow map[string]any

type PgxMemoryRunner struct {
	pool *pgxpool.Pool
}

func NewPgxMemoryRunner(pool *pgxpool.Pool) *PgxMemoryRunner {
	return &PgxMemoryRunner{pool: pool}
}

func (r *PgxMemoryRunner) Query(
	ctx context.Context,
	sql string,
	args ...any,
) ([]MemorySQLRow, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return nil, fmt.Errorf("PostgreSQL query runner requires a pool and context")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("begin PostgreSQL read-only query transaction: %w", err)
	}
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, rollbackReadOnlyQuery(tx, fmt.Errorf("execute PostgreSQL read-only query: %w", err))
	}

	fieldDescriptions := rows.FieldDescriptions()
	out := []MemorySQLRow{}
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			rows.Close()
			return nil, rollbackReadOnlyQuery(tx, fmt.Errorf("read PostgreSQL query row: %w", err))
		}
		row := MemorySQLRow{}
		for index, field := range fieldDescriptions {
			row[string(field.Name)] = values[index]
		}
		out = append(out, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, rollbackReadOnlyQuery(tx, fmt.Errorf("iterate PostgreSQL query rows: %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit PostgreSQL read-only query transaction: %w", err)
	}
	return out, nil
}

func rollbackReadOnlyQuery(tx pgx.Tx, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), readOnlyQueryRollbackTimeout)
	defer cancel()
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return errors.Join(cause, fmt.Errorf("rollback PostgreSQL read-only query transaction: %w", err))
	}
	return cause
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%v", value)
}
