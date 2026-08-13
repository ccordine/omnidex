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
