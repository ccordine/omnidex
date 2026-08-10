package queue

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFileMigrationSQLAndLedgerRecordCommitAtomically(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repo := New(pool)
	dir := t.TempDir()
	writeMigrationTestFile(t, dir, "001_committed.sql", `
		CREATE TABLE committed_migration_probe (id BIGINT PRIMARY KEY);
	`)
	writeMigrationTestFile(t, dir, "002_rejected_record.sql", `
		CREATE TABLE rejected_migration_probe (id BIGINT PRIMARY KEY);
		ALTER TABLE schema_migrations
			ADD CONSTRAINT reject_002_ledger_record
			CHECK (filename <> '002_rejected_record.sql');
	`)
	bundle := loadMigrationTestBundle(t, dir)

	err := repo.applyMigrationBundle(context.Background(), bundle)
	if err == nil || !strings.Contains(err.Error(), "record migration 002_rejected_record.sql") {
		t.Fatalf("applyMigrationBundle error=%v, want rejected ledger record", err)
	}

	assertMigrationRelationExists(t, pool, "committed_migration_probe", true)
	assertMigrationRelationExists(t, pool, "rejected_migration_probe", false)
	assertAppliedMigrationCount(t, pool, "001_committed.sql", 1)
	assertAppliedMigrationCount(t, pool, "002_rejected_record.sql", 0)
}

func TestConcurrentFileMigrationRunnersAreSerialized(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	dir := t.TempDir()
	writeMigrationTestFile(t, dir, "001_serialized.sql", `
		CREATE TABLE serialized_migration_probe (id BIGINT PRIMARY KEY);
		SELECT pg_sleep(0.35);
	`)
	bundle := loadMigrationTestBundle(t, dir)

	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			ready.Done()
			<-start
			results <- New(pool).applyMigrationBundle(context.Background(), bundle)
		}()
	}
	ready.Wait()
	close(start)

	for range 2 {
		select {
		case err := <-results:
			if err != nil {
				t.Fatalf("concurrent applyMigrationBundle: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent migration runners did not finish")
		}
	}

	assertMigrationRelationExists(t, pool, "serialized_migration_probe", true)
	assertAppliedMigrationCount(t, pool, "001_serialized.sql", 1)
}

func TestFileMigrationRunnerFailsExplicitlyWhenLockWaitIsCanceled(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	guard, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := acquireFileMigrationLock(context.Background(), guard); err != nil {
		guard.Release()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := releaseFileMigrationLock(guard); err != nil {
			t.Errorf("release test migration lock: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	directory := t.TempDir()
	writeMigrationTestFile(t, directory, "001_lock.sql", "SELECT 1;\n")
	err = New(pool).applyMigrationBundle(ctx, loadMigrationTestBundle(t, directory))
	if err == nil || !strings.Contains(err.Error(), "acquire file migration advisory lock") {
		t.Fatalf("applyMigrationBundle error=%v, want explicit lock acquisition failure", err)
	}
}

func TestFileMigrationAppliesOnlyBytesSealedBeforeDatabaseLock(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	directory := t.TempDir()
	writeMigrationTestFile(t, directory, "001_sealed.sql", `
		CREATE TABLE sealed_before_lock_probe (id BIGINT PRIMARY KEY);
	`)
	bundle := loadMigrationTestBundle(t, directory)
	writeMigrationTestFile(t, directory, "001_sealed.sql", `
		CREATE TABLE substituted_after_load_probe (id BIGINT PRIMARY KEY);
	`)
	if err := New(pool).applyMigrationBundle(context.Background(), bundle); err != nil {
		t.Fatal(err)
	}
	assertMigrationRelationExists(t, pool, "sealed_before_lock_probe", true)
	assertMigrationRelationExists(t, pool, "substituted_after_load_probe", false)
}

func loadMigrationTestBundle(t *testing.T, directory string) MigrationBundle {
	t.Helper()
	names := make([]string, 0)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if migrationFileNamePattern.MatchString(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	digest := rewriteTestMigrationManifest(t, directory, names)
	bundle, err := LoadMigrationBundle(directory, digest)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func openIsolatedMigrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL migration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `
		CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;
		CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;
		CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;
	`); err != nil {
		admin.Close()
		t.Fatalf("install isolated migration test extensions: %v", err)
	}
	schema := fmt.Sprintf("file_migrations_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	var authoritySchema string
	if err := admin.QueryRow(ctx, `SELECT 'omnidex_host_authority_' || md5($1)`, schema).Scan(
		&authoritySchema,
	); err != nil {
		admin.Close()
		t.Fatal(err)
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		admin.Close()
		t.Fatal(err)
	}
	config.MaxConns = 4
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx,
			"DROP SCHEMA IF EXISTS "+pgx.Identifier{authoritySchema}.Sanitize()+" CASCADE",
		); err != nil {
			t.Errorf("drop migration authority schema: %v", err)
		}
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			t.Errorf("drop migration test schema: %v", err)
		}
		admin.Close()
	})
	return pool
}

func writeMigrationTestFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertMigrationRelationExists(t *testing.T, pool *pgxpool.Pool, name string, want bool) {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM pg_class relations
			JOIN pg_namespace namespaces ON namespaces.oid=relations.relnamespace
			WHERE namespaces.nspname=current_schema() AND relations.relname=$1
		)
	`, name).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists != want {
		t.Fatalf("relation %q exists=%t want %t", name, exists, want)
	}
}

func assertAppliedMigrationCount(t *testing.T, pool *pgxpool.Pool, name string, want int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM schema_migrations WHERE filename=$1
	`, name).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("schema_migrations count for %q=%d want %d", name, count, want)
	}
}
