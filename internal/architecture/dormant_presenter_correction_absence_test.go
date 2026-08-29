package architecture

import (
	"os"
	"strings"
	"testing"
)

func TestRetiredStepContextPresentersAreAbsent(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../cmd/cli/watch_context.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, forbidden := range []string{
		`case "workspace":`,
		`case "coding_diff":`,
		`"host_environment"`,
		`"retrieved_memory"`,
		`"recent_conversation"`,
		`"llm_model_prepare"`,
		`"llm_prompt"`,
		`"llm_response"`,
		`"llm_error"`,
		"Explore step",
		"Review step",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("CLI context presenter retains retired branch %q", forbidden)
		}
	}
}

func TestDormantCorrectionResidueIsAbsent(t *testing.T) {
	t.Parallel()
	for path, forbidden := range map[string][]string{
		"../../internal/assemblyline/typescript_violation.go": {
			"Instruction string",
			"TypeScriptFragmentCorrectionInstruction",
		},
		"../../internal/worker/v3_coding_typescript_feedback.go": {
			"directCodingTypeScriptFragmentFailure",
			"CORRECTION_REJECTION",
		},
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Errorf("%s retains dormant correction token %q", path, token)
			}
		}
	}
	if _, err := os.Stat("../../internal/worker/exact_station_convergence_context.go"); !os.IsNotExist(err) {
		t.Fatalf("retired exact-station convergence context remains: %v", err)
	}
	if _, err := os.Stat("../../cmd/cli/watch_llm_trace.go"); !os.IsNotExist(err) {
		t.Fatalf("retired LLM step-context presenter remains: %v", err)
	}
}
