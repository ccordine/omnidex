package queue

import (
	"context"
	"strings"
	"testing"
)

func TestPostgresMigrationAuthorityNormalizesOnlyExactHistoricalPrefix(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	through058 := loadMigrationBundleThroughPrefix(t, "058")
	if err := repository.applyMigrationBundle(context.Background(), through058); err != nil {
		t.Fatal(err)
	}
	forgedManifest := strings.Repeat("a", 64)
	if _, err := pool.Exec(context.Background(), `
		UPDATE schema_migrations SET manifest_sha256=$1
	`, forgedManifest); err != nil {
		t.Fatal(err)
	}
	through059 := loadMigrationBundleThroughPrefix(t, "059")
	if err := repository.applyMigrationBundle(context.Background(), through059); err != nil {
		t.Fatal(err)
	}
	assertExactMigrationLedger(t, pool, through059)
}

func TestPostgresMigrationInstallerRejectsHigherAppliedEntryAfterGap(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	body1 := []byte("SELECT 1;\n")
	body2 := []byte("SELECT 2;\n")
	bundle := migrationBundleForValidationTest(body1, body2)
	if _, err := pool.Exec(context.Background(), schemaMigrationsTableSQL); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO schema_migrations(filename,sha256,manifest_sha256)
		VALUES ('002_next.sql',$1,$2)
	`, digestMigrationBytes(body2), bundle.ManifestSHA256()); err != nil {
		t.Fatal(err)
	}
	err := New(pool).applyMigrationBundle(context.Background(), bundle)
	if err == nil || !strings.Contains(err.Error(), "follows missing registered migration") {
		t.Fatalf("applyMigrationBundle error=%v, want missing-lower rejection", err)
	}
}

func TestPostgresMigrationInstallerRejectsLegacyFilenameOnlyLedger(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	body := []byte("SELECT 1;\n")
	bundle := migrationBundleForValidationTest(body)
	if _, err := pool.Exec(context.Background(), `
		CREATE TABLE schema_migrations (
			filename TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		INSERT INTO schema_migrations(filename) VALUES ('001_first.sql');
	`); err != nil {
		t.Fatal(err)
	}
	err := New(pool).applyMigrationBundle(context.Background(), bundle)
	if err == nil || !strings.Contains(err.Error(), "body digest differs") {
		t.Fatalf("applyMigrationBundle error=%v, want incomplete legacy-ledger rejection", err)
	}
}

func TestPostgresNormalizedMigrationHistoryRejectsPostCommitMutation(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	bundle := loadMigrationBundleThroughPrefix(t, "059")
	if err := New(pool).applyMigrationBundle(context.Background(), bundle); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`UPDATE schema_migrations SET sha256=repeat('a',64) WHERE filename='001_init.sql'`,
		`UPDATE schema_migrations SET filename='001_changed.sql' WHERE filename='001_init.sql'`,
		`UPDATE schema_migrations SET manifest_sha256=repeat('a',64)`,
		`DELETE FROM schema_migrations WHERE filename='001_init.sql'`,
		`TRUNCATE schema_migrations`,
		`DELETE FROM schema_migration_bundle_authority`,
	} {
		if _, err := pool.Exec(context.Background(), statement); err == nil {
			t.Fatalf("normalized migration history accepted mutation: %s", statement)
		}
	}
	assertExactMigrationLedger(t, pool, bundle)
}
