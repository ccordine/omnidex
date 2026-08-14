package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const legacyCutoverFinalMigrationPrefix = 88

// PreserveLegacyPublic performs the sole authorized public-to-runtime durable
// state transition. It never copies or resets data. Every check, schema rename,
// migration, receipt write, and postcondition occurs in one serializable
// transaction under the two migration locks in their fixed global order.
func PreserveLegacyPublic(
	ctx context.Context,
	pool *pgxpool.Pool,
	runtimeSchema string,
	bundle MigrationBundle,
) (LegacyPublicCutoverReceipt, error) {
	if ctx == nil || pool == nil {
		return LegacyPublicCutoverReceipt{}, fmt.Errorf("legacy public cutover requires PostgreSQL and context")
	}
	if err := db.ValidateRuntimeSchemaName(runtimeSchema); err != nil {
		return LegacyPublicCutoverReceipt{}, err
	}
	if err := validateLegacyCutoverBundle(bundle); err != nil {
		return LegacyPublicCutoverReceipt{}, err
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return LegacyPublicCutoverReceipt{}, fmt.Errorf("acquire legacy public cutover connection: %w", err)
	}
	defer conn.Release()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return LegacyPublicCutoverReceipt{}, fmt.Errorf("begin serializable legacy public cutover: %w", err)
	}
	defer tx.Rollback(context.Background())
	if err := acquireLegacyCutoverLocks(ctx, tx); err != nil {
		return LegacyPublicCutoverReceipt{}, err
	}
	legacyExists, runtimeExists, err := loadLegacyCutoverState(ctx, tx, runtimeSchema)
	if err != nil {
		return LegacyPublicCutoverReceipt{}, err
	}
	if runtimeExists || !legacyExists {
		receipt, err := verifyCompletedLegacyCutover(
			ctx, tx, runtimeSchema, bundle, legacyExists, runtimeExists,
		)
		if err != nil {
			return LegacyPublicCutoverReceipt{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return LegacyPublicCutoverReceipt{}, fmt.Errorf("commit verified legacy cutover retry: %w", err)
		}
		return receipt, nil
	}
	legacyCatalog, extensions, err := validateLegacyCutoverPreconditions(
		ctx, tx, runtimeSchema, bundle,
	)
	if err != nil {
		return LegacyPublicCutoverReceipt{}, err
	}
	runtimeOID, err := renameLegacySchemaAndRestorePublic(
		ctx, tx, runtimeSchema, extensions,
	)
	if err != nil {
		return LegacyPublicCutoverReceipt{}, err
	}
	if err := verifyRenamedLegacyInventory(
		ctx, tx, runtimeSchema, legacyCatalog, extensions,
	); err != nil {
		return LegacyPublicCutoverReceipt{}, err
	}
	if err := bindLegacyMigrationAuthority(ctx, tx, bundle); err != nil {
		return LegacyPublicCutoverReceipt{}, err
	}
	if err := applyPostLegacyMigrations(ctx, tx, bundle); err != nil {
		return LegacyPublicCutoverReceipt{}, err
	}
	finalCatalog, err := loadLegacyCatalogSnapshot(ctx, tx, runtimeSchema)
	if err != nil {
		return LegacyPublicCutoverReceipt{}, err
	}
	if finalCatalog.SHA256 != legacyExpectedRuntimeCatalogSHA256 {
		return LegacyPublicCutoverReceipt{}, fmt.Errorf(
			"final runtime catalog differs from the frozen release attestation: sha256=%s",
			finalCatalog.SHA256,
		)
	}
	finalExtensions, err := loadLegacyExtensions(ctx, tx)
	if err != nil {
		return LegacyPublicCutoverReceipt{}, err
	}
	receipt := LegacyPublicCutoverReceipt{
		Schema: legacyPublicCutoverReceiptSchema, RuntimeSchema: runtimeSchema,
		MigrationManifestSHA256: bundle.manifestSHA256,
		LegacyCatalogSHA256:     legacyCatalog.SHA256,
		RuntimeCatalogSHA256:    finalCatalog.SHA256,
		ObjectOIDsSHA256:        finalCatalog.ObjectOIDsSHA256,
		ExtensionsSHA256:        legacyExtensionSHA256(finalExtensions),
		RuntimeSchemaOID:        runtimeOID, MigrationCount: len(bundle.entries),
	}
	if err := receipt.validate(bundle, runtimeSchema); err != nil {
		return LegacyPublicCutoverReceipt{}, err
	}
	if err := installLegacyCutoverReceipt(ctx, tx, runtimeSchema, receipt); err != nil {
		return LegacyPublicCutoverReceipt{}, err
	}
	if err := verifyFinalLegacyCutoverState(ctx, tx, runtimeSchema, bundle, receipt); err != nil {
		return LegacyPublicCutoverReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LegacyPublicCutoverReceipt{}, fmt.Errorf("commit legacy public cutover: %w", err)
	}
	return receipt, nil
}

func validateLegacyCutoverBundle(bundle MigrationBundle) error {
	if err := bundle.validate(); err != nil {
		return err
	}
	if _, err := legacyMigrationEntries(bundle); err != nil {
		return err
	}
	if bundle.manifestSHA256 != legacyExpectedMigrationManifestSHA256 {
		return fmt.Errorf("legacy cutover release migration manifest differs from its frozen authority")
	}
	lastPrefix, err := migrationNumericPrefix(bundle.entries[len(bundle.entries)-1].name)
	if err != nil || lastPrefix != legacyCutoverFinalMigrationPrefix {
		return fmt.Errorf("legacy cutover requires the exact sealed 001..088 migration bundle")
	}
	return nil
}

func acquireLegacyCutoverLocks(ctx context.Context, tx pgx.Tx) error {
	for _, lockID := range []int64{legacyRuntimeBootstrapLockID, fileMigrationAdvisoryLockID} {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lockID); err != nil {
			return fmt.Errorf("acquire fixed-order legacy cutover lock %x: %w", lockID, err)
		}
	}
	return nil
}

func loadLegacyCutoverState(
	ctx context.Context,
	tx pgx.Tx,
	runtimeSchema string,
) (bool, bool, error) {
	var legacyExists, runtimeExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		 SELECT 1 FROM pg_class objects JOIN pg_namespace namespaces
		 ON namespaces.oid=objects.relnamespace
		 WHERE namespaces.nspname='public' AND objects.relname='schema_migrations' AND
		       objects.relkind='r'
		),EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)
	`, runtimeSchema).Scan(&legacyExists, &runtimeExists); err != nil {
		return false, false, fmt.Errorf("inspect legacy cutover state: %w", err)
	}
	return legacyExists, runtimeExists, nil
}
