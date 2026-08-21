package queue

import (
	"os"
	"strings"
	"testing"
)

const roleplayUserTurnAuthorityMigration = "128_roleplay_user_turn_authority.sql"

func TestRoleplayUserTurnMigrationPersistsOneImmutableTypedAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + roleplayUserTurnAuthorityMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"CREATE TABLE roleplay_user_turns",
		"persona_kind IN ('character','narrator','legacy_untyped')",
		"'dialogue','action','action_dialogue','narration','direction'",
		"CREATE FUNCTION validate_roleplay_user_turn_insert()",
		"selected user persona must be a current participant distinct from the responding character",
		"CREATE CONSTRAINT TRIGGER ai_channel_messages_require_roleplay_user_turn",
		"roleplay user message requires explicit turn authority in the same transaction",
		"roleplay_user_turns_immutable",
		"roleplay_user_turns_truncate_immutable",
		"result ?& ARRAY['preparation_id'",
		"'active_character_id','user_turn','input_kind'",
		"job.metadata->'roleplay_user_turn'=preparation.result->'user_turn'",
		"'roleplay_generation_config','roleplay_user_turn'",
		"roleplay user turn authority postcondition failed",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("roleplay user-turn migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"ON DELETE CASCADE", "CREATE TABLE IF NOT EXISTS", "fallback", "compatibility",
	} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Fatalf("roleplay user-turn migration contains forbidden %q", forbidden)
		}
	}
}
