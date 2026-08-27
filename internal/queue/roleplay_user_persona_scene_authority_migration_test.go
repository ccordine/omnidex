package queue

import (
	"os"
	"strings"
	"testing"
)

const roleplayUserPersonaSceneAuthorityMigration = "150_roleplay_user_persona_scene_authority.sql"

func TestRoleplayUserPersonaSceneAuthorityMigrationRestoresCurrentSceneFence(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + roleplayUserPersonaSceneAuthorityMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"CREATE OR REPLACE FUNCTION validate_roleplay_user_turn_insert()",
		"roleplay_simulation_turn_preparations",
		"preparation.result->'user_turn'=user_turn.authority",
		"preparation.result->'participant_character_ids' ?",
		"user_turn.persona_character_id",
		"retained character turn without exact prepared scene authority",
		"FROM roleplay_current_scenes AS scene",
		"FOR SHARE",
		"FROM roleplay_scene_participants AS participant",
		"participant.scene_id=current_scene_id",
		"selected user persona must be a current scene participant",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("roleplay user-persona scene migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"preparation.active_character_id IS DISTINCT FROM user_turn.persona_character_id",
		"NEW.persona_character_id=current_responder_id",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("roleplay user-persona scene migration retained active-character exclusion %q", forbidden)
		}
	}
	for _, forbidden := range []string{"CREATE TABLE IF NOT EXISTS", "fallback", "compatibility"} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Fatalf("roleplay user-persona scene migration contains forbidden %q", forbidden)
		}
	}
}
