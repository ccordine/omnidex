package queue

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLoadDatabaseSetupReadsOnlyOneExactCurrentStateFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, databaseSetupName)
	body := "-- Omnidex authoritative fresh-database setup.\n" +
		"CREATE FUNCTION current_schema_marker() RETURNS text " +
		"LANGUAGE sql SET search_path TO '__OMNIDEX_RUNTIME_SCHEMA__' " +
		"AS $$ SELECT current_schema() $$;\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	setup, err := LoadDatabaseSetup(path)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := setup.render("omnidex_test")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rendered), databaseSetupRuntimeSchemaSlot) ||
		!strings.Contains(string(rendered), "omnidex_test") {
		t.Fatalf("runtime schema slot was not resolved: %s", rendered)
	}

	symlink := filepath.Join(directory, "linked", databaseSetupName)
	if err := os.Mkdir(filepath.Dir(symlink), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDatabaseSetup(symlink); err == nil {
		t.Fatal("database setup symlink was accepted")
	}
}

func loadCurrentDatabaseSetup(t testing.TB) DatabaseSetup {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "database", databaseSetupName))
	if err != nil {
		t.Fatal(err)
	}
	setup, err := LoadDatabaseSetup(path)
	if err != nil {
		t.Fatal(err)
	}
	return setup
}

func openIsolatedDatabasePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL database tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("omnidex_database_%d", time.Now().UnixNano())
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
			t.Errorf("drop database authority schema: %v", err)
		}
		if _, err := admin.Exec(cleanupCtx,
			"DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE",
		); err != nil {
			t.Errorf("drop database test schema: %v", err)
		}
		admin.Close()
	})
	return pool
}
