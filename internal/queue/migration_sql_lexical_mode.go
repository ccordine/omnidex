package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type migrationSQLModeExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func enforceMigrationSessionSQLMode(
	ctx context.Context,
	executor migrationSQLModeExecutor,
) error {
	return enforceMigrationSQLMode(
		ctx, executor, "SET SESSION standard_conforming_strings TO on", "session",
	)
}

func enforceMigrationTransactionSQLMode(
	ctx context.Context,
	executor migrationSQLModeExecutor,
) error {
	return enforceMigrationSQLMode(
		ctx, executor, "SET LOCAL standard_conforming_strings TO on", "transaction",
	)
}

func enforceMigrationSQLMode(
	ctx context.Context,
	executor migrationSQLModeExecutor,
	command string,
	scope string,
) error {
	if _, err := executor.Exec(ctx, command); err != nil {
		return fmt.Errorf("enforce migration %s SQL lexical mode: %w", scope, err)
	}
	var value string
	if err := executor.QueryRow(ctx, "SHOW standard_conforming_strings").Scan(&value); err != nil {
		return fmt.Errorf("verify migration %s SQL lexical mode: %w", scope, err)
	}
	if value != "on" {
		return fmt.Errorf(
			"migration %s standard_conforming_strings=%q, want on", scope, value,
		)
	}
	return nil
}
