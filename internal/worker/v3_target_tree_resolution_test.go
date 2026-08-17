package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTargetTreeResolutionUsesOnlyPathTree(t *testing.T) {
	runtime := typedWorkerRuntime{Context: context.Background(), MaxAttempts: 1, Execute: testPortableExecutor(func(_ string, model, prompt string, _ map[string]any) (string, error) {
		if model != "tree-model" || strings.Contains(strings.ToLower(prompt), "requirement") || !strings.Contains(prompt, "TypeScript React") {
			t.Fatalf("tree model=%q prompt=%s", model, prompt)
		}
		return `{"schema":"omnidex.target-tree.v1","paths":["src/counter.tsx","tests/counter.test.tsx"]}`, nil
	})}
	specification := assemblyline.ApplicationSpecification{Surface: assemblyline.ApplicationSurfaceBrowser, ProductQuote: "counter application"}
	tree, err := resolveDirectCodingTargetTree(runtime, "tree-model", "review-model", specification, []string{}, []string{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(tree.Paths, ",") != "src/counter.tsx,tests/counter.test.tsx" {
		t.Fatalf("paths=%v", tree.Paths)
	}
}

func TestTargetTreeResolutionCorrectsUnsupportedLeafBeforeContentWork(t *testing.T) {
	calls := 0
	runtime := typedWorkerRuntime{Context: context.Background(), MaxAttempts: 2, Execute: testPortableExecutor(func(_ string, model, prompt string, _ map[string]any) (string, error) {
		calls++
		switch calls {
		case 1:
			if model != "tree-model" {
				t.Fatalf("initial model=%q", model)
			}
			return `{"schema":"omnidex.target-tree.v1","paths":["src/styles/globals.css"]}`, nil
		case 2:
			if model != "review-model" {
				t.Fatalf("correction model=%q", model)
			}
			for _, expected := range []string{"CURRENT_TARGET_TREE_CANDIDATE_JSON", "VALIDATION_FAILURE", "src/styles/globals.css", "selected project stack"} {
				if !strings.Contains(prompt, expected) {
					t.Fatalf("correction prompt missing %q: %s", expected, prompt)
				}
			}
			return `{"schema":"omnidex.target-tree.v1","paths":["src/counter.tsx","tests/counter.test.tsx"]}`, nil
		default:
			t.Fatalf("unexpected call %d", calls)
			return "", nil
		}
	})}
	specification := assemblyline.ApplicationSpecification{Surface: assemblyline.ApplicationSurfaceBrowser, ProductQuote: "counter application"}
	tree, err := resolveDirectCodingTargetTree(runtime, "tree-model", "review-model", specification, []string{}, []string{})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || strings.Join(tree.Paths, ",") != "src/counter.tsx,tests/counter.test.tsx" {
		t.Fatalf("calls=%d paths=%v", calls, tree.Paths)
	}
}
