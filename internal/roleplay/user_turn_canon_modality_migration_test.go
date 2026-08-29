package roleplay

import (
	"os"
	"strings"
	"testing"
)

const roleplayUserCanonModalityMigration = "157_roleplay_user_canon_modality_authority.sql"

func TestRoleplayUserCanonModalityMigrationOwnsOneTypedPredicate(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/" + roleplayUserCanonModalityMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"BEGIN;",
		"inherited roleplay user canon modality authority differs",
		"CREATE FUNCTION roleplay_user_turn_requires_canon(",
		"LANGUAGE SQL IMMUTABLE STRICT",
		"persona_kind_value='character'",
		"part.value->>'kind' IN ('action','event')",
		"contribution_kind_value IN ('direction','command') THEN FALSE",
		"CREATE OR REPLACE FUNCTION validate_roleplay_user_canon_completion()",
		"CREATE OR REPLACE FUNCTION enforce_roleplay_user_canon_lifecycle_receipt()",
		"roleplay_user_canon_completions_immutable",
		"roleplay_user_canon_completions_truncate_immutable",
		"roleplay user canon modality authority postcondition failed",
		"COMMIT;",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("migration 157 lacks %q", required)
		}
	}
	if strings.Count(source, "roleplay_user_turn_requires_canon(") < 12 {
		t.Fatal("migration 157 does not route validators and postconditions through one predicate")
	}
	for _, forbidden := range []string{"fallback", "keyword", "ILIKE", "regexp_matches"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("migration 157 contains forbidden semantic routing %q", forbidden)
		}
	}
}

func TestUserCanonAppendConsumesDatabaseOwnedModalityPredicate(t *testing.T) {
	raw, err := os.ReadFile("user_turn_canon_append.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if !strings.Contains(source, "roleplay_user_turn_requires_canon(") {
		t.Fatal("user canon append does not consume the database-owned typed predicate")
	}
	if strings.Contains(source, "user_turn.contribution_kind<>'command'") {
		t.Fatal("user canon append retains the stale command-only predicate")
	}
}
