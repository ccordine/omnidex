package database

import (
	"strings"
	"testing"
)

func TestCodingLedgerOwnsFiniteTaskQueue(t *testing.T) {
	setup := string(SetupSQL())
	for _, required := range []string{
		"CREATE TABLE coding_ledgers (",
		"CREATE TABLE coding_ledger_revisions (",
		"CREATE TABLE coding_ledger_items (",
		"CREATE TABLE coding_ledger_item_versions (",
		"CREATE TABLE coding_ledger_revision_items (",
		"CREATE TABLE coding_ledger_edges (",
		"CREATE TABLE coding_ledger_routes (",
		"CREATE TABLE coding_ledger_checks (",
		"CREATE TABLE coding_agent_tasks (",
		"state text NOT NULL CHECK (state IN ('blocked','ready','accepted','cancelled'))",
		"CREATE FUNCTION validate_coding_agent_task_transition() RETURNS trigger",
		"ledger completion requires every queued task accepted",
		"queued task transitions require the exact active frozen ledger",
	} {
		if !strings.Contains(setup, required) {
			t.Fatalf("coding ledger schema lacks %q", required)
		}
	}
}

func TestCodingLedgerHasNoRetiredAgentStateMachine(t *testing.T) {
	setup := string(SetupSQL())
	for _, retired := range []string{
		"coding_agent_attempts",
		"coding_agent_effect_manifests",
		"coding_agent_effects",
		"coding_agent_check_results",
		"coding_agent_interruptions",
		"coding_agent_grants",
		"coding_agent_internal_failures",
		"overall_reviewer",
		"overall_review",
		"task_reopened",
		"'executing'",
		"'verifying'",
		"'reviewing'",
		"'awaiting_input'",
	} {
		if strings.Contains(setup, retired) {
			t.Fatalf("coding ledger schema retains retired machinery %q", retired)
		}
	}
}
