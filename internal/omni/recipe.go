package omni

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

const MaxRecipePageSize = 100

type RecipePage struct {
	Recipes []Recipe `json:"recipes"`
	Offset  int      `json:"offset"`
	HasMore bool     `json:"has_more"`
}

func LoadRecipePage(root string, limit, offset int) (RecipePage, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return RecipePage{}, fmt.Errorf("recipe directory is required")
	}
	if limit < 1 || limit > MaxRecipePageSize {
		return RecipePage{}, fmt.Errorf("recipe page limit must be between 1 and %d", MaxRecipePageSize)
	}
	if offset < 0 {
		return RecipePage{}, fmt.Errorf("recipe page offset must be non-negative")
	}
	directory, err := os.Open(root)
	if err != nil {
		return RecipePage{}, fmt.Errorf("open recipe directory %s: %w", root, err)
	}
	defer directory.Close()
	recipes := make([]Recipe, 0, limit+1)
	matched := 0
	for len(recipes) < limit+1 {
		entries, readErr := directory.ReadDir(64)
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			if matched < offset {
				matched++
				continue
			}
			recipe, err := LoadRecipeFile(filepath.Join(root, entry.Name()))
			if err != nil {
				return RecipePage{}, err
			}
			recipes = append(recipes, recipe)
			matched++
			if len(recipes) == limit+1 {
				break
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return RecipePage{}, fmt.Errorf("read recipe directory %s: %w", root, readErr)
		}
	}
	hasMore := len(recipes) > limit
	if hasMore {
		recipes = recipes[:limit]
	}
	return RecipePage{Recipes: recipes, Offset: offset, HasMore: hasMore}, nil
}

func LoadRecipeByID(root, id string) (Recipe, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Recipe{}, fmt.Errorf("recipe id is required")
	}
	for offset := 0; ; {
		page, err := LoadRecipePage(root, MaxRecipePageSize, offset)
		if err != nil {
			return Recipe{}, err
		}
		for _, recipe := range page.Recipes {
			if recipe.ID == id {
				return recipe, nil
			}
		}
		if !page.HasMore {
			return Recipe{}, fmt.Errorf("recipe %q was not found", id)
		}
		if len(page.Recipes) == 0 {
			return Recipe{}, fmt.Errorf("recipe pagination reported more entries without returning an entry")
		}
		offset += len(page.Recipes)
	}
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
