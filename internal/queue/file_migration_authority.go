package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func normalizeAppliedMigrationAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	bundle MigrationBundle,
	applied map[string]appliedMigration,
) error {
	var normalizedAuthority bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_class relations
			JOIN pg_namespace namespaces ON namespaces.oid=relations.relnamespace
			WHERE namespaces.nspname=current_schema() AND
			      relations.relname='schema_migration_bundle_authority'
		)
	`).Scan(&normalizedAuthority); err != nil {
		return fmt.Errorf("inspect normalized migration authority: %w", err)
	}
	if normalizedAuthority {
		result, err := tx.Exec(ctx, `
			UPDATE schema_migration_bundle_authority
			SET manifest_sha256=$1
			WHERE singleton AND manifest_sha256 IS DISTINCT FROM $1
		`, bundle.manifestSHA256)
		if err != nil {
			return fmt.Errorf("advance normalized migration authority: %w", err)
		}
		if result.RowsAffected() > 1 {
			return fmt.Errorf("normalized migration authority has multiple singleton rows")
		}
		var count int
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM schema_migration_bundle_authority
			WHERE singleton AND manifest_sha256=$1
		`, bundle.manifestSHA256).Scan(&count); err != nil || count != 1 {
			return fmt.Errorf("normalized migration authority is unavailable after advance")
		}
		return nil
	}

	for _, entry := range bundle.entries {
		if _, found := applied[entry.name]; !found {
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE schema_migrations
			SET sha256=$2,manifest_sha256=$3
			WHERE filename=$1 AND (
				sha256 IS DISTINCT FROM $2 OR manifest_sha256 IS DISTINCT FROM $3
			)
		`, entry.name, entry.sha256, bundle.manifestSHA256); err != nil {
			return fmt.Errorf("normalize migration authority for %s: %w", entry.name, err)
		}
	}
	return nil
}
