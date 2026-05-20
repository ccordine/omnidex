package omni

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFrontendRecipeManifest(t *testing.T) {
	root := repoRootFromOmniTest(t)
	recipe, err := LoadRecipeFile(filepath.Join(root, "recipes", "frontend.stimulus-tailwind-recyclr.json"))
	if err != nil {
		t.Fatal(err)
	}
	if recipe.ID != "frontend.stimulus-tailwind-recyclr" {
		t.Fatalf("recipe id = %q", recipe.ID)
	}
	dependencies := map[string][]string{}
	for _, objective := range recipe.Objectives {
		dependencies[objective.ID] = objective.DependsOn
	}
	if len(dependencies["verify_build"]) == 0 {
		t.Fatal("verify_build should depend on earlier recipe objectives")
	}
	ledger := RecipeObjectiveLedger(recipe)
	if len(ledger) != len(recipe.Objectives) {
		t.Fatalf("ledger objectives = %d, want %d", len(ledger), len(recipe.Objectives))
	}
	for _, objective := range ledger {
		if objective.Status != "pending" {
			t.Fatalf("objective %s status = %q", objective.ID, objective.Status)
		}
	}
}

func TestRecipeValidationRejectsDependencyCycles(t *testing.T) {
	recipe := Recipe{
		ID:               "cycle.recipe",
		Description:      "cycle recipe",
		AllowedCommands:  []string{"true"},
		EvidenceRequired: []string{"evidence"},
		Objectives: []RecipeObjective{
			{ID: "a", Description: "A", DependsOn: []string{"b"}},
			{ID: "b", Description: "B", DependsOn: []string{"a"}},
		},
	}
	if err := validateRecipe(recipe); err == nil {
		t.Fatal("expected dependency cycle to be rejected")
	}
}

func TestLoadRecipesSortsAndValidates(t *testing.T) {
	root := repoRootFromOmniTest(t)
	recipes, err := LoadRecipes(filepath.Join(root, "recipes"))
	if err != nil {
		t.Fatal(err)
	}
	if len(recipes) == 0 {
		t.Fatal("expected at least one recipe")
	}
	for i := 1; i < len(recipes); i++ {
		if recipes[i-1].ID > recipes[i].ID {
			t.Fatalf("recipes not sorted: %q before %q", recipes[i-1].ID, recipes[i].ID)
		}
	}
}

func TestLoadOptionalRecipesFindsInstalledRecipeRoot(t *testing.T) {
	installRoot := t.TempDir()
	recipeDir := filepath.Join(installRoot, "recipes")
	if err := os.MkdirAll(recipeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	recipe := `{
  "id": "installed.recipe",
  "description": "installed recipe discovery",
  "allowed_commands": ["true"],
  "evidence_required": ["recipe loaded"],
  "objectives": [
    {
      "id": "verify_recipe_discovery",
      "description": "Verify recipe discovery"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(recipeDir, "installed.recipe.json"), []byte(recipe), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OMNIDEX_DIR", installRoot)
	cwd := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	recipes := loadOptionalRecipes("recipes")
	if len(recipes) != 1 {
		t.Fatalf("recipes loaded = %d, want 1", len(recipes))
	}
	if recipes[0].ID != "installed.recipe" {
		t.Fatalf("recipe id = %q", recipes[0].ID)
	}
}
