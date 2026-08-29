package roleplay

import (
	"os"
	"strings"
	"testing"
)

func TestRoleplayUserCanonMigrationHasOneReceiptBackedSourceAuthority(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/152_roleplay_user_canon_provenance.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"CREATE TABLE roleplay_user_canon_completions",
		"source_message_id BIGINT NOT NULL UNIQUE",
		"roleplay_user_canon_completions_authority",
		"roleplay_lifecycle_requires_user_canon_receipt",
		"DEFERRABLE INITIALLY DEFERRED",
		"roleplay_user_canon_payload_valid",
		"'roleplay_responses','roleplay_user_canon'",
		"message.role='user' AND EXISTS",
		"completion.source_message_id=message.id",
		"completion.facts ? NEW.content",
		"roleplay_user_canon_materialization_exact",
		"memory.content<>event.content",
		"NEW.knowledge_character_ids<>expected_recipients",
		"roleplay_user_canon_completions_immutable",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("migration 152 lacks %q", required)
		}
	}
	for _, forbidden := range []string{
		"COALESCE(NEW.knowledge_character_ids",
		"ON CONFLICT",
		"fallback",
		"keyword",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("migration 152 contains forbidden compatibility or heuristic path %q", forbidden)
		}
	}
}
