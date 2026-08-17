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
