package api

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func openIsolatedAPIMigrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL API tests")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	schema := fmt.Sprintf("api_test_%d", time.Now().UnixNano())
	pool, err := db.ConnectRuntime(ctx, databaseURL, schema)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		pool.Close()
		t.Fatal(err)
	}
	var authoritySchema string
	if err := admin.QueryRow(ctx, `SELECT 'omnidex_host_authority_' || md5($1)`, schema).Scan(
		&authoritySchema,
	); err != nil {
		pool.Close()
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
			t.Errorf("drop API migration authority schema: %v", err)
		}
		if _, err := admin.Exec(cleanupCtx,
			"DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE",
		); err != nil {
			t.Errorf("drop API migration test schema: %v", err)
		}
		admin.Close()
	})
	return pool
}
