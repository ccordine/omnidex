package queue

import (
	"os"
	"strings"
	"testing"
)

const generatedWorkloadDeploymentMigration = "140_generated_workload_deployment_journal.sql"
const generatedWorkloadDeploymentEvidenceMigration = "141_generated_workload_deployment_evidence_rail.sql"

func TestGeneratedWorkloadDeploymentMigrationDefinesSecretFreeImmutableAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + generatedWorkloadDeploymentMigration)
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(string(raw), "\n"); lines >= 300 {
		t.Fatalf("generated deployment migration has %d lines; maximum is 299", lines)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"UNIQUE (job_id,generation)",
		"compose_project TEXT NOT NULL UNIQUE",
		"generated_deployments_endpoint_port_authority",
		"REFERENCES job_step_attempts(job_id,generation,step_id,attempt)",
		"creator_step_attempt",
		"current_step_attempt",
		"NEW.current_step_attempt<=OLD.current_step_attempt",
		"secret_set_sha256",
		"generated deployment requires the exact current active step attempt",
		"generated deployment receipt is immutable",
		"generated deployment journal is immutable",
		"sealed generated deployment receipt evidence is immutable",
		"sealed generated deployment verification evidence is immutable",
		"item#>>'{}' !~ '^[1-9][0-9]*$'",
		"array_agg(DISTINCT value::BIGINT ORDER BY value::BIGINT)",
		"position('..' IN command->>'endpoint_host')>0",
		"command->>'endpoint_path' ~ '(^|/)[.][.]?(/|$)|//'",
		"NEW.receipt_sha256<>encode(digest(convert_to(NEW.receipt_json,'UTF8'),'sha256'),'hex')",
		"service->>'state'<>'running' OR service->>'health'<>'healthy'",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("generated deployment migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"secret_value", "environment_json", "raw_config", "stdout", "stderr", "command_output TEXT",
		"BETWEEN 1 AND 32",
	} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Fatalf("generated deployment schema contains forbidden payload %q", forbidden)
		}
	}
}

func TestGeneratedWorkloadDeploymentMigrationAppliesAfterIntentStation(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "140")); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, generatedWorkloadDeploymentMigration, 1)
}

func TestGeneratedWorkloadDeploymentEvidenceMigrationDefinesExactBoundedRail(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + generatedWorkloadDeploymentEvidenceMigration)
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(string(raw), "\n"); lines >= 300 {
		t.Fatalf("deployment evidence migration has %d lines; maximum is 299", lines)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"generated_workload_verifications",
		"cardinality(command_evidence_ids) BETWEEN 1 AND 128",
		"generated_workload_deployment_verifications",
		"lifecycle_manifest_sha256",
		"generated_workload_deployment_executions",
		"status TEXT NOT NULL CHECK (status IN ('started','completed'))",
		"generated_workload_deployment_execution",
		"deployment_operation_id",
		"workspace_sha256'=NEW.workspace_sha256",
		"generated_workload_deployment_observations",
		"compose_ps_evidence_id",
		"workspace_verification_receipt_id",
		"execution_evidence_ids",
		"observation_evidence_ids",
		"omnidex.generated-workload-deployment-receipt.v2",
		"generated_deployment_resolved_config_distinct",
		"implicit_env_disabled",
		"service_hashes",
		"environment_names",
		"(('build',20),('initial_start',30),('migrate',40),('initial_observe',50)",
		"generated_deployment_verification_truncate_immutable",
		"generated_deployment_execution_truncate_immutable",
		"generated_deployment_observation_truncate_immutable",
		"generated deployment cited evidence is immutable",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("deployment evidence migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"BETWEEN 1 AND 32", "verification_evidence_ids",
		"omnidex.generated-workload-deployment-receipt.v1",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("deployment evidence migration retains forbidden authority %q", forbidden)
		}
	}
}

func TestGeneratedWorkloadDeploymentEvidenceMigrationAppliesAfterJournal(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "141")); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, generatedWorkloadDeploymentMigration, 1)
	assertAppliedMigrationCount(t, pool, generatedWorkloadDeploymentEvidenceMigration, 1)
}
