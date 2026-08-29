package queue

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSingleOutputLedgerAuthorityRetirementMigrationIsExplicit(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/170_single_output_and_ledger_authority_retirement.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"DROP COLUMN thinking_enabled",
		"DROP COLUMN source_entry_id",
		"DROP COLUMN acceptance_policy",
		"DROP COLUMN accepted_by",
		"task_nodes_created_by_code",
		"task_entries_creator_matches_authority",
		"task_events_command_event_pair",
		"context_projection_selected_authority_registered",
		"context_projection_omitted_authority_registered",
		"model-decision authority retirement requires a fresh reset",
		"jsonb_path_exists(payload,'$.entry.provenance')",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("authority retirement migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"DROP COLUMN IF EXISTS", "DROP CONSTRAINT IF EXISTS", "DROP INDEX IF EXISTS",
		"CASCADE", "UPDATE ", "DELETE FROM",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("authority retirement migration contains fallback or rewrite %q", forbidden)
		}
	}
}

func TestSingleOutputLedgerAuthorityRetirementDatabaseShape(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool := openIsolatedMigrationPool(t)
	if err := New(pool).EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	var retiredColumnCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_schema=current_schema() AND (
			(table_name='llm_call_evidence' AND column_name='thinking_enabled') OR
			(table_name='task_entries' AND column_name IN (
				'source_entry_id','acceptance_policy','accepted_by'
			))
		)
	`).Scan(&retiredColumnCount); err != nil {
		t.Fatal(err)
	}
	if retiredColumnCount != 0 {
		t.Fatalf("retired authority columns remain: %d", retiredColumnCount)
	}
	var retiredConstraintCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_constraint
		WHERE conrelid IN (
			'task_nodes'::regclass,'task_entries'::regclass,'task_events'::regclass,
			'context_projection_selected_refs'::regclass,
			'context_projection_omitted_refs'::regclass
		) AND (
			pg_get_constraintdef(oid) LIKE '%model_proposal%' OR
			pg_get_constraintdef(oid) LIKE '%accepted_model_decision%' OR
			pg_get_constraintdef(oid) LIKE '%decision_candidate%' OR
			pg_get_constraintdef(oid) LIKE '%accepted_decision%' OR
			pg_get_constraintdef(oid) LIKE '%accept_decision%' OR
			pg_get_constraintdef(oid) LIKE '%decision_accepted%'
		)
	`).Scan(&retiredConstraintCount); err != nil {
		t.Fatal(err)
	}
	if retiredConstraintCount != 0 {
		t.Fatalf("retired model-decision constraints remain: %d", retiredConstraintCount)
	}
}
