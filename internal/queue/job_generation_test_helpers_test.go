package queue

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func expectGenerationDatabaseFailure(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	statement string,
	arguments ...any,
) {
	t.Helper()
	const savepoint = "generation_expected_failure"
	if _, err := tx.Exec(ctx, "SAVEPOINT "+savepoint); err != nil {
		t.Fatal(err)
	}
	_, operationErr := tx.Exec(ctx, statement, arguments...)
	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT "+savepoint); err != nil {
		t.Fatalf("recover expected PostgreSQL failure: %v (operation error: %v)", err, operationErr)
	}
	if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT "+savepoint); err != nil {
		t.Fatal(err)
	}
	if operationErr == nil {
		t.Fatalf("PostgreSQL accepted forbidden generation statement: %s", strings.TrimSpace(statement))
	}
}
