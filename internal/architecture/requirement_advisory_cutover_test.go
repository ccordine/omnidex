package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionRequirementPartitionHasNoAdvisoryOrSplitRoute(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	checks := map[string][]string{
		"internal/worker/v3_coding_driver_plan.go": {
			"coding_requirement_adviser", "coding_requirement_split", "AdvisoryModel",
		},
		"internal/worker/v3_coding_workers.go": {
			"validateProductionAdvisoryJob", "llmGeneratePortableAdvisoryTrace",
		},
		"internal/worker/typed_worker.go": {
			"AdvisoryModel", "Advise func", "typedWorkerAdvisory",
		},
		"internal/worker/llm_response_contract.go": {
			"portable_advisory_worker",
		},
		"internal/modelconfig/config.go": {
			"coding_requirement_adviser_model", "coding_requirement_split_model",
		},
		"internal/modelconfig/routing.go": {
			"coding_requirement_adviser_model", "coding_requirement_split_model",
		},
		"internal/assemblyline/portable_job_inputs.go": {
			"RequirementDecidePresence", "RequirementPresenceInput",
		},
		"internal/assemblyline/application_spec.go": {
			"RequirementPresenceDecision", "RequirementPresenceSchema",
		},
	}
	for name, forbidden := range checks {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Fatalf("removed production requirement token %q remains in %s", token, name)
			}
		}
	}
}
