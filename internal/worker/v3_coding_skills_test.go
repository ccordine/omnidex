package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/specialists"
)

func TestLearnedCodingSkillIdentityAndSchemasAreCodeOwned(t *testing.T) {
	t.Parallel()

	input := assemblyline.SkillProcedureInput{
		LocalContext: "interactive browser tool",
		Need:         "support pressure-sensitive pointer interaction",
		Boundary:     assemblyline.SkillBoundaryTypeScriptReactView,
	}
	spec, err := newLearnedCodingSkillSpec(input, assemblyline.SkillProcedureDecision{
		Schema:    assemblyline.SkillProcedureSchemaV1,
		Procedure: "Track pointer pressure in local state and expose the current value through a labelled interactive control.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(spec.ID, "learned_") || strings.Contains(spec.ID, "pointer") {
		t.Fatalf("learned skill identity is not opaque and code-owned: %q", spec.ID)
	}
	if len(spec.AllowedTools) != 0 || len(spec.ForbiddenTools) != 0 {
		t.Fatalf("learned code procedure gained tools: %#v", spec)
	}
	if len(spec.InputSchemaDocument()) == 0 || len(spec.OutputSchemaDocument()) == 0 {
		t.Fatal("learned code procedure omitted fixed schemas")
	}
	jobID := int64(9)
	hash, err := specialists.SkillContentHash(spec, specialists.SkillKindCodeProcedure)
	if err != nil {
		t.Fatal(err)
	}
	version := specialists.SkillVersion{
		Spec: spec, Version: 1, Status: specialists.SkillStatusCandidate,
		Source: specialists.SkillSourceLearned, Kind: specialists.SkillKindCodeProcedure,
		CreatedByJobID: &jobID, ContentSHA256: hash,
	}
	if err := version.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCodingProgramRejectsMissingLearnedSkillBinding(t *testing.T) {
	t.Parallel()

	requirements := []assemblyline.Requirement{{ID: "requirement_001", SourceQuote: "filter visible records"}}
	err := validateDirectCodingSkillBindings(requirements, nil)
	if err == nil || !strings.Contains(err.Error(), "do not cover") {
		t.Fatalf("missing binding error=%v", err)
	}
}
