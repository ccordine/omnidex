package omni

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunRecipeCompletionProbesPassesObservableChecks(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recipe := Recipe{
		ID: "test.recipe",
		CompletionChecks: []string{
			"test -f package.json",
		},
	}
	results, passed := RunRecipeCompletionProbes(context.Background(), recipe, workspace)
	if !passed {
		t.Fatalf("checks should pass: %#v", results)
	}
	if len(results) != 1 || results[0].ExitCode != 0 {
		t.Fatalf("unexpected probe results: %#v", results)
	}
}

func TestApplyRecipeProbeCompletionSatisfiesRecipeObjectives(t *testing.T) {
	recipe := Recipe{
		ID: "test.recipe",
		Objectives: []RecipeObjective{
			{ID: "a", Description: "A"},
			{ID: "b", Description: "B"},
		},
	}
	ledger := RecipeObjectiveLedger(recipe)
	ledger = ApplyRecipeProbeCompletion(ledger, recipe, true, "probes passed")
	if pending := pendingStructuredObjectiveIDs(ledger); pending != "" {
		t.Fatalf("pending objectives = %s", pending)
	}
}
