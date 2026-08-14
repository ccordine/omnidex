package queue

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/db"
	"github.com/jackc/pgx/v5"
)

func TestPostgresLegacyPublicCatalogMatchesFrozenShape(t *testing.T) {
	fixture := openLegacyPublicFixture(t)
	tx, err := fixture.Pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(t.Context(), `SET LOCAL search_path TO public,pg_catalog`); err != nil {
		t.Fatal(err)
	}
	snapshot, err := loadLegacyCatalogSnapshot(t.Context(), tx, "public")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("legacy catalog sha256=%s shape=%d/%d/%d/%d/%d",
		snapshot.SHA256, snapshot.Tables, snapshot.Sequences, snapshot.Indexes,
		snapshot.Functions, snapshot.Triggers,
	)
	if snapshot.Tables != legacyExpectedTableCount ||
		snapshot.Sequences != legacyExpectedSequenceCount ||
		snapshot.Indexes != legacyExpectedIndexCount ||
		snapshot.Functions != legacyExpectedFunctionCount ||
		snapshot.Triggers != legacyExpectedTriggerCount {
		t.Fatalf("legacy shape=%+v differs from frozen audit", snapshot)
	}
	if snapshot.SHA256 != legacyExpectedCatalogSHA256 {
		t.Fatalf("legacy catalog sha256=%s want %s",
			snapshot.SHA256, legacyExpectedCatalogSHA256)
	}
	extensions, err := loadLegacyExtensions(t.Context(), tx)
	if err != nil {
		t.Fatal(err)
	}
	if got := legacyExtensionDescriptorSHA256(extensions); got != legacyExpectedExtensionSHA256 {
		t.Fatalf("legacy extension descriptors sha256=%s want %s",
			got, legacyExpectedExtensionSHA256)
	}
}

func TestPostgresLegacyPublicRuntimeCatalogMatchesFrozenAttestation(t *testing.T) {
	fixture := openLegacyPublicFixture(t)
	fixture.Pool.Close()
	pool := openLegacyCutoverPool(t, fixture.DatabaseURL)
	tx, err := pool.BeginTx(t.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err := acquireLegacyCutoverLocks(t.Context(), tx); err != nil {
		t.Fatal(err)
	}
	legacy, extensions, err := validateLegacyCutoverPreconditions(
		t.Context(), tx, db.DefaultRuntimeSchema, fixture.Bundle,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := renameLegacySchemaAndRestorePublic(
		t.Context(), tx, db.DefaultRuntimeSchema, extensions,
	); err != nil {
		t.Fatal(err)
	}
	if err := verifyRenamedLegacyInventory(
		t.Context(), tx, db.DefaultRuntimeSchema, legacy, extensions,
	); err != nil {
		t.Fatal(err)
	}
	if err := bindLegacyMigrationAuthority(t.Context(), tx, fixture.Bundle); err != nil {
		t.Fatal(err)
	}
	if err := applyPostLegacyMigrations(t.Context(), tx, fixture.Bundle); err != nil {
		t.Fatal(err)
	}
	snapshot, err := loadLegacyCatalogSnapshot(t.Context(), tx, db.DefaultRuntimeSchema)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf(
		"runtime catalog sha256=%s object_oids_sha256=%s shape=%d/%d/%d/%d/%d",
		snapshot.SHA256, snapshot.ObjectOIDsSHA256, snapshot.Tables,
		snapshot.Sequences, snapshot.Indexes, snapshot.Functions, snapshot.Triggers,
	)
	if snapshot.SHA256 != legacyExpectedRuntimeCatalogSHA256 {
		t.Fatalf(
			"runtime catalog sha256=%s want frozen %s",
			snapshot.SHA256, legacyExpectedRuntimeCatalogSHA256,
		)
	}
}
