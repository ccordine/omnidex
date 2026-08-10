package queue

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCognitionFactAcceptanceMigrationOwnsExactAuthority(t *testing.T) {
	t.Parallel()
	raw := readCognitionMigration(t, "056_cognition_fact_acceptance.sql") +
		readCognitionMigration(t, "056_cognition_fact_acceptance_guards.sql")
	for _, required := range []string{
		"migration 056 cannot invent fact acceptance authority",
		"fact_authority_json TEXT NOT NULL",
		"fact_authority_identity_sha256",
		"CREATE TABLE cognition_episode_fact_policies",
		"CREATE TABLE cognition_accepted_facts",
		"CREATE TABLE cognition_accepted_fact_evidence",
		"require_exact_cognition_accepted_fact",
		"task_entries_require_cognition_accepted_fact",
		"cognition_accepted_facts_immutable",
		"DEFERRABLE INITIALLY DEFERRED",
	} {
		if !strings.Contains(raw, required) {
			t.Fatalf("fact acceptance migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"DEFAULT '{}'", "ON DELETE CASCADE"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("fact acceptance migration contains fallback %q", forbidden)
		}
	}
}

func TestPostgresCognitionFactAcceptanceMigrationRejectsLegacyEpisodes(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		CREATE TABLE cognition_episodes (episode_id TEXT PRIMARY KEY);
		INSERT INTO cognition_episodes VALUES ('legacy-episode');
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, readCognitionMigration(t, "056_cognition_fact_acceptance.sql")); err == nil ||
		!strings.Contains(err.Error(), "cannot invent fact acceptance authority") {
		t.Fatalf("migration error=%v, want explicit fact-authority preflight", err)
	}
	assertMigrationRelationExists(t, pool, "cognition_accepted_facts", false)
}

func TestCognitionFactPersistenceHasNoOptionalStoreOrCallerPlanPath(t *testing.T) {
	t.Parallel()
	storeRaw, err := os.ReadFile("../cognitionstore/store.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(storeRaw), "func New(repository *queue.Repository)") ||
		!strings.Contains(string(storeRaw), "facts cognitionstate.FactAcceptanceAuthority") {
		t.Fatal("cognitionstore.New does not require the exact executable fact authority")
	}
	for _, name := range []string{"cognition_episode_store.go", "cognition_action_success.go"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "...cognitionstate.FactAcceptanceAuthority") ||
			!strings.Contains(string(raw), "facts cognitionstate.FactAcceptanceAuthority") {
			t.Fatalf("%s exposes an optional or missing fact authority", name)
		}
	}
}
