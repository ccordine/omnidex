package db_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gryph/omnidex/database"
	"github.com/gryph/omnidex/internal/db"
	"github.com/jackc/pgx/v5"
)

func TestRuntimeStartupPreservesPriorInternalSchemaAndRows(t *testing.T) {
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for preserving startup coverage")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	schema := fmt.Sprintf("omnidex_startup_test_%d", time.Now().UnixNano())
	pool, err := db.ConnectRuntime(ctx, databaseURL, schema, database.SetupSQL())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanup, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
		pool.Close()
	}()
	if _, err := pool.Exec(ctx, `
		CREATE TABLE obsolete_layout (value text);
		INSERT INTO obsolete_layout VALUES ('old internal data');
		INSERT INTO projects (location,name,description)
		VALUES ('/fixture/startup-test-project','previous run','retain on startup');
	`); err != nil {
		t.Fatal(err)
	}
	resumed, err := db.ConnectRuntime(ctx, databaseURL, schema, database.SetupSQL())
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	var projects int
	var obsolete *string
	if err := resumed.QueryRow(ctx, `SELECT count(*) FROM projects`).Scan(&projects); err != nil {
		t.Fatal(err)
	}
	if err := resumed.QueryRow(ctx, `SELECT to_regclass($1)::text`, schema+".obsolete_layout").Scan(&obsolete); err != nil {
		t.Fatal(err)
	}
	if projects != 1 || obsolete == nil {
		t.Fatalf("startup lost internal state: projects=%d retained_table=%v", projects, obsolete)
	}
	var value string
	if err := resumed.QueryRow(ctx, `SELECT value FROM obsolete_layout`).Scan(&value); err != nil || value != "old internal data" {
		t.Fatalf("restart changed retained data: %q, %v", value, err)
	}
}
