package assemblyline

import (
	"strings"
	"testing"
)

func TestRuntimeCapabilitySelectionRendersOneDialectBoundOpaqueLeaf(t *testing.T) {
	t.Parallel()
	input := RuntimeCapabilitySelectionInput{
		LocalContext:     "A command that reads one external value.",
		Need:             "Return a caller-named local text resource.",
		Dialect:          "Go 1.24 command-line source",
		AcceptedPurposes: []string{},
		Candidates: []RuntimeCapabilityCandidateSummary{
			{CandidateID: "RUNTIME_CAPABILITY_1", Purpose: "Read one process environment value and report whether it is defined."},
			{CandidateID: "RUNTIME_CAPABILITY_2", Purpose: "Read all bytes from one user-supplied local file name."},
		},
	}
	job, err := NewRuntimeCapabilitySelectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkRuntimeCapabilitySelection {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"Go 1.24 command-line source", "RUNTIME_CAPABILITY_1",
		"RUNTIME_CAPABILITY_2", "Return exactly one raw opaque candidate ID or NONE",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("runtime capability prompt omitted %q:\n%s", required, prompt)
		}
	}
	for _, raw := range []string{"RUNTIME_CAPABILITY_2", RuntimeCapabilitySelectionNone} {
		decision, err := DecodeRuntimeCapabilitySelectionDecision(input, raw)
		if err != nil || decision.Selected != raw {
			t.Fatalf("raw=%q decision=%+v error=%v", raw, decision, err)
		}
	}
	for _, raw := range []string{" RUNTIME_CAPABILITY_1", "RUNTIME_CAPABILITY_3", "none"} {
		if _, err := DecodeRuntimeCapabilitySelectionDecision(input, raw); err == nil {
			t.Fatalf("invalid raw result %q was accepted", raw)
		}
	}
}

func TestRuntimeCapabilitySelectionRejectsSourceAuthorityAndUnboundedState(t *testing.T) {
	t.Parallel()
	valid := RuntimeCapabilitySelectionInput{
		LocalContext: "A bounded command.", Need: "Read one external value.",
		Dialect: "Go 1.24 command-line source", AcceptedPurposes: []string{},
		Candidates: []RuntimeCapabilityCandidateSummary{{
			CandidateID: "RUNTIME_CAPABILITY_1",
			Purpose:     "Read one process environment value and report whether it is defined.",
		}},
	}
	fixtures := map[string]func(*RuntimeCapabilitySelectionInput){
		"nil accepted set": func(input *RuntimeCapabilitySelectionInput) {
			input.AcceptedPurposes = nil
		},
		"missing dialect": func(input *RuntimeCapabilitySelectionInput) {
			input.Dialect = ""
		},
		"non opaque ID": func(input *RuntimeCapabilitySelectionInput) {
			input.Candidates[0].CandidateID = "runtime.stdlib.environment_value"
		},
		"package symbol": func(input *RuntimeCapabilitySelectionInput) {
			input.Candidates[0].Purpose = "Call os.LookupEnv for the value."
		},
		"function declaration": func(input *RuntimeCapabilitySelectionInput) {
			input.Candidates[0].Purpose = "Use func RuntimeEnvironmentValue for the value."
		},
		"path": func(input *RuntimeCapabilitySelectionInput) {
			input.Candidates[0].Purpose = "Read bytes through /private/value."
		},
	}
	for name, mutate := range fixtures {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := valid
			input.AcceptedPurposes = append([]string(nil), valid.AcceptedPurposes...)
			input.Candidates = append([]RuntimeCapabilityCandidateSummary(nil), valid.Candidates...)
			mutate(&input)
			if _, err := NewRuntimeCapabilitySelectionJob(input); err == nil {
				t.Fatal("invalid runtime capability input was accepted")
			}
		})
	}
}
