package queue

import (
	"context"
	"strings"
	"testing"
)

func TestCognitionEnvironmentJournalMigrationOwnsDurabilityAndBrainAuthority(t *testing.T) {
	raw := readCognitionMigration(t, "051_cognition_environment_journal.sql") +
		readCognitionMigration(t, "051_cognition_environment_projection_guards.sql")
	for _, required := range []string{
		"migration 051 cannot invent attested Brain authority",
		"attested_brain_json TEXT NOT NULL",
		"NEW.attested_brain_json,NEW.attested_brain_sha256",
		"CREATE TABLE cognition_environment_journals",
		"start_json::jsonb#>>'{current,episode_id}'=episode_id",
		"NOT (start_json::jsonb ? 'previous')",
		"CREATE TABLE cognition_environment_receipts",
		"CREATE TABLE cognition_episode_cancellations",
		"source_evidence_id='cognition_cancellation_evidence_'||source_evidence_sha256",
		"UNIQUE (episode_id,source_evidence_id)",
		"OLD.terminal",
		"NEW.current_revision=OLD.current_revision+1",
		"cognition_environment_projection_exact",
		"DEFERRABLE INITIALLY DEFERRED",
		"actions.registered_action_json::jsonb-'actor'",
		"cognition_terminal_seals_require_cancellation",
		"prevent_cognition_immutable_mutation",
	} {
		if !strings.Contains(raw, required) {
			t.Fatalf("migration 051 lacks %q", required)
		}
	}
	for _, forbidden := range []string{
		"DEFAULT '{}'", "ON DELETE CASCADE", "nullable brain", "backfill",
		"source_evidence_id TEXT NOT NULL UNIQUE",
	} {
		if strings.Contains(strings.ToLower(raw), strings.ToLower(forbidden)) {
			t.Fatalf("migration 051 contains forbidden compatibility path %q", forbidden)
		}
	}
}

func TestPostgresCognitionEnvironmentMigrationRejectsExistingEpisodes(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		CREATE TABLE cognition_episodes (episode_id TEXT PRIMARY KEY);
		INSERT INTO cognition_episodes VALUES ('legacy-episode');
	`); err != nil {
		t.Fatal(err)
	}
	migration := readCognitionMigration(t, "051_cognition_environment_journal.sql")
	if _, err := pool.Exec(ctx, migration); err == nil ||
		!strings.Contains(err.Error(), "cannot invent attested Brain authority") {
		t.Fatalf("migration error=%v, want explicit brain-authority preflight", err)
	}
	assertMigrationRelationExists(t, pool, "cognition_environment_journals", false)
}
