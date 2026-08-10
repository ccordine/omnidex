package queue

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCognitionDecisionAuthorityMigrationsOwnExactDisposition(t *testing.T) {
	t.Parallel()
	var schema strings.Builder
	for _, name := range []string{
		"055_cognition_decision_authority.sql", "055_cognition_proposal_disposition.sql",
	} {
		raw, err := os.ReadFile("../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		schema.Write(raw)
	}
	for _, required := range []string{
		"migration 055 requires cognition_reconciliations empty",
		"CREATE TABLE cognition_decision_acceptances",
		"identity_json::jsonb=descriptor_json::jsonb-'id'-'sha256'",
		"cognition-policy-call-and-action-schema-v1",
		"CREATE TABLE cognition_proposal_dispositions",
		"accepted_materialization", "rejected_action_failure", "rejected_terminal_transition",
		"task_entries_require_cognition_proposal_disposition",
		"resolved cognition action omitted exact proposal disposition",
		"cognition_proposal_dispositions_immutable",
	} {
		if !strings.Contains(schema.String(), required) {
			t.Fatalf("decision authority migrations omitted %q", required)
		}
	}
}

func TestPostgresCognitionDecisionAuthorityRejectsLegacyReconciliation(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		CREATE TABLE cognition_reconciliations (reconciliation_id TEXT PRIMARY KEY);
		INSERT INTO cognition_reconciliations VALUES ('legacy');
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, readCognitionMigration(t, "055_cognition_decision_authority.sql")); err == nil || !strings.Contains(err.Error(), "accepted decision authority cannot be backfilled") {
		t.Fatalf("migration error=%v, want explicit legacy reconciliation rejection", err)
	}
	assertMigrationRelationExists(t, pool, "cognition_decision_acceptances", false)
}

func TestCognitionAcceptedDecisionHasNoDirectEntryInsertionPath(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "Kind: taskstate.EntryAcceptedDecision") {
			t.Fatalf("%s directly inserts accepted decision state", entry.Name())
		}
	}
}
