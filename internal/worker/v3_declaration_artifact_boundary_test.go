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
			prompt, schema, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			prompts = append(prompts, prompt)
			kinds = append(kinds, job.Kind)
			if len(prompts) == 1 {
				return assemblyline.PortableResult{
					JobID:     job.ID,
					Candidate: `{"schema":"omnidex.declaration-artifact-boundary.v1","declaration_id":"DECLARATION_1","boundary":"unsupported"}`,
				}, nil
			}
			properties := schema["properties"].(map[string]any)
			if len(properties) != 1 || properties["boundary"] == nil {
				t.Fatalf("correction schema may alter more than boundary: %#v", schema)
			}
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: `{"boundary":"independent_artifact"}`,
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
	for _, retained := range []string{input.RequirementQuote, input.GoSignature, input.DeclarationID} {
		if strings.Contains(prompts[1], retained) {
			t.Fatalf("correction replayed retained authority %q: %s", retained, prompts[1])
		}
	}
	if !strings.Contains(prompts[1], "unsupported") {
		t.Fatalf("correction omitted exact validation failure: %s", prompts[1])
	}
}
