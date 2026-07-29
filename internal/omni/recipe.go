package omni

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Recipe struct {
	ID                      string            `json:"id"`
	Description             string            `json:"description"`
	Operation               string            `json:"operation,omitempty"`
	RequiredProjectStates   []string          `json:"required_project_states,omitempty"`
	ForbiddenUserOperations []string          `json:"forbidden_user_operations,omitempty"`
	Objectives              []RecipeObjective `json:"objectives"`
	AllowedCommands         []string          `json:"allowed_commands"`
	EvidenceRequired        []string          `json:"evidence_required"`
	CompletionChecks        []string          `json:"completion_checks"`
}

type RecipeObjective struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	DependsOn   []string `json:"depends_on,omitempty"`
	Packages    []string `json:"packages,omitempty"`
}

func LoadRecipes(root string) ([]Recipe, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("recipe directory is required")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read recipe directory %s: %w", root, err)
	}
	recipes := make([]Recipe, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		recipe, err := LoadRecipeFile(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, err
		}
		recipes = append(recipes, recipe)
	}
	sort.Slice(recipes, func(i, j int) bool { return recipes[i].ID < recipes[j].ID })
	return recipes, nil
}

func LoadRecipeFile(path string) (Recipe, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return Recipe{}, fmt.Errorf("read recipe %s: %w", path, err)
	}
	var recipe Recipe
	if err := json.Unmarshal(blob, &recipe); err != nil {
		return Recipe{}, fmt.Errorf("decode recipe %s: %w", path, err)
	}
	if err := validateRecipe(recipe); err != nil {
		return Recipe{}, fmt.Errorf("invalid recipe %s: %w", path, err)
	}
	return recipe, nil
}

func validateRecipe(recipe Recipe) error {
	if strings.TrimSpace(recipe.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if strings.TrimSpace(recipe.Description) == "" {
		return fmt.Errorf("description is required")
	}
	if len(recipe.Objectives) == 0 {
		return fmt.Errorf("at least one objective is required")
	}
	known := make(map[string]struct{}, len(recipe.Objectives))
	for _, objective := range recipe.Objectives {
		id := strings.TrimSpace(objective.ID)
		if id == "" || strings.TrimSpace(objective.Description) == "" {
			return fmt.Errorf("every objective requires an id and description")
		}
		if _, duplicate := known[id]; duplicate {
			return fmt.Errorf("duplicate objective id %q", id)
		}
		known[id] = struct{}{}
	}
	for _, objective := range recipe.Objectives {
		for _, dependency := range objective.DependsOn {
			dependency = strings.TrimSpace(dependency)
			if _, exists := known[dependency]; !exists {
				return fmt.Errorf("objective %q depends on unknown objective %q", objective.ID, dependency)
			}
		}
	}
	if err := validateRecipeObjectiveDAG(recipe.Objectives); err != nil {
		return err
	}
	if len(recipe.AllowedCommands) == 0 || len(recipe.EvidenceRequired) == 0 {
		return fmt.Errorf("allowed_commands and evidence_required are required")
	}
	return nil
}

func validateRecipeObjectiveDAG(objectives []RecipeObjective) error {
	byID := make(map[string]RecipeObjective, len(objectives))
	for _, objective := range objectives {
		byID[objective.ID] = objective
	}
	visiting := make(map[string]bool, len(objectives))
	visited := make(map[string]bool, len(objectives))
	var visit func(string) error
	visit = func(id string) error {
		if visited[id] {
			return nil
		}
		if visiting[id] {
			return fmt.Errorf("objective dependency cycle includes %q", id)
		}
		visiting[id] = true
		for _, dependency := range byID[id].DependsOn {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		return nil
	}
	for _, objective := range objectives {
		if err := visit(objective.ID); err != nil {
			return err
		}
	}
	return nil
}
