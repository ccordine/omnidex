package assemblyline

import (
	"os"
	"strings"
	"testing"
)

func TestRuntimeCapabilityNecessityHasNoModelOwnedSelectionLoop(t *testing.T) {
	t.Parallel()
	if _, err := os.Stat("runtime_capability_selection.go"); !os.IsNotExist(err) {
		t.Fatalf("retired runtime capability selection source still exists: %v", err)
	}
	for _, file := range []string{
		"portable_job_registry.go",
		"semantic_uncertainty_coding.go",
		"../worker/v3_coding_runtime_capabilities.go",
		"../../database/setup.sql",
	} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, forbidden := range []string{
			"WorkRuntimeCapabilitySelection",
			"RuntimeCapabilitySelectionNone",
			"RuntimeCapabilitySelectionInput",
			"runtime_capability_selection",
			"coding_runtime_capability_selection",
			"REMAINING_CANDIDATE_PURPOSES",
			"ALREADY_ACCEPTED_PURPOSES",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("%s retains model-owned runtime selection authority %q", file, forbidden)
			}
		}
	}
}
