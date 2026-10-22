package datasource

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestEvidenceQueryContinuesPastServerStatementTimeout(t *testing.T) {
	url := os.Getenv("OMNI_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	config, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.RuntimeParams["statement_timeout"] = "1000ms"
	config.RuntimeParams["lock_timeout"] = "1000ms"
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := setExecutionTimeouts(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "SELECT pg_sleep(1.1)"); err != nil {
		t.Fatalf("active query aborted after one second: %v", err)
	}
	var statement, lock string
	if err := tx.QueryRow(ctx, "SELECT current_setting('statement_timeout'), current_setting('lock_timeout')").Scan(&statement, &lock); err != nil {
		t.Fatal(err)
	}
	if statement != "0" || lock != "0" {
		t.Fatalf("query deadlines remained: statement=%s lock=%s", statement, lock)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, "SELECT current_setting('statement_timeout')").Scan(&statement); err != nil {
		t.Fatal(err)
	}
	if statement != "1s" {
		t.Fatalf("query changed session configuration outside its transaction: %s", statement)
	}
}
