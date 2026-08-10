package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func (r *Repository) MigrateFresh(
	ctx context.Context,
	bundle MigrationBundle,
) (resultErr error) {
	if ctx == nil || r == nil || r.pool == nil {
		return fmt.Errorf("migrate fresh requires PostgreSQL and context")
	}
	if err := bundle.validate(); err != nil {
		return err
	}
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire fresh migration connection: %w", err)
	}
	if err := acquireFileMigrationLock(ctx, conn); err != nil {
		return errors.Join(err, destroyLockedConnection(conn))
	}
	locked := true
	defer func() {
		if locked {
			resultErr = errors.Join(resultErr, releaseFileMigrationLock(conn))
		}
	}()

	if err := resetExactRuntimeSchema(ctx, conn); err != nil {
		return err
	}
	if err := applyMigrationBundleOnLockedConnection(ctx, conn, bundle); err != nil {
		return err
	}
	if err := releaseFileMigrationLock(conn); err != nil {
		locked = false
		return err
	}
	locked = false
	return nil
}

func resetExactRuntimeSchema(ctx context.Context, conn *pgxpool.Conn) error {
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin exact runtime schema reset: %w", err)
	}
	defer tx.Rollback(context.Background())

	runtimeSchema, owner, err := loadResettableRuntimeSchema(ctx, tx)
	if err != nil {
		return err
	}
	if err := rejectRuntimeSchemaExtensions(ctx, tx, runtimeSchema); err != nil {
		return err
	}
	if err := dropExactFenceAuthoritySchema(ctx, tx, runtimeSchema); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DROP SCHEMA `+
		pgx.Identifier{runtimeSchema}.Sanitize()+` CASCADE`); err != nil {
		return fmt.Errorf("drop exact runtime schema: %w", err)
	}
	if _, err := tx.Exec(ctx, `CREATE SCHEMA `+
		pgx.Identifier{runtimeSchema}.Sanitize()+` AUTHORIZATION `+
		pgx.Identifier{owner}.Sanitize()); err != nil {
		return fmt.Errorf("recreate exact runtime schema: %w", err)
	}
	var restored string
	if err := tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&restored); err != nil {
		return fmt.Errorf("verify recreated runtime schema: %w", err)
	}
	if restored != runtimeSchema {
		return fmt.Errorf("recreated runtime schema is not the active schema")
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit exact runtime schema reset: %w", err)
	}
	return nil
}

func loadResettableRuntimeSchema(
	ctx context.Context,
	tx pgx.Tx,
) (string, string, error) {
	var schema, owner string
	var owned bool
	if err := tx.QueryRow(ctx, `
		SELECT namespaces.nspname,roles.rolname,namespaces.nspowner=roles.oid
		FROM pg_namespace namespaces
		JOIN pg_roles roles ON roles.rolname=current_user
		WHERE namespaces.nspname=current_schema()
	`).Scan(&schema, &owner, &owned); err != nil {
		return "", "", fmt.Errorf("load exact runtime schema authority: %w", err)
	}
	if err := validateResettableRuntimeSchema(schema, owned); err != nil {
		return "", "", err
	}
	return schema, owner, nil
}

func validateResettableRuntimeSchema(schema string, owned bool) error {
	if !owned || schema == "" || schema == "public" || schema == "information_schema" ||
		strings.HasPrefix(schema, "pg_") {
		return fmt.Errorf("fresh migration refuses non-owned or broad runtime schema %q", schema)
	}
	return nil
}

func rejectRuntimeSchemaExtensions(
	ctx context.Context,
	tx pgx.Tx,
	runtimeSchema string,
) error {
	var count int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_extension extensions
		JOIN pg_namespace namespaces ON namespaces.oid=extensions.extnamespace
		WHERE namespaces.nspname=$1
	`, runtimeSchema).Scan(&count); err != nil {
		return fmt.Errorf("inspect runtime schema extensions: %w", err)
	}
	if count != 0 {
		return fmt.Errorf("fresh migration refuses runtime schema containing extensions")
	}
	return nil
}

func dropExactFenceAuthoritySchema(
	ctx context.Context,
	tx pgx.Tx,
	runtimeSchema string,
) error {
	var registryExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_class relations
			JOIN pg_namespace namespaces ON namespaces.oid=relations.relnamespace
			WHERE namespaces.nspname=$1 AND
			      relations.relname='step_attempt_transaction_fence_authority' AND
			      relations.relkind='r'
		)
	`, runtimeSchema).Scan(&registryExists); err != nil {
		return fmt.Errorf("inspect step-attempt fence registry: %w", err)
	}
	var authoritySchema string
	if err := tx.QueryRow(ctx, `SELECT 'omnidex_host_authority_' || md5($1)`,
		runtimeSchema,
	).Scan(&authoritySchema); err != nil {
		return fmt.Errorf("derive exact step-attempt fence schema: %w", err)
	}
	if registryExists {
		registered, err := loadInstalledStepAttemptFenceTx(ctx, tx)
		if err != nil {
			return err
		}
		if registered != authoritySchema {
			return fmt.Errorf("fresh migration refuses changed step-attempt fence schema authority")
		}
	}
	var exists, owned bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname=$1),
		       EXISTS (
			   SELECT 1 FROM pg_namespace namespaces
			   WHERE namespaces.nspname=$1 AND namespaces.nspowner=(
				   SELECT oid FROM pg_roles WHERE rolname=current_user
			   )
		       )
	`, authoritySchema).Scan(&exists, &owned); err != nil {
		return fmt.Errorf("inspect exact step-attempt fence schema: %w", err)
	}
	if !exists {
		if registryExists {
			return fmt.Errorf("fresh migration requires the registered step-attempt fence schema")
		}
		return nil
	}
	if !registryExists {
		return fmt.Errorf("fresh migration refuses unregistered step-attempt fence schema")
	}
	if !owned {
		return fmt.Errorf("fresh migration refuses non-owned step-attempt fence schema")
	}
	if _, err := tx.Exec(ctx, `DROP SCHEMA `+
		pgx.Identifier{authoritySchema}.Sanitize()+` CASCADE`); err != nil {
		return fmt.Errorf("drop exact step-attempt fence schema: %w", err)
	}
	return nil
}
