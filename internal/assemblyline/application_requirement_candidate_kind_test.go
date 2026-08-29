package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationRequirementCandidateKindReturnsCandidateBoundRelation(t *testing.T) {
	t.Parallel()
	input := ApplicationRequirementCandidateKindInput{
		Candidate: "The user can increment the current count.",
	}
	job, err := NewApplicationRequirementCandidateKindJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := job.Validate(); err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		input.Candidate,
		ApplicationRequirementCandidateTaskLocal,
		ApplicationRequirementCandidateNonRuntime,
		"exact candidate",
		"cardinality is a separate question",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("candidate-kind prompt omitted %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{
		"USER REQUEST:", "ACCEPTED REQUIREMENT", "EXCLUDED NON-RUNTIME", "PRODUCT CONTEXT:",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("candidate-kind prompt received non-candidate context %q:\n%s", forbidden, prompt)
		}
	}

	for _, relation := range []string{
		ApplicationRequirementCandidateTaskLocal,
		ApplicationRequirementCandidateNonRuntime,
	} {
		result, decodeErr := DecodeApplicationRequirementCandidateKindResult(input, relation)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		if result.Schema != ApplicationRequirementCandidateKindSchemaV1 ||
			result.CandidateSHA256 != ExactObjectiveContextSHA(input.Candidate) ||
			result.Relation != relation {
			t.Fatalf("candidate-kind result=%+v", result)
		}
		if err := result.ValidateFor(input); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := DecodeApplicationRequirementCandidateKindResult(input, "UNKNOWN"); err == nil {
		t.Fatal("unregistered candidate kind was accepted")
	}
	if maximum, err := PortableResponseMaximumBytesForJob(job); err != nil {
		t.Fatal(err)
	} else if maximum != len(ApplicationRequirementCandidateTaskLocal) {
		t.Fatalf("candidate-kind response maximum=%d", maximum)
	}
	if framing, err := PortableResponseFramingForWorkKind(job.Kind); err != nil {
		t.Fatal(err)
	} else if framing != PortableResponseFramingSingleLine {
		t.Fatalf("candidate-kind response framing=%q", framing)
	}

}

func TestApplicationRequirementCandidateKindReceiptRejectsUnboundState(t *testing.T) {
	t.Parallel()
	input := ApplicationRequirementCandidateKindInput{Candidate: "Export reports as CSV."}
	result, err := DecodeApplicationRequirementCandidateKindResult(
		input, ApplicationRequirementCandidateTaskLocal,
	)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*ApplicationRequirementCandidateKindResult){
		"schema": func(value *ApplicationRequirementCandidateKindResult) {
			value.Schema = "invalid"
		},
		"candidate hash": func(value *ApplicationRequirementCandidateKindResult) {
			value.CandidateSHA256 = strings.Repeat("0", 64)
		},
		"relation": func(value *ApplicationRequirementCandidateKindResult) {
			value.Relation = "UNKNOWN"
		},
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			changed := result
			mutate(&changed)
			if err := changed.ValidateFor(input); err == nil {
				t.Fatal("unbound candidate-kind receipt validated")
			}
		})
	}
}
