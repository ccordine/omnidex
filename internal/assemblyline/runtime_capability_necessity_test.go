package assemblyline

import (
	"strings"
	"testing"
)

func TestRuntimeCapabilityNecessityRendersOneCandidateBoundRelation(t *testing.T) {
	t.Parallel()
	input := RuntimeCapabilityNecessityInput{
		LocalContext:     "A command that reads one external value.",
		Need:             "Return a caller-named local text resource.",
		Dialect:          "Go 1.24 command-line source",
		CandidatePurpose: "Read all bytes from one user-supplied local file name.",
	}
	job, err := NewRuntimeCapabilityNecessityJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkRuntimeCapabilityNecessity {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"Go 1.24 command-line source",
		input.CandidatePurpose,
		"RUNTIME_CAPABILITY_NECESSARY or RUNTIME_CAPABILITY_NOT_NECESSARY",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("runtime capability prompt omitted %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{
		"RUNTIME_CAPABILITY_1", "ALREADY_ACCEPTED", "REMAINING_CANDIDATE", " or NONE",
		"This call sees", "workflow", "completion", "queue", "code owns",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("runtime capability prompt retained set-selection authority %q:\n%s", forbidden, prompt)
		}
	}
	for _, raw := range []string{RuntimeCapabilityNecessary, RuntimeCapabilityNotNecessary} {
		decision, err := DecodeRuntimeCapabilityNecessityDecision(input, raw)
		if err != nil || decision.Relation != raw {
			t.Fatalf("raw=%q decision=%+v error=%v", raw, decision, err)
		}
	}
	for _, raw := range []string{" " + RuntimeCapabilityNecessary, "NONE", "RUNTIME_CAPABILITY_1"} {
		if _, err := DecodeRuntimeCapabilityNecessityDecision(input, raw); err == nil {
			t.Fatalf("invalid raw result %q was accepted", raw)
		}
	}
}

func TestRuntimeCapabilityNecessityRejectsSourceAuthority(t *testing.T) {
	t.Parallel()
	valid := RuntimeCapabilityNecessityInput{
		LocalContext: "A bounded command.", Need: "Read one external value.",
		Dialect:          "Go 1.24 command-line source",
		CandidatePurpose: "Read one process environment value and report whether it is defined.",
	}
	fixtures := map[string]func(*RuntimeCapabilityNecessityInput){
		"missing dialect": func(input *RuntimeCapabilityNecessityInput) {
			input.Dialect = ""
		},
		"missing candidate": func(input *RuntimeCapabilityNecessityInput) {
			input.CandidatePurpose = ""
		},
		"package symbol": func(input *RuntimeCapabilityNecessityInput) {
			input.CandidatePurpose = "Call os.LookupEnv for the value."
		},
		"function declaration": func(input *RuntimeCapabilityNecessityInput) {
			input.CandidatePurpose = "Use func RuntimeEnvironmentValue for the value."
		},
		"path": func(input *RuntimeCapabilityNecessityInput) {
			input.CandidatePurpose = "Read bytes through /private/value."
		},
	}
	for name, mutate := range fixtures {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := valid
			mutate(&input)
			if _, err := NewRuntimeCapabilityNecessityJob(input); err == nil {
				t.Fatal("invalid runtime capability input was accepted")
			}
		})
	}
}
