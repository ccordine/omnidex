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

type RecipePromptCandidate struct {
	ID               string   `json:"id"`
	Description      string   `json:"description"`
	Operation        string   `json:"operation,omitempty"`
	ObjectiveIDs     []string `json:"objective_ids"`
	EvidenceRequired []string `json:"evidence_required,omitempty"`
}

type RecipeRuntimeConstraint struct {
	ID               string   `json:"id"`
	Description      string   `json:"description"`
	Operation        string   `json:"operation,omitempty"`
	AllowedCommands  []string `json:"allowed_commands,omitempty"`
	EvidenceRequired []string `json:"evidence_required,omitempty"`
	CompletionChecks []string `json:"completion_checks,omitempty"`
}

func LoadRecipes(root string) ([]Recipe, error) {
	dir := strings.TrimSpace(root)
	if dir == "" {
		dir = "recipes"
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read recipe directory: %w", err)
	}
	recipes := []Recipe{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		recipe, err := LoadRecipeFile(filepath.Join(dir, entry.Name()))
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

func RecipeObjectiveLedger(recipe Recipe) []StructuredObjective {
	ledger := make([]StructuredObjective, 0, len(recipe.Objectives))
	for _, objective := range recipe.Objectives {
		ledger = append(ledger, StructuredObjective{
			ID:          objective.ID,
			Description: objective.Description,
			Status:      "pending",
			Source:      structuredObjectiveSourceRecipeRequired,
			Required:    true,
			Packages:    cleanStringList(objective.Packages),
		})
	}
	return ledger
}

func SelectRecipesByID(recipes []Recipe, ids []string) []Recipe {
	wanted := map[string]struct{}{}
	for _, id := range ids {
		clean := strings.TrimSpace(id)
		if clean != "" {
			wanted[clean] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	selected := []Recipe{}
	for _, recipe := range recipes {
		if _, ok := wanted[recipe.ID]; ok {
			selected = append(selected, recipe)
		}
	}
	return selected
}

func FilterRecipesForWorksiteSurvey(recipes []Recipe, survey WorksiteSurvey) []Recipe {
	if len(recipes) == 0 {
		return nil
	}
	out := []Recipe{}
	for _, recipe := range recipes {
		if RecipeAllowedByWorksiteSurvey(recipe, survey) {
			out = append(out, recipe)
		}
	}
	return out
}

func RecipeAllowedByWorksiteSurvey(recipe Recipe, survey WorksiteSurvey) bool {
	if stringListContains(cleanStringList(survey.ForbiddenRecipeIDs), strings.TrimSpace(recipe.ID)) {
		return false
	}
	operation := normalizeUserOperation(recipe.Operation)
	if survey.UserOperation != "" && survey.UserOperation != userOperationUnknown && operation != userOperationUnknown && operation != survey.UserOperation {
		return false
	}
	forbidden := cleanStringList(recipe.ForbiddenUserOperations)
	if survey.UserOperation != "" && stringListContains(forbidden, survey.UserOperation) {
		return false
	}
	requiredStates := cleanStringList(recipe.RequiredProjectStates)
	if len(requiredStates) > 0 && !stringListContains(requiredStates, survey.ProjectState) {
		return false
	}
	return true
}

func stringListContains(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func recipeIDs(recipes []Recipe) []string {
	ids := make([]string, 0, len(recipes))
	for _, recipe := range recipes {
		if strings.TrimSpace(recipe.ID) != "" {
			ids = append(ids, recipe.ID)
		}
	}
	return ids
}

func recipePromptCandidates(recipes []Recipe) []RecipePromptCandidate {
	out := make([]RecipePromptCandidate, 0, len(recipes))
	for _, recipe := range recipes {
		objectiveIDs := []string{}
		for _, objective := range recipe.Objectives {
			objectiveIDs = append(objectiveIDs, objective.ID)
		}
		out = append(out, RecipePromptCandidate{
			ID:               recipe.ID,
			Description:      recipe.Description,
			Operation:        recipe.Operation,
			ObjectiveIDs:     objectiveIDs,
			EvidenceRequired: recipe.EvidenceRequired,
		})
	}
	return out
}

func recipeRuntimeConstraints(recipes []Recipe) []RecipeRuntimeConstraint {
	out := make([]RecipeRuntimeConstraint, 0, len(recipes))
	for _, recipe := range recipes {
		out = append(out, RecipeRuntimeConstraint{
			ID:               recipe.ID,
			Description:      recipe.Description,
			Operation:        recipe.Operation,
			AllowedCommands:  recipe.AllowedCommands,
			EvidenceRequired: recipe.EvidenceRequired,
			CompletionChecks: recipe.CompletionChecks,
		})
	}
	return out
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
	seen := map[string]struct{}{}
	for _, objective := range recipe.Objectives {
		id := strings.TrimSpace(objective.ID)
		if id == "" {
			return fmt.Errorf("objective id is required")
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("duplicate objective id %q", id)
		}
		seen[id] = struct{}{}
		if strings.TrimSpace(objective.Description) == "" {
			return fmt.Errorf("objective %q description is required", id)
		}
	}
	for _, objective := range recipe.Objectives {
		for _, dep := range objective.DependsOn {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				return fmt.Errorf("objective %q has empty dependency", objective.ID)
			}
			if _, ok := seen[dep]; !ok {
				return fmt.Errorf("objective %q depends on unknown objective %q", objective.ID, dep)
			}
			if dep == objective.ID {
				return fmt.Errorf("objective %q cannot depend on itself", objective.ID)
			}
		}
	}
	if err := validateRecipeObjectiveDAG(recipe.Objectives); err != nil {
		return err
	}
	if len(recipe.AllowedCommands) == 0 {
		return fmt.Errorf("allowed_commands is required")
	}
	if len(recipe.EvidenceRequired) == 0 {
		return fmt.Errorf("evidence_required is required")
	}
	return nil
}

func validateRecipeObjectiveDAG(objectives []RecipeObjective) error {
	byID := map[string]RecipeObjective{}
	for _, objective := range objectives {
		byID[objective.ID] = objective
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visited[id] {
			return nil
		}
		if visiting[id] {
			return fmt.Errorf("objective dependency cycle includes %q", id)
		}
		visiting[id] = true
		for _, dep := range byID[id].DependsOn {
			if err := visit(dep); err != nil {
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
