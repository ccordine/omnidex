package omni

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRecipePageUsesExactBounds(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < 5; index++ {
		body := fmt.Sprintf(`{
			"id":"recipe-%d","description":"Recipe %d",
			"objectives":[{"id":"objective","description":"Do work"}],
			"allowed_commands":["go test ./..."],"evidence_required":["tests pass"]
		}`, index, index)
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("%02d.json", index)), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	first, err := LoadRecipePage(root, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadRecipePage(root, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	last, err := LoadRecipePage(root, 2, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Recipes) != 2 || !first.HasMore || first.Recipes[0].ID != "recipe-0" ||
		len(second.Recipes) != 2 || !second.HasMore || second.Recipes[0].ID != "recipe-2" ||
		len(last.Recipes) != 1 || last.HasMore || last.Recipes[0].ID != "recipe-4" {
		t.Fatalf("pages first=%+v second=%+v last=%+v", first, second, last)
	}
	exact, err := LoadRecipeByID(root, "recipe-3")
	if err != nil || exact.ID != "recipe-3" {
		t.Fatalf("recipe=%+v err=%v", exact, err)
	}
}

func TestLoadRecipePageRejectsInvalidBounds(t *testing.T) {
	root := t.TempDir()
	if _, err := LoadRecipePage(root, 0, 0); err == nil {
		t.Fatal("expected zero limit to fail")
	}
	if _, err := LoadRecipePage(root, 1, -1); err == nil {
		t.Fatal("expected negative offset to fail")
	}
}
