package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type offlineAuthorityValidator func(context.Context, pgx.Tx) error

func executeOfflineAuthorityTransaction(
	ctx context.Context,
	admin *pgxpool.Pool,
	label string,
	commands []string,
	validate offlineAuthorityValidator,
) error {
	if ctx == nil || admin == nil || len(commands) == 0 || validate == nil {
		return fmt.Errorf("%s transaction authority is incomplete", label)
	}
	tx, err := admin.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin %s: %w", label, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, command := range commands {
		if command == "" {
			return fmt.Errorf("%s contains an empty command", label)
		}
		if _, err := tx.Exec(ctx, command); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
	}
	if err := validate(ctx, tx); err != nil {
		return fmt.Errorf("validate %s before commit: %w", label, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit %s: %w", label, err)
	}
	return nil
}

func validateCreatedOfflineAuthorities(
	ctx context.Context,
	tx pgx.Tx,
	runtimeSchema string,
	hostSchema string,
	inferenceRole string,
	hostRole string,
) error {
	for _, schema := range []string{runtimeSchema, hostSchema} {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT to_regnamespace($1) IS NOT NULL`, schema).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("created schema %q is absent", schema)
		}
		for _, role := range []string{inferenceRole, hostRole} {
			var usage, create bool
			if err := tx.QueryRow(ctx,
				`SELECT has_schema_privilege($1,$2,'USAGE'), has_schema_privilege($1,$2,'CREATE')`,
				role, schema,
			).Scan(&usage, &create); err != nil {
				return err
			}
			if usage || create {
				return fmt.Errorf("new process role received premature schema authority")
			}
		}
	}
	for _, role := range []string{inferenceRole, hostRole} {
		var login, superuser, createDB, createRole, inherit, replication, bypassRLS bool
		var connections int
		if err := tx.QueryRow(ctx, `
			SELECT rolcanlogin, rolsuper, rolcreatedb, rolcreaterole,
			       rolinherit, rolreplication, rolbypassrls, rolconnlimit
			FROM pg_roles WHERE rolname=$1`, role,
		).Scan(
			&login, &superuser, &createDB, &createRole, &inherit,
			&replication, &bypassRLS, &connections,
		); err != nil {
			return err
		}
		if login || superuser || createDB || createRole || inherit || replication || bypassRLS ||
			connections != 8 {
			return fmt.Errorf("new process role %q has invalid authority", role)
		}
	}
	return nil
}
