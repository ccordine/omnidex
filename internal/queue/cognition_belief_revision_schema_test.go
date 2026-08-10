package queue

import (
	"os"
	"strings"
	"testing"
)

func TestCognitionBeliefRevisionMigrationBindsOneCodeOwnedRejection(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/052_cognition_belief_revision.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, required := range []string{
		"CREATE TABLE cognition_belief_revisions",
		"reconciliation_id TEXT NOT NULL UNIQUE",
		"source_snapshot_sha256 TEXT NOT NULL UNIQUE",
		"events.command_kind='reject_entry'",
		"events.actor='code'",
		"entries.kind='hypothesis'",
		"entries.authority='model_proposal'",
		"entries.status='rejected'",
		"NEW.target_uri='task:ledger/'||NEW.ledger_id||'/entry/'||entries.id",
		"NEW.target_version::BIGINT=entries.created_version",
		"target_ref'->>'uri' IS NOT DISTINCT FROM target_uri",
		"task_events_require_cognition_belief_revision",
		"cognition_belief_revisions_require_exact_authority",
		"cognition_belief_revisions_immutable",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("belief revision migration omitted %q", required)
		}
	}
}

func TestCognitionBeliefRevisionHasNoPublicOrModelRejectFallback(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("cognition_runtime_reconciliation.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if strings.Contains(source, "RejectEntryCommand{") ||
		strings.Contains(source, "ApplyTaskCommand(") {
		t.Fatal("reconciliation contains an untyped/public hypothesis rejection path")
	}
	if !strings.Contains(source, "applyCognitionBeliefRevisionTx(") {
		t.Fatal("reconciliation omitted the sole code-owned belief revision path")
	}
}
