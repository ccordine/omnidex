package omni

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresQueryRunnerEnforcesTransactionReadOnly(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL query-runner tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("query_runner_read_only_%d", time.Now().UnixNano())
	qualifiedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+qualifiedSchema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+qualifiedSchema+" CASCADE"); err != nil {
			t.Errorf("drop query-runner test schema: %v", err)
		}
		admin.Close()
	})
	if _, err := admin.Exec(ctx, "CREATE TABLE "+qualifiedSchema+".read_only_probe (value TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 4
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	runner := NewPgxMemoryRunner(pool)

	rows, err := runner.Query(ctx, "SELECT 7 AS value")
	if err != nil {
		t.Fatalf("read query: %v", err)
	}
	if len(rows) != 1 || rows[0]["value"] != int32(7) {
		t.Fatalf("read rows=%#v, want one committed result", rows)
	}
	if acquired := pool.Stat().AcquiredConns(); acquired != 0 {
		t.Fatalf("acquired connections after committed read=%d, want 0", acquired)
	}

	dataModifyingQuery := `
		WITH inserted AS(INSERT INTO read_only_probe (value) VALUES ('forbidden')
			RETURNING value
		)
		SELECT value FROM inserted
	`
	if err := ValidateReadOnlyPostgresQuery(dataModifyingQuery); err != nil {
		t.Fatalf("mutation fixture must pass the lexical boundary: %v", err)
	}
	_, err = runner.Query(ctx, dataModifyingQuery)
	if err == nil {
		t.Fatal("data-modifying CTE unexpectedly succeeded through read-only query runner")
	}
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "25006" {
		t.Fatalf("data-modifying CTE error=%v, want PostgreSQL read-only transaction rejection", err)
	}
	if acquired := pool.Stat().AcquiredConns(); acquired != 0 {
		t.Fatalf("acquired connections after rejected write=%d, want 0", acquired)
	}
	var count int
	if err := admin.QueryRow(ctx, "SELECT COUNT(*) FROM "+qualifiedSchema+".read_only_probe").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("read-only query runner persisted %d rows, want 0", count)
	}
}
