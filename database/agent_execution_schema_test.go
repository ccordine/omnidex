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
		"'check','dependency'",
		"CREATE FUNCTION coding_ledger_owner_turn_statements_valid(statements jsonb) RETURNS boolean",
		"action text NOT NULL DEFAULT 'planning' CHECK (action IN ('planning','planning_control','execution_configuration','sign_off','cancel'))",
		"state text NOT NULL CHECK (state IN ('blocked','ready','accepted','cancelled'))",
		"CREATE FUNCTION validate_coding_agent_task_transition() RETURNS trigger",
		"ledger completion requires every queued task accepted",
		"queued task transitions require the exact active frozen ledger",
		"coding execution input requires a parsed final response",
		"action IN ('v3_coding','v3_coding_plan') AND status='running'",
		"continuation feedback lacks its exact owner reply and replanning boundary",
		"continuation_priority boolean NOT NULL DEFAULT false",
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
		"coding_ledger_questions",
		"coding_ledger_findings",
		"coding_ledger_planning_audits",
		"planning_audit_id",
		"coverage_passed",
		"'unresolved','mixed'",
		"'decision'",
		"'finding'",
		"payload_json->>'phase'='questions'",
		"payload_json->>'phase'='answers'",
	} {
		if strings.Contains(setup, retired) {
			t.Fatalf("coding ledger schema retains retired machinery %q", retired)
		}
	}
}
