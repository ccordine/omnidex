package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationProjectStackConstraintUsesOnlyOpaqueModelIDs(t *testing.T) {
	t.Parallel()
	input := ApplicationProjectStackConstraintInput{
		UserRequest: "Build a terminal utility.",
		Candidates: []ApplicationProjectStackCandidate{
			{CandidateID: "STACK_CANDIDATE_1", TechnicalFormat: "Go with a module manifest"},
			{CandidateID: "STACK_CANDIDATE_2", TechnicalFormat: "Rust with a Cargo manifest"},
		},
	}
	prompt, err := BuildApplicationProjectStackConstraintPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"A. Use this technical format and packaging shape: Go with a module manifest",
		"B. Use this technical format and packaging shape: Rust with a Cargo manifest",
		"C. No registered technical format can satisfy an explicit technical constraint",
		"Answer with A or B or C.",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("project-stack prompt omitted %q: %s", required, prompt)
		}
	}
	for _, forbidden := range []string{
		input.Candidates[0].CandidateID,
		input.Candidates[1].CandidateID,
		ApplicationProjectStackUnsupported,
		`"candidate_id"`,
		`"schema"`,
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("project-stack prompt exposed code-owned value %q: %s", forbidden, prompt)
		}
	}
	decision, err := DecodeApplicationProjectStackConstraintDecision(input, "B")
	if err != nil {
		t.Fatal(err)
	}
	if decision.CandidateID != input.Candidates[1].CandidateID {
		t.Fatalf("decoded candidate = %q", decision.CandidateID)
	}
	if _, err := DecodeApplicationProjectStackConstraintDecision(
		input,
		input.Candidates[1].CandidateID,
	); err == nil {
		t.Fatal("code-owned project-stack candidate ID was accepted as model output")
	}
}
