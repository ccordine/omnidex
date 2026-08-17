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
	target, err := resolveDirectCodingTargetTree(runtime, "planner-model", "reviewer-model", specification, []assemblyline.CurrentTargetArtifact{})
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

func TestTargetTreeValidationUsesCompleteCorrectiveDeclaration(t *testing.T) {
	specification := assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceBrowser,
		ProductQuote: "counter application",
		Requirements: []assemblyline.Requirement{{ID: "requirement_001", SourceQuote: "show and change a count"}},
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 2,
		Execute: testPortableExecutor(func(_ string, model, prompt string, _ map[string]any) (string, error) {
			calls++
			switch calls {
			case 1:
				if model != "planner-model" {
					t.Fatalf("initial model=%q", model)
				}
				return `{"schema":"omnidex.target-tree.v1","artifacts":[{"path":"ui/counter.tsx","kind":"implementation","purpose":"implement the counter behavior","requirement_ids":["requirement_001"],"existing_artifact_id":"","new_key":"counter_source"}]}`, nil
			case 2:
				if model != "reviewer-model" {
					t.Fatalf("correction model=%q", model)
				}
				for _, required := range []string{"CURRENT_TARGET_TREE_CANDIDATE_JSON", "VALIDATION_FAILURE", "counter_source"} {
					if !strings.Contains(prompt, required) {
						t.Fatalf("corrective prompt lacks %q: %s", required, prompt)
					}
				}
				for _, forbidden := range []string{"merge patch", "one top-level field", "one invalid leaf"} {
					if strings.Contains(strings.ToLower(prompt), forbidden) {
						t.Fatalf("corrective prompt leaks obsolete %q: %s", forbidden, prompt)
					}
				}
				return `{"schema":"omnidex.target-tree.v1","artifacts":[{"path":"ui/counter.tsx","kind":"implementation","purpose":"implement the counter behavior","requirement_ids":["requirement_001"],"existing_artifact_id":"","new_key":"counter_source"},{"path":"checks/counter.test.tsx","kind":"verification","purpose":"verify the counter behavior","requirement_ids":["requirement_001"],"existing_artifact_id":"","new_key":"counter_test"}]}`, nil
			default:
				t.Fatalf("unexpected call %d", calls)
				return "", nil
			}
		}),
	}
	target, err := resolveDirectCodingTargetTree(runtime, "planner-model", "reviewer-model", specification, []assemblyline.CurrentTargetArtifact{})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d, want 2", calls)
	}
	if _, err := target.RequirementFiles("requirement_001"); err != nil {
		t.Fatal(err)
	}
}
