package worker

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/operation"
	"github.com/gryph/omnidex/internal/queue"
)

func TestPersistedCleanupConfigProofRejectsResolvedComposeDrift(t *testing.T) {
	t.Parallel()
	command := queue.GeneratedWorkloadDeploymentCommand{
		Services: []string{"api"}, WorkspaceSHA256: strings.Repeat("a", 64),
		SecretSetSHA256: strings.Repeat("b", 64),
	}
	result := operation.Result{Output: map[string]any{
		"succeeded": true, "stdout_truncated": false,
		"stdout": "api " + strings.Repeat("c", 64) + "\n", "stderr": "",
	}}
	configSHA, _, err := directCodingResolvedConfigSHA256(
		result, command.Services, command.WorkspaceSHA256, command.SecretSetSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	command.ConfigSHA256 = configSHA
	if err := validateDirectCodingPersistedCleanupConfig(command, result); err != nil {
		t.Fatal(err)
	}
	result.Output["stdout"] = "api " + strings.Repeat("d", 64) + "\n"
	if err := validateDirectCodingPersistedCleanupConfig(command, result); err == nil ||
		!strings.Contains(err.Error(), "differs from persisted authority") {
		t.Fatalf("resolved Compose drift error=%v", err)
	}
}

func TestCleanupConfigDriftCannotReachRollbackSpawn(t *testing.T) {
	t.Parallel()
	binder, err := os.ReadFile("v3_coding_deployment_recovery_rollback_execution.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(binder)
	proof := strings.Index(text, "executeDirectCodingDeploymentCommand(")
	config := strings.Index(text, "directCodingDeploymentConfig, command.ProfileID")
	validation := strings.Index(text, "validateDirectCodingPersistedCleanupConfig(command, configResult)")
	authorize := strings.Index(text, "runtime.prepared.environment = environment")
	if proof < 0 || config <= proof || validation <= config || authorize <= validation {
		t.Fatal("deployment cleanup can be authorized before exact resolved Compose config proof")
	}
	gate, err := os.ReadFile("v3_coding_deployment_early_recovery.go")
	if err != nil {
		t.Fatal(err)
	}
	text = string(gate)
	bind := strings.Index(text, "bindPersistedRollbackExecution(observer, snapshot)")
	rollback := strings.Index(text, "observer.Rollback(transition)")
	if bind < 0 || rollback <= bind {
		t.Fatal("deployment cleanup can spawn rollback before persisted input binding")
	}
}
