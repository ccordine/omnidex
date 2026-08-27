package roleplay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTransitionObserverMigrationFreezesExactPresenceAuthority(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join(
		"..", "..", "migrations", "151_roleplay_transition_observer_authority.sql",
	))
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(payload)), " ")
	for _, required := range []string{
		"ADD COLUMN observer_character_ids JSONB",
		"observer_character_ids IS NULL OR roleplay_transition_observers_are_exact(observer_character_ids)",
		"SET observer_character_ids=preparation.result->'participant_character_ids'",
		"preparation.pending_transition_id=transition.operation_id",
		"preparation.result->'pending_transition'=transition.result",
		"FOR UPDATE",
		"FOR SHARE",
		"NEW.observer_character_ids IS NULL",
		"current_observers IS DISTINCT FROM NEW.observer_character_ids",
		"preparation.result->'participant_character_ids'= NEW.observer_character_ids",
		"simulation transition differs from its frozen preparation observer authority",
		"CREATE TRIGGER roleplay_simulation_transitions_immutable BEFORE UPDATE OR DELETE",
		"idx_roleplay_transitions_observers",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("transition observer migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"ALTER COLUMN observer_character_ids SET NOT NULL",
		"observer_character_ids JSONB NOT NULL",
		"observer_character_ids JSONB DEFAULT",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("transition observer migration made historical authority guessable via %q", forbidden)
		}
	}
	if strings.Count(source, "UPDATE roleplay_simulation_transitions AS transition") != 1 {
		t.Fatalf("transition observer migration has an unexpected historical backfill surface")
	}
}
