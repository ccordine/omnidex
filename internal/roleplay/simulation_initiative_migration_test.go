package roleplay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitiativeMigrationBindsExactNextCursorAndTypedClockAuthority(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join(
		"..", "..", "migrations", "148_roleplay_initiative_time_authority.sql",
	))
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(payload)), " ")
	for _, required := range []string{
		"CREATE FUNCTION roleplay_next_initiative_character(result_value JSONB)",
		"NEW.active_character_id=roleplay_next_initiative_character(preparation.result)",
		"advance.active_character_id=roleplay_next_initiative_character(preparation.result)",
		"before_initiative_round,before_initiative_turn,before_fictional_time_tick",
		"after_initiative_round,after_initiative_turn,after_fictional_time_tick",
		"responder#>'{narrative_projection,scene,initiative}'<>frozen_initiative",
		"old_fingerprint||':'||initiative_round::text||':'||initiative_turn::text||':'||initiative_tick::text",
		"advance.active_character_id IS DISTINCT FROM preparation.result->'responder_routes'->0->>'character_id'",
		"SET active_character_id=clock.active_character_id",
		"UNIQUE (scene_id,before_revision)",
		"CREATE CONSTRAINT TRIGGER roleplay_scenes_require_initiative_advance",
		"scene initiative mutation requires one exact authoritative turn advance",
		"participant.character_id=OLD.current_character_id",
		"participant.character_id=NEW.current_character_id AND participant.turn_position=0",
		"NEW.revision=OLD.revision+1",
		"SET CONSTRAINTS ALL IMMEDIATE; SET CONSTRAINTS ALL DEFERRED; ALTER TABLE roleplay_current_scenes",
		"cannot reconstruct contradictory legacy turn-advance authority",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("initiative migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"advance.active_character_id= preparation.result->'responder_routes'->0->>'character_id'",
		"NEW.active_character_id=preparation.result->'responder_routes'->0->>'character_id'",
		"user_character_id=result_value->>'active_character_id'",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("initiative migration retained first-responder cursor assumption %q", forbidden)
		}
	}
}
