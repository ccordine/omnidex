package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestDeclarationArtifactBoundaryCorrectionChangesOnlyBoundaryLeaf(t *testing.T) {
	t.Parallel()

	input := assemblyline.DeclarationArtifactBoundaryInput{
		RequirementQuote: "func Normalize(input string) string has an independent artifact boundary",
		GoSignature:      "func Normalize(input string) string",
		DeclarationID:    "DECLARATION_1",
	}
	var prompts []string
	var kinds []assemblyline.WorkKind
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 2,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			prompt, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			prompts = append(prompts, prompt)
			kinds = append(kinds, job.Kind)
			if len(prompts) == 1 {
				return assemblyline.PortableResult{
					JobID: job.ID, Candidate: "unsupported",
				}, nil
			}
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: string(assemblyline.DeclarationBoundaryIndependentArtifact),
			}, nil
		},
	}
	decision, err := classifyDeclarationArtifactBoundary(runtime, "semantic", input)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Boundary != assemblyline.DeclarationBoundaryIndependentArtifact {
		t.Fatalf("decision=%+v", decision)
	}
	if len(prompts) != 2 || kinds[0] != assemblyline.WorkDeclarationArtifactBoundary ||
		kinds[1] != assemblyline.WorkResponseCorrection {
		t.Fatalf("calls=%v prompts=%d", kinds, len(prompts))
	}
	for _, required := range []string{
		"ORIGINAL_SEMANTIC_QUESTION:", input.RequirementQuote, input.GoSignature, input.DeclarationID,
		"CURRENT_REJECTED_LEAF:", "unsupported", "EXACT_GROUNDED_DEFECT:",
	} {
		if !strings.Contains(prompts[1], required) {
			t.Fatalf("correction omitted retained one-leaf authority %q: %s", required, prompts[1])
		}
	}
}
