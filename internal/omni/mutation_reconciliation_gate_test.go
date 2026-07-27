package omni

import (
	"bytes"
	"context"
	"testing"
)

func TestMutationGateObservationPreservesCommandOwnership(t *testing.T) {
	workspace := t.TempDir()
	result := CommandDecisionResult{
		ObjectiveLedger: []StructuredObjective{{ID: "rename_app_js_to_jsx", Status: "pending", Required: true}},
		ChildJobs: []ChildJob{{
			ID:                         "rename_app_js_to_jsx",
			Status:                     ChildJobStatusActive,
			RequiredEvidencePredicates: []string{"file_exists:src/App.jsx"},
		}},
	}
	if err := runStructuredPayloadCommand(context.Background(), 1, "mv src/App.js src/App.jsx", workspace, false, "", &bytes.Buffer{}, &bytes.Buffer{}, nil, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) != 1 {
		t.Fatalf("observations = %#v", result.Observations)
	}
	observation := result.Observations[0]
	if observation.ChildJobID != "rename_app_js_to_jsx" || observation.ObjectiveID != "rename_app_js_to_jsx" || observation.CommandID == "" {
		t.Fatalf("mutation gate dropped command ownership: %#v", observation)
	}
}
