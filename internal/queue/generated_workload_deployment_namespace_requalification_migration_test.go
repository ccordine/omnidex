package queue

import (
	"os"
	"strings"
	"testing"
)

const generatedWorkloadDeploymentNamespaceRequalificationMigration = "146_generated_workload_deployment_namespace_requalification.sql"

func TestGeneratedWorkloadDeploymentNamespaceRequalificationMigrationAuthority(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/" + generatedWorkloadDeploymentNamespaceRequalificationMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"explicit audit of every legacy protected execution",
		"generated_workload_deployment_namespace_requalifications",
		"PRIMARY KEY(operation_id,slot_ordinal,step_attempt)",
		"generated_workload_deployment_namespace_requalification",
		"generated_deployment_vacant_namespace_preflight_valid",
		"deployment.current_step_attempt,deployment.current_worker_id",
		"candidate_step_attempt=NEW.step_attempt",
		"attempts.expires_at>clock_timestamp()",
		"FOR UPDATE",
		"generated_deployment_execution_00_namespace_requalification_require",
		"protected deployment execution lacks exact current-attempt namespace requalification",
		"generated_deployment_namespace_requalification_change_immutable",
		"generated_deployment_namespace_requalification_evidence_immutable",
		"generated_deployment_namespace_requalification_converge_from_proof",
		"generated_deployment_namespace_requalification_converge_from_execution",
		"generated_deployment_head_consistency_from_namespace_requalification",
		"EXECUTE FUNCTION validate_generated_deployment_head_consistency()",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("namespace requalification migration lacks %q", required)
		}
	}
	if strings.Contains(source, "UPDATE generated_workload_deployment_namespace_requalifications") ||
		strings.Contains(source, "DELETE FROM generated_workload_deployment_namespace_requalifications") {
		t.Fatal("namespace requalification migration contains mutable cleanup/adoption authority")
	}
}

func TestGeneratedWorkloadDeploymentProtectedNamespaceSlotsAreTaskNeutralAndExact(t *testing.T) {
	for _, testCase := range []struct {
		slot GeneratedWorkloadDeploymentLifecycleSlot
		want bool
	}{
		{GeneratedDeploymentSlotBuild, true},
		{GeneratedDeploymentSlotInitialStart, true},
		{GeneratedDeploymentSlotMigrate, false},
		{GeneratedDeploymentSlotRestart, false},
		{GeneratedWorkloadDeploymentLifecycleSlot{Name: "benchmark-specific", Ordinal: 777}, false},
	} {
		if got := generatedDeploymentSlotRequiresNamespaceRequalification(testCase.slot); got != testCase.want {
			t.Fatalf("slot %+v requires requalification=%t want %t", testCase.slot, got, testCase.want)
		}
	}
}
