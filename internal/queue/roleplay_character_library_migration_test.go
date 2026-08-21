package queue

import (
	"os"
	"strings"
	"testing"
)

func TestRoleplayCharacterLibraryMigrationHasOnePortableIdentityAuthority(t *testing.T) {
	body, err := os.ReadFile("../../migrations/122_roleplay_character_library.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, required := range []string{
		"CREATE TABLE roleplay_character_library",
		"ADD COLUMN library_character_id TEXT",
		"UNIQUE (world_id,library_character_id)",
		"CREATE TABLE roleplay_character_profiles",
		"DROP TABLE roleplay_character_personas",
		"DROP FUNCTION reject_roleplay_identity_binding_update()",
		"reject_roleplay_world_identity_binding_update",
		"reject_roleplay_character_identity_binding_update",
		"roleplay_character_library_immutable",
		"roleplay_character_profiles_binding_immutable",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration omitted %q", required)
		}
	}
	if strings.Contains(sql, "ON DELETE CASCADE") {
		t.Fatal("portable character authority must not be destructively cascaded")
	}
}
