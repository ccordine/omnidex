package cognitiongauntlet

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func createOfflineGrantFixture(
	t *testing.T,
	admin *pgxpool.Pool,
	schema string,
	role string,
	withSequence bool,
) {
	t.Helper()
	schemaID, roleID := pgx.Identifier{schema}.Sanitize(), pgx.Identifier{role}.Sanitize()
	commands := []string{
		"CREATE SCHEMA " + schemaID,
		"REVOKE ALL ON SCHEMA " + schemaID + " FROM PUBLIC",
		"CREATE ROLE " + roleID + " NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION",
		"CREATE TABLE " + pgx.Identifier{schema, "authority_probe"}.Sanitize() + "(id BIGINT)",
	}
	if withSequence {
		commands = append(commands,
			"CREATE SEQUENCE "+pgx.Identifier{schema, "authority_probe_sequence"}.Sanitize(),
		)
	}
	for _, command := range commands {
		if _, err := admin.Exec(t.Context(), command); err != nil {
			t.Fatal(err)
		}
	}
}

func assertOfflinePrivilege(
	t *testing.T,
	admin *pgxpool.Pool,
	role string,
	schema string,
	privilege string,
	want bool,
) {
	t.Helper()
	var got bool
	if err := admin.QueryRow(t.Context(), `SELECT has_schema_privilege($1,$2,$3)`,
		role, schema, privilege,
	).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("schema privilege %s=%t want %t", privilege, got, want)
	}
}

func dropOfflineAuthorityFixture(
	admin *pgxpool.Pool,
	firstSchema string,
	secondSchema string,
	role string,
) {
	if admin == nil {
		return
	}
	ctx := context.Background()
	for _, schema := range []string{firstSchema, secondSchema} {
		if schema != "" {
			_, _ = admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		}
	}
	if role == "" {
		return
	}
	var exists bool
	if err := admin.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)`, role,
	).Scan(&exists); err != nil || !exists {
		return
	}
	roleID := pgx.Identifier{role}.Sanitize()
	_, _ = admin.Exec(ctx, "DROP OWNED BY "+roleID)
	_, _ = admin.Exec(ctx, "DROP ROLE "+roleID)
}
