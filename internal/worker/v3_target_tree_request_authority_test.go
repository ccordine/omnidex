package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRawRequestAuthorityStopsAtSemanticTargetTreeBoundary(t *testing.T) {
	const sentinel = "REQUEST_BOUNDARY_THETA"
	const request = "Build a browser display. Structural authority marker: " + sentinel + "."
	specification, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceBrowser,
		"measurement display", "Display the current measurement.",
	)
	stack := inferredTypeScriptTargetTreeFixture(t)
	input, err := directCodingTargetTreeInput(
		request, specification, workload, stack, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if input.Objective != request {
		t.Fatalf("target-tree objective=%q want exact request %q", input.Objective, request)
	}
	if strings.Contains(input.TechnicalContext, sentinel) {
		t.Fatalf("technical context received request authority: %q", input.TechnicalContext)
	}

	var targetPrompt string
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, _ string, prompt string) (string, error) {
			targetPrompt = prompt
			return "ROOT\n  D workload\n    F capability.test.tsx\n    F capability.tsx", nil
		}),
	}
	target, coverage, err := resolveDirectCodingTargetTree(
		runtime, "tree-model", "tree-model", request,
		specification, workload, stack, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(targetPrompt, "ACCEPTED_GOALS:\n"+request) ||
		!strings.Contains(targetPrompt, "CODE_SELECTED_TECHNICAL_CONTEXT:\n"+input.TechnicalContext) {
		t.Fatalf("semantic target-tree prompt lost separated request or technical authority:\n%s", targetPrompt)
	}

	target.VersionProfileID = stack.DefaultVersionProfileID
	program, err := compileDirectCodingProgram(
		"request-boundary", specification, nil,
		map[string]directCodingSkillBinding{}, workload,
		directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, document := range program.Source.Documents {
		for _, block := range document.Blocks {
			if strings.Contains(block.Contract, sentinel) {
				t.Fatalf("source block %s received raw request sentinel: %s", block.ID, block.Contract)
			}
		}
	}
	ref := directCodingTestGeneratedBlockRef(t, program.Source, "feature.001")
	fragmentInput, err := directCodingLanguageFragmentInput(&program, ref, "typescript")
	if err != nil {
		t.Fatal(err)
	}
	job, err := assemblyline.NewFragmentGenerationJob(fragmentInput)
	if err != nil {
		t.Fatal(err)
	}
	sourcePrompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sourcePrompt, sentinel) {
		t.Fatalf("initial source prompt received raw request sentinel:\n%s", sourcePrompt)
	}
}
