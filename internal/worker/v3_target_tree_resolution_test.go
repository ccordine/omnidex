package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTargetTreeResolutionUsesOneBoundedStructureDeclaration(t *testing.T) {
	specification := assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceBrowser,
		ProductQuote: "counter application",
		Requirements: []assemblyline.Requirement{{ID: "requirement_001", SourceQuote: "show and change a count"}},
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, model, prompt string, _ map[string]any) (string, error) {
			calls++
			if model != "planner-model" {
				t.Fatalf("model=%q", model)
			}
			for _, forbidden := range []string{"mkdir", "write_file", "task ledger", "completion", "tool"} {
				if strings.Contains(strings.ToLower(prompt), forbidden) {
					t.Fatalf("target-tree prompt leaks %q: %s", forbidden, prompt)
				}
			}
			return `{"schema":"omnidex.target-tree.v1","artifacts":[{"path":"ui/counter.tsx","kind":"implementation","purpose":"implement the counter behavior","requirement_ids":["requirement_001"],"existing_artifact_id":"","new_key":"counter_source"},{"path":"checks/counter.test.tsx","kind":"verification","purpose":"verify the counter behavior","requirement_ids":["requirement_001"],"existing_artifact_id":"","new_key":"counter_test"}]}`, nil
		}),
	}
	target, err := resolveDirectCodingTargetTree(runtime, "planner-model", specification, []assemblyline.CurrentTargetArtifact{})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("structure calls=%d, want one", calls)
	}
	files, err := target.RequirementFiles("requirement_001")
	if err != nil {
		t.Fatal(err)
	}
	if files.ImplementationPath != "ui/counter.tsx" || files.VerificationPath != "checks/counter.test.tsx" {
		t.Fatalf("resolved files=%+v", files)
	}
}
