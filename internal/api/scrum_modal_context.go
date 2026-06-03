package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/omni"
)

type scrumConfigField struct {
	Key         string
	Label       string
	Description string
	Options     []string
	Value       string
}

type scrumModalRenderContext struct {
	Card            ScrumCard
	Board           ScrumBoard
	ProjectID       int64
	Tab             string
	Files           []string
	Dirs            []string
	PlayQueue       map[string]any
	ModelFields     []scrumConfigField
	ModelSource     string
	ModelOverrides  map[string]string
	AgentFields     []scrumConfigField
	AgentSource     string
	AgentSystem     string
	AgentOverrides  map[string]string
	Recipes         []omni.Recipe
	ProjectRecipeID string
	ProjectRecipe   map[string]any
	PilotPending    bool
}

func scrumPlayControlUnlocked(card ScrumCard) bool {
	if normalizeScrumColumn(card.Column) == "assigned" {
		return true
	}
	switch strings.TrimSpace(card.PlayState) {
	case "running", "queued", "paused":
		return true
	default:
		return false
	}
}

func (s *Server) buildScrumModalContext(r *http.Request, cardID, tab string) (*scrumModalRenderContext, error) {
	cardID = strings.TrimSpace(cardID)
	if cardID == "" {
		return nil, fmt.Errorf("card id is required")
	}
	board, projectID, err := s.loadScrumContext(r)
	if err != nil {
		return nil, err
	}
	board, err = s.refreshScrumPlayQueue(r, projectID, board)
	if err != nil {
		return nil, err
	}
	board, err = s.refreshScrumCardLlmJobs(r.Context(), projectID, board)
	if err != nil {
		return nil, err
	}
	cardPtr := findScrumCard(board, cardID)
	if cardPtr == nil {
		return nil, fmt.Errorf("card not found")
	}
	card := *cardPtr
	ctx := &scrumModalRenderContext{
		Card:      card,
		Board:     board,
		ProjectID: projectID,
		Tab:       normalizeScrumModalTab(tab),
		PlayQueue: scrumPlayQueueSummary(board),
	}
	if ctx.Tab == "" {
		ctx.Tab = "card"
	}
	if root := strings.TrimSpace(board.ProjectDirectory); root != "" {
		if abs, absErr := filepath.Abs(root); absErr == nil {
			ctx.Files, ctx.Dirs = scrumListProjectFiles(abs)
		}
	}
	modelResolved, modelErr := s.resolvedModelsForProjectCard(r.Context(), projectID, card)
	if modelErr != nil {
		return nil, modelErr
	}
	ctx.ModelFields = scrumConfigFieldsFromList(modelResolved["fields"])
	ctx.ModelSource = scrumConfigString(modelResolved, "source")
	ctx.ModelOverrides = modelConfigStringMap(card.ModelConfig)
	agentResolved, agentErr := s.resolvedAgentsForProjectCard(r.Context(), projectID, card)
	if agentErr != nil {
		return nil, agentErr
	}
	ctx.AgentFields = scrumConfigFieldsFromList(agentResolved["fields"])
	ctx.AgentSource = scrumConfigString(agentResolved, "source")
	ctx.AgentSystem = scrumConfigString(agentResolved, "system")
	if ctx.AgentSystem == "" {
		ctx.AgentSystem = agentconfig.SystemOmnidex
	}
	ctx.AgentOverrides = agentConfigStringMap(card.AgentConfig)
	if recipes, loadErr := omni.LoadRecipes(s.recipeRoot()); loadErr == nil {
		ctx.Recipes = recipes
	}
	if projectID > 0 && s.repo != nil {
		if project, projErr := s.repo.GetProject(r.Context(), projectID); projErr == nil {
			ctx.ProjectRecipeID = strings.TrimSpace(project.RecipeID)
			ctx.ProjectRecipe = jsonRawObjectMap(project.Recipe)
		}
	}
	return ctx, nil
}

func normalizeScrumModalTab(tab string) string {
	tab = strings.TrimSpace(strings.ToLower(tab))
	switch tab {
	case "card", "files", "tests", "config", "recipe", "channel":
		return tab
	default:
		return ""
	}
}

func scrumConfigFieldsFromList(raw any) []scrumConfigField {
	items, ok := raw.([]map[string]any)
	if !ok {
		return nil
	}
	fields := make([]scrumConfigField, 0, len(items))
	for _, item := range items {
		field := scrumConfigField{
			Key:         scrumConfigString(item, "key"),
			Label:       scrumConfigString(item, "label"),
			Description: scrumConfigString(item, "description"),
			Value:       scrumConfigString(item, "value"),
		}
		switch opts := item["options"].(type) {
		case []string:
			field.Options = append([]string(nil), opts...)
		case []any:
			for _, option := range opts {
				if text, ok := option.(string); ok && strings.TrimSpace(text) != "" {
					field.Options = append(field.Options, text)
				}
			}
		}
		fields = append(fields, field)
	}
	return fields
}

func scrumConfigString(payload any, key string) string {
	switch typed := payload.(type) {
	case map[string]any:
		value, _ := typed[key].(string)
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func modelConfigStringMap(raw json.RawMessage) map[string]string {
	cfg := modelconfig.FromJSON(raw)
	if len(cfg) == 0 {
		return map[string]string{}
	}
	return cfg.ToMap()
}

func agentConfigStringMap(raw json.RawMessage) map[string]string {
	cfg := agentconfig.FromJSON(raw)
	if len(cfg) == 0 {
		return map[string]string{}
	}
	return cfg.ToMap()
}

func jsonRawObjectMap(raw json.RawMessage) map[string]any {
	if len(raw) <= 2 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func scrumListProjectFiles(root string) (files, dirs []string) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, nil
	}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.Count(rel, string(os.PathSeparator)) > 2 {
				return filepath.SkipDir
			}
			dirs = append(dirs, filepath.ToSlash(rel))
			return nil
		}
		if strings.Count(rel, string(os.PathSeparator)) > 3 {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		if len(files) >= 200 {
			return fs.SkipAll
		}
		return nil
	})
	return files, dirs
}
