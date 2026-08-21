package queue

import (
	"os"
	"strings"
	"testing"
)

func TestRoleplayVoiceRetirementRemovesConfigurationAndRejectsNewOpenings(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/129_retire_roleplay_voice_rewrite.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"DROP COLUMN voice_rewrite_enabled",
		"DROP COLUMN voice_rewrite_model",
		"roleplay_voice_rewrite','roleplay_voice_preservation",
		"station_gap_openings_reject_retired_roleplay_voice",
		"WITH RECURSIVE unresolved_chain",
		"outcome.id IS NULL",
		"job.status IN ('pending','running','waiting_input')",
		"omnidex.roleplay-character-generation.v1",
		"WHILE kind='response_correction' LOOP",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("roleplay voice retirement lacks %q", required)
		}
	}
}

func TestRoleplayVoiceRetirementMigrationAppliesAfterUserTurnAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	var retiredColumns, triggerCount int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT count(*) FROM information_schema.columns
			 WHERE table_schema=current_schema()
			   AND table_name='roleplay_character_generation_configs'
			   AND column_name IN ('voice_rewrite_enabled','voice_rewrite_model')),
			(SELECT count(*) FROM pg_trigger
			 WHERE tgrelid='station_gap_openings'::regclass
			   AND tgname='station_gap_openings_reject_retired_roleplay_voice'
			   AND NOT tgisinternal)
	`).Scan(&retiredColumns, &triggerCount); err != nil {
		t.Fatal(err)
	}
	if retiredColumns != 0 || triggerCount != 1 {
		t.Fatalf("retired columns=%d trigger=%d", retiredColumns, triggerCount)
	}
}
