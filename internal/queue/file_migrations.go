package queue

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const schemaMigrationsTableSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    filename TEXT PRIMARY KEY,
	sha256 TEXT NOT NULL,
	manifest_sha256 TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`

func (r *Repository) applyMigrationBundle(
	ctx context.Context,
	bundle MigrationBundle,
) (resultErr error) {
	if r == nil || r.pool == nil || ctx == nil {
		return fmt.Errorf("apply migration bundle requires PostgreSQL and context")
	}
	if err := bundle.validate(); err != nil {
		return err
	}

	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire file migration connection: %w", err)
	}
	if err := acquireFileMigrationLock(ctx, conn); err != nil {
		return errors.Join(err, destroyLockedConnection(conn))
	}
	defer func() {
		resultErr = errors.Join(resultErr, releaseFileMigrationLock(conn))
	}()
	return applyMigrationBundleOnLockedConnection(ctx, conn, bundle)
}

func applyMigrationBundleOnLockedConnection(
	ctx context.Context,
	conn *pgxpool.Conn,
	bundle MigrationBundle,
) error {
	if err := enforceMigrationSessionSQLMode(ctx, conn); err != nil {
		return err
	}
	if err := prepareFileMigrationLedger(ctx, conn, bundle); err != nil {
		return err
	}

	applied, err := loadAppliedFileMigrations(ctx, conn)
	if err != nil {
		return err
	}
	if err := validateAppliedFileMigrations(bundle, applied, true); err != nil {
		return err
	}

	for _, entry := range bundle.entries {
		if _, found := applied[entry.name]; found {
			continue
		}
		if err := applyFileMigration(ctx, conn, entry, bundle.manifestSHA256); err != nil {
			return err
		}
		log.Printf("schema migration applied: %s", entry.name)
	}
	return nil
}

func prepareFileMigrationLedger(
	ctx context.Context,
	conn *pgxpool.Conn,
	bundle MigrationBundle,
) error {
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin file migration ledger transaction: %w", err)
	}
	defer tx.Rollback(context.Background())
	if err := enforceMigrationTransactionSQLMode(ctx, tx); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, schemaMigrationsTableSQL); err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		ALTER TABLE schema_migrations ADD COLUMN IF NOT EXISTS sha256 TEXT;
		ALTER TABLE schema_migrations ADD COLUMN IF NOT EXISTS manifest_sha256 TEXT;
	`); err != nil {
		return fmt.Errorf("upgrade schema_migrations authority columns: %w", err)
	}
	if err := rejectUntrackedMigrationSchemaTx(ctx, tx); err != nil {
		return err
	}
	applied, err := loadAppliedFileMigrationsTx(ctx, tx)
	if err != nil {
		return err
	}
	if err := validateAppliedFileMigrations(bundle, applied, false); err != nil {
		return err
	}
	if err := normalizeAppliedMigrationAuthorityTx(ctx, tx, bundle, applied); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		ALTER TABLE schema_migrations ALTER COLUMN sha256 SET NOT NULL;
		ALTER TABLE schema_migrations ALTER COLUMN manifest_sha256 SET NOT NULL;
	`); err != nil {
		return fmt.Errorf("require schema_migrations authority columns: %w", err)
	}
	applied, err = loadAppliedFileMigrationsTx(ctx, tx)
	if err != nil {
		return err
	}
	if err := validateAppliedFileMigrations(bundle, applied, true); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit file migration ledger transaction: %w", err)
	}
	return nil
}

func rejectUntrackedMigrationSchemaTx(ctx context.Context, tx pgx.Tx) error {
	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		return fmt.Errorf("count applied file migrations: %w", err)
	}
	if count > 0 {
		return nil
	}

	var untracked bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_class relations
			JOIN pg_namespace namespaces ON namespaces.oid=relations.relnamespace
			WHERE namespaces.nspname=current_schema() AND
			      relations.relname<>'schema_migrations' AND
			      relations.relkind IN ('r','p','v','m','S','f')
		) OR EXISTS (
			SELECT 1 FROM pg_proc procedures
			JOIN pg_namespace namespaces ON namespaces.oid=procedures.pronamespace
			WHERE namespaces.nspname=current_schema()
		) OR EXISTS (
			SELECT 1 FROM pg_type types
			JOIN pg_namespace namespaces ON namespaces.oid=types.typnamespace
			WHERE namespaces.nspname=current_schema() AND types.typrelid=0 AND
			      NOT EXISTS (
				  SELECT 1 FROM pg_type elements
				  JOIN pg_class relations ON relations.reltype=elements.oid
				  JOIN pg_namespace relation_namespaces
				    ON relation_namespaces.oid=relations.relnamespace
				  WHERE types.typelem=elements.oid AND
				        relation_namespaces.nspname=current_schema() AND
				        relations.relname='schema_migrations'
			      )
		)
	`).Scan(&untracked); err != nil {
		return fmt.Errorf("inspect untracked migration schema: %w", err)
	}
	if untracked {
		return fmt.Errorf("runtime schema contains durable objects without a migration ledger")
	}
	return nil
}

type appliedMigration struct{ sha256, manifestSHA256 string }

func loadAppliedFileMigrations(
	ctx context.Context,
	conn *pgxpool.Conn,
) (map[string]appliedMigration, error) {
	return scanAppliedFileMigrations(ctx, conn)
}

func loadAppliedFileMigrationsTx(
	ctx context.Context,
	tx pgx.Tx,
) (map[string]appliedMigration, error) {
	return scanAppliedFileMigrations(ctx, tx)
}

type migrationRows interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func scanAppliedFileMigrations(
	ctx context.Context,
	querier migrationRows,
) (map[string]appliedMigration, error) {
	rows, err := querier.Query(ctx, `
		SELECT filename,COALESCE(sha256,''),COALESCE(manifest_sha256,'')
		FROM schema_migrations ORDER BY filename
	`)
	if err != nil {
		return nil, fmt.Errorf("load applied file migrations: %w", err)
	}
	defer rows.Close()

	out := map[string]appliedMigration{}
	for rows.Next() {
		var name string
		var row appliedMigration
		if err := rows.Scan(&name, &row.sha256, &row.manifestSHA256); err != nil {
			return nil, err
		}
		out[name] = row
	}
	return out, rows.Err()
}

func validateAppliedFileMigrations(
	bundle MigrationBundle,
	applied map[string]appliedMigration,
	requireCurrentManifest bool,
) error {
	registered := make(map[string]migrationBundleEntry, len(bundle.entries))
	for _, entry := range bundle.entries {
		registered[entry.name] = entry
	}
	for name := range applied {
		if _, found := registered[name]; !found {
			return fmt.Errorf("applied migration %q is absent from the registered bundle", name)
		}
	}
	missingLower := ""
	for _, entry := range bundle.entries {
		row, found := applied[entry.name]
		if !found {
			if missingLower == "" {
				missingLower = entry.name
			}
			continue
		}
		if missingLower != "" {
			return fmt.Errorf("applied migration %q follows missing registered migration %q", entry.name, missingLower)
		}
		if row.sha256 != entry.sha256 {
			return fmt.Errorf("applied migration %q body digest differs from the registered bundle", entry.name)
		}
		if !validMigrationDigest(row.manifestSHA256) {
			return fmt.Errorf("applied migration %q manifest identity is invalid", entry.name)
		}
		if requireCurrentManifest && row.manifestSHA256 != bundle.manifestSHA256 {
			return fmt.Errorf("applied migration %q is not bound to the current manifest", entry.name)
		}
	}
	return nil
}

func applyFileMigration(
	ctx context.Context,
	conn *pgxpool.Conn,
	entry migrationBundleEntry,
	manifestSHA256 string,
) error {
	body, err := entry.transactionalBody()
	if err != nil {
		return err
	}
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", entry.name, err)
	}
	defer tx.Rollback(context.Background())
	if err := enforceMigrationTransactionSQLMode(ctx, tx); err != nil {
		return fmt.Errorf("prepare migration %s SQL lexical mode: %w", entry.name, err)
	}

	if _, err := tx.Exec(ctx, string(body)); err != nil {
		return fmt.Errorf("apply migration %s: %w", entry.name, err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO schema_migrations (filename,sha256,manifest_sha256)
		VALUES ($1,$2,$3)
	`, entry.name, entry.sha256, manifestSHA256); err != nil {
		return fmt.Errorf("record migration %s: %w", entry.name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %s: %w", entry.name, err)
	}
	return nil
}
