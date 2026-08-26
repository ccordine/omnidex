package queue

import (
	"os"
	"strings"
	"testing"
)

const generatedWorkloadDeploymentAuthorityHardeningMigration = "147_generated_workload_deployment_authority_hardening.sql"

func TestGeneratedWorkloadDeploymentAuthorityHardeningMigrationIsNullSafeAndForwardOnly(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/" + generatedWorkloadDeploymentAuthorityHardeningMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"generated_deployment_lifecycle_manifest_valid",
		"every existing lifecycle manifest to be exactly constructible",
		"every existing execution to match its exact lifecycle manifest entry",
		"initial_start execution ownership for every existing rollback attempt",
		"CREATE OR REPLACE FUNCTION validate_generated_deployment_binding_insert()",
		"CREATE OR REPLACE FUNCTION validate_generated_deployment_execution_insert()",
		"CREATE OR REPLACE FUNCTION validate_generated_deployment_rollback_attempt_insert()",
		"CREATE OR REPLACE FUNCTION validate_generated_deployment_rollback_observation_insert()",
		"manifest_entry->>'command_sha256' IS DISTINCT FROM NEW.command_sha256",
		"jsonb_typeof(entry->'command_sha256') IS DISTINCT FROM 'string'",
		"jsonb_typeof(entry->'workspace_sha256') IS DISTINCT FROM 'string'",
		"jsonb_typeof(service->'sha256') IS DISTINCT FROM 'string'",
		"jsonb_typeof(command->'config_sha256') IS DISTINCT FROM 'string'",
		"jsonb_typeof(command->'workspace_sha256') IS DISTINCT FROM 'string'",
		"jsonb_typeof(command->'secret_set_sha256') IS DISTINCT FROM 'string'",
		"exactly typed resolved-config evidence for every existing binding",
		"config.id IS NULL OR generated_deployment_resolved_config_binding_valid(",
		"initial_start_exists IS DISTINCT FROM TRUE",
		"pre_attempt_count<plan.max_attempts OR (NEW.outcome='clean' AND NOT EXISTS(",
		"WHERE operation_id=NEW.operation_id AND status='started'",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("deployment authority hardening migration lacks %q", required)
		}
	}
	if strings.Contains(source, "<>") || strings.Contains(source, "!~") {
		t.Fatal("deployment authority hardening reintroduced null-sensitive inequality")
	}
	if strings.Count(source, "generated_deployment_resolved_config_binding_valid(") != 3 {
		t.Fatal("existing-row preflight and binding insert do not share one full resolved-config validator")
	}
	for _, sealed := range []string{
		"141_generated_workload_deployment_evidence_rail.sql",
		"144_generated_workload_deployment_recovery.sql",
	} {
		if strings.Contains(source, "ALTER "+sealed) {
			t.Fatalf("forward migration attempts to alter sealed migration %s", sealed)
		}
	}
}
