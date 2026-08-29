package queue

import (
	"os"
	"strings"
	"testing"
)

func TestConversationContextSelectionMigrationIsNarrowAndHashGuarded(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("../../migrations/073_conversation_context_selection_station.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, required := range []string{
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"expected_pre_sha256 CONSTANT TEXT",
		"expected_post_sha256 CONSTANT TEXT",
		"digest(convert_to(observed_source,'UTF8'),'sha256')",
		"CREATE OR REPLACE FUNCTION station_owns_portable_work",
		"WHEN 'conversation_context_selection' THEN station='conversation_context_selection'",
		"prior station function hash",
		"context-selection station postcondition failed",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("migration lacks %q", required)
		}
	}
	for _, forbidden := range []string{
		"ALTER TABLE", "CREATE TABLE", "DROP TABLE", "DROP FUNCTION", "CASCADE",
		"IF EXISTS", "fallback", "legacy",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("migration contains out-of-scope %q", forbidden)
		}
	}
}

func TestPostgresConversationContextSelectionMigrationRejectsChangedPriorFunction(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "072")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		CREATE OR REPLACE FUNCTION station_owns_portable_work(station TEXT, work_kind TEXT, payload JSONB)
		RETURNS BOOLEAN AS 'SELECT FALSE' LANGUAGE SQL IMMUTABLE STRICT
	`); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "073"))
	if err == nil || !strings.Contains(err.Error(), "prior station function hash") {
		t.Fatalf("migration error=%v", err)
	}
	var installed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename='073_conversation_context_selection_station.sql')
	`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("rejected migration wrote its ledger entry")
	}
}
