package specialists

import (
	"strings"
	"testing"
)

func TestSpecAcceptsBoundedPathFreeCodeLocalProcedure(t *testing.T) {
	t.Parallel()

	spec := Spec{
		ID:      "learned_0123456789abcdef0123456789abcdef",
		Purpose: "Transform one bounded collection using its declared local inputs.",
		Instructions: "Iterate over the local values, retain values that satisfy the supplied " +
			"predicate, and return the resulting collection.",
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("Validate() rejected a code-local procedure: %v", err)
	}
}

func TestSpecRetainsLegitimateAgentWorkerAndOrchestrationConcepts(t *testing.T) {
	t.Parallel()

	spec := Spec{
		ID: "learned_0123456789abcdef0123456789abcdef",
		Purpose: "Interpret a user agent value and coordinate a Web Worker during domain " +
			"orchestration.",
		Instructions: "Parse the local user agent value, pass the derived numeric input to the " +
			"declared Web Worker capability, and return its result after domain orchestration completes.",
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("Validate() inferred framework control from domain nouns: %v", err)
	}
}

func TestSpecRejectsUnsafeModelVisibleProcedureText(t *testing.T) {
	t.Parallel()

	base := Spec{
		ID:           "learned_0123456789abcdef0123456789abcdef",
		Purpose:      "Transform one bounded collection using its declared local inputs.",
		Instructions: "Return a collection containing the matching local values.",
	}
	tests := map[string]struct {
		mutate func(*Spec)
		want   string
	}{
		"purpose leading whitespace": {
			mutate: func(spec *Spec) { spec.Purpose = " " + spec.Purpose },
			want:   "exact-trimmed",
		},
		"instructions trailing whitespace": {
			mutate: func(spec *Spec) { spec.Instructions += "\n" },
			want:   "exact-trimmed",
		},
		"purpose path": {
			mutate: func(spec *Spec) { spec.Purpose = "Transform values described in src/value.go." },
			want:   "filesystem identity",
		},
		"instructions path": {
			mutate: func(spec *Spec) { spec.Instructions = `Read the value from C:\private\value.` },
			want:   "filesystem identity",
		},
		"purpose framework control": {
			mutate: func(spec *Spec) { spec.Purpose = "Choose the downstream worker for the task queue." },
			want:   "framework-control language",
		},
		"instructions orchestration": {
			mutate: func(spec *Spec) {
				spec.Instructions = "Act as an orchestrator before returning the local value."
			},
			want: "framework-control language",
		},
		"purpose oversized": {
			mutate: func(spec *Spec) { spec.Purpose = strings.Repeat("p", maxSkillPurposeBytes+1) },
			want:   "exceeds",
		},
		"instructions oversized": {
			mutate: func(spec *Spec) {
				spec.Instructions = strings.Repeat("i", maxSkillInstructionsBytes+1)
			},
			want: "exceeds",
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			spec := base
			test.mutate(&spec)
			err := spec.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestSkillVersionRejectsUnsafeModelVisibleProcedureText(t *testing.T) {
	t.Parallel()

	version := validSkillVersion(t)
	version.Spec.Instructions = "Send the result to a downstream agent."
	if err := version.Validate(); err == nil || !strings.Contains(err.Error(), "framework-control language") {
		t.Fatalf("Validate() error=%v, want model-visible procedure failure", err)
	}
}
