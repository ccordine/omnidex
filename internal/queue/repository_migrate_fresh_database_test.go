package queue

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresMigrationInstallerRejectsPublicExtensionObjectsWithoutLedger(t *testing.T) {
	databaseURL := freshRuntimeDatabaseURL(
		t,
		strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL")),
	)
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL migration tests")
	}
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(t.Context(), `
		CREATE EXTENSION vector WITH SCHEMA public;
		CREATE EXTENSION pg_trgm WITH SCHEMA public;
		CREATE EXTENSION pgcrypto WITH SCHEMA public;
	`); err != nil {
		t.Fatal(err)
	}

	err = New(pool).EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "059"))
	if err == nil || !strings.Contains(err.Error(), "without a migration ledger") {
		t.Fatalf("EnsureSchema error=%v, want public extension-state rejection", err)
	}
	assertMigrationRelationExists(t, pool, "schema_migrations", false)
}

func TestPostgresEnsureSchemaRejectsUntrackedRuntimeObjects(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	if _, err := pool.Exec(context.Background(), `CREATE TABLE untracked_probe (id BIGINT)`); err != nil {
		t.Fatal(err)
	}
	bundle := loadMigrationBundleThroughPrefix(t, "059")
	err := New(pool).EnsureSchema(context.Background(), bundle)
	if err == nil || !strings.Contains(err.Error(), "without a migration ledger") {
		t.Fatalf("EnsureSchema error=%v, want untracked schema rejection", err)
	}
	assertMigrationRelationExists(t, pool, "untracked_probe", true)
	assertMigrationRelationExists(t, pool, "schema_migrations", false)
}

func TestPostgresMigrateFreshResetsAllExactSchemaObjectsAndReinstalls(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repo := New(pool)
	bundle := loadMigrationBundleThroughPrefix(t, "059")
	if err := repo.EnsureSchema(context.Background(), bundle); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		CREATE TYPE fresh_probe_type AS ENUM ('old');
		CREATE SEQUENCE fresh_probe_sequence;
		CREATE FUNCTION fresh_probe_function() RETURNS integer
		LANGUAGE SQL IMMUTABLE AS 'SELECT 1';
		CREATE VIEW fresh_probe_view AS SELECT fresh_probe_function() AS value;
	`); err != nil {
		t.Fatal(err)
	}

	for cycle := 1; cycle <= 2; cycle++ {
		if err := repo.MigrateFresh(context.Background(), bundle); err != nil {
			t.Fatalf("MigrateFresh cycle %d: %v", cycle, err)
		}
		assertExactMigrationLedger(t, pool, bundle)
		for _, relation := range []string{
			"jobs", "claims", "claim_support", "ai_channels", "ai_channel_messages",
			"ui_sessions", "data_source_channels", "data_source_channel_messages",
		} {
			assertMigrationRelationExists(t, pool, relation, true)
		}
		assertMigrationRelationExists(t, pool, "fresh_probe_view", false)
		assertRuntimeObjectAbsent(t, pool, "fresh_probe_type", "fresh_probe_sequence", "fresh_probe_function")
	}
}

func TestPostgresMigrateFreshRefusesUnregisteredAuthoritySchema(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	bundle := loadMigrationBundleThroughPrefix(t, "059")
	var authoritySchema string
	if err := pool.QueryRow(context.Background(),
		`SELECT 'omnidex_host_authority_'||md5(current_schema())`,
	).Scan(&authoritySchema); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `CREATE SCHEMA `+
		pgx.Identifier{authoritySchema}.Sanitize()+`; CREATE TABLE `+
		pgx.Identifier{authoritySchema, "unregistered_probe"}.Sanitize()+` (id BIGINT)`,
	); err != nil {
		t.Fatal(err)
	}

	err := repository.MigrateFresh(context.Background(), bundle)
	if err == nil || !strings.Contains(err.Error(), "unregistered step-attempt fence schema") {
		t.Fatalf("MigrateFresh error=%v, want unregistered authority rejection", err)
	}
	var retained bool
	if err := pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM pg_class relations
			JOIN pg_namespace namespaces ON namespaces.oid=relations.relnamespace
			WHERE namespaces.nspname=$1 AND relations.relname='unregistered_probe'
		)
	`, authoritySchema).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if !retained {
		t.Fatal("fresh migration deleted an unregistered authority schema")
	}
}

func assertExactMigrationLedger(t *testing.T, pool *pgxpool.Pool, bundle MigrationBundle) {
	t.Helper()
	var count int
	var distinctManifest int
	var manifest string
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*),COUNT(DISTINCT manifest_sha256),MIN(manifest_sha256)
		FROM schema_migrations
	`).Scan(&count, &distinctManifest, &manifest); err != nil {
		t.Fatal(err)
	}
	if count != len(bundle.entries) || distinctManifest != 1 || manifest != bundle.ManifestSHA256() {
		t.Fatalf("migration ledger=(%d,%d,%s), want (%d,1,%s)",
			count, distinctManifest, manifest, len(bundle.entries), bundle.ManifestSHA256())
	}
}

func assertRuntimeObjectAbsent(t *testing.T, pool *pgxpool.Pool, names ...string) {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `
		SELECT
			(SELECT COUNT(*) FROM pg_type types JOIN pg_namespace n ON n.oid=types.typnamespace
			 WHERE n.nspname=current_schema() AND types.typname=$1) +
			(SELECT COUNT(*) FROM pg_class relations JOIN pg_namespace n ON n.oid=relations.relnamespace
			 WHERE n.nspname=current_schema() AND relations.relname=$2) +
			(SELECT COUNT(*) FROM pg_proc procedures JOIN pg_namespace n ON n.oid=procedures.pronamespace
			 WHERE n.nspname=current_schema() AND procedures.proname=$3)
	`, names[0], names[1], names[2]).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("fresh migration left %d non-table runtime objects", count)
	}
}
