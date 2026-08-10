package queue

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresShippedRuntimeSchemaInstallsAndMigratesFresh(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	databaseURL := freshRuntimeDatabaseURL(t, baseURL)
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL runtime schema tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	schema := fmt.Sprintf("omnidex_runtime_test_%d", time.Now().UnixNano())
	pool, err := db.ConnectRuntime(ctx, databaseURL, schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	bundle := loadMigrationBundleThroughPrefix(t, "059")
	repo := New(pool)
	if err := repo.EnsureSchema(ctx, bundle); err != nil {
		t.Fatal(err)
	}
	assertSharedExtensionsUsePublic(t, pool)
	if err := repo.MigrateFresh(ctx, bundle); err != nil {
		t.Fatal(err)
	}
	assertExactMigrationLedger(t, pool, bundle)
	assertSharedExtensionsUsePublic(t, pool)
}

func TestPostgresRuntimeBootstrapRejectsLegacyPublicDataWithoutCreatingSchema(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	databaseURL := freshRuntimeDatabaseURL(t, baseURL)
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL runtime schema tests")
	}
	admin, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	if _, err := admin.Exec(context.Background(), `
		CREATE TABLE public.jobs (id BIGINT PRIMARY KEY);
		CREATE TABLE public.job_steps (id BIGINT PRIMARY KEY);
	`); err != nil {
		t.Fatal(err)
	}
	runtimeSchema := fmt.Sprintf("omnidex_runtime_test_%d", time.Now().UnixNano())
	if _, err := db.ConnectRuntime(context.Background(), databaseURL, runtimeSchema); err == nil ||
		!strings.Contains(err.Error(), "legacy Omnidex state") {
		t.Fatalf("ConnectRuntime error=%v, want legacy public-state rejection", err)
	}
	var exists bool
	if err := admin.QueryRow(context.Background(), `
		SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname=$1)
	`, runtimeSchema).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("runtime schema was created beside legacy public Omnidex data")
	}
}

func freshRuntimeDatabaseURL(t *testing.T, databaseURL string) string {
	t.Helper()
	if databaseURL == "" {
		return ""
	}
	admin, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	database := fmt.Sprintf("omnidex_runtime_install_%d", time.Now().UnixNano())
	if _, err := admin.Exec(context.Background(), `CREATE DATABASE `+
		pgx.Identifier{database}.Sanitize()+` TEMPLATE template0`); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		admin.Close()
		t.Fatal(err)
	}
	parsed.Path = "/" + database
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), `DROP DATABASE `+
			pgx.Identifier{database}.Sanitize()+` WITH (FORCE)`); err != nil {
			t.Errorf("drop fresh runtime database: %v", err)
		}
		admin.Close()
	})
	return parsed.String()
}

func assertSharedExtensionsUsePublic(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT extensions.extname,namespaces.nspname
		FROM pg_extension extensions
		JOIN pg_namespace namespaces ON namespaces.oid=extensions.extnamespace
		WHERE extensions.extname IN ('vector','pg_trgm','pgcrypto')
		ORDER BY extensions.extname
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var name, schema string
		if err := rows.Scan(&name, &schema); err != nil {
			t.Fatal(err)
		}
		if schema != "public" {
			t.Fatalf("extension %s installed in %s, want public", name, schema)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("shared extension count=%d want 3", count)
	}
}
