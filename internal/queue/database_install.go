package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// InstallDatabase executes the sole checked-in schema definition after the
// runtime connection constructor has atomically recreated its dedicated
// schema. It has no second reset, lock, migration, or fallback authority.
func (r *Repository) InstallDatabase(ctx context.Context, setupSQL []byte) error {
	if ctx == nil || r == nil || r.pool == nil {
		return fmt.Errorf("install database requires PostgreSQL and context")
	}
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire database setup connection: %w", err)
	}
	defer conn.Release()

	var runtimeSchema string
	if err := conn.QueryRow(ctx, `SELECT current_schema()`).Scan(&runtimeSchema); err != nil {
		return fmt.Errorf("load database setup runtime schema: %w", err)
	}
	body, err := renderDatabaseSetup(setupSQL, runtimeSchema)
	if err != nil {
		return err
	}
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin database setup transaction: %w", err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `
		SET LOCAL standard_conforming_strings TO on;
		SET LOCAL check_function_bodies TO false;
	`); err != nil {
		return fmt.Errorf("set database setup SQL mode: %w", err)
	}
	if _, err := tx.Exec(ctx, string(body)); err != nil {
		return fmt.Errorf("execute authoritative database setup: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit authoritative database setup: %w", err)
	}
	return nil
}
