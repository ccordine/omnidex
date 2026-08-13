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
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Options     []string `json:"options,omitempty"`
	Value       string   `json:"value,omitempty"`
}

type scrumModalRenderContext struct {
	Card                ScrumCard
	Board               ScrumBoard
	ProjectID           int64
	Tab                 string
	Files               []string
	Dirs                []string
	PlayQueue           map[string]any
	ModelFields         []scrumConfigField
	ModelSource         string
	ModelOverrides      map[string]string
	AgentFields         []scrumConfigField
	AgentSource         string
	AgentSystem         string
	AgentOverrides      map[string]string
	Recipes             []omni.Recipe
	RecipeOffset        int
	RecipeHasMore       bool
	ProjectRecipeID     string
	ProjectRecipe       map[string]any
	PilotPending        bool
	ChannelBeforeCursor string
	ChannelHasMore      bool
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
	stored, err := s.repo.GetScrumCard(r.Context(), projectID, cardID)
	if err != nil {
		return nil, fmt.Errorf("card not found: %w", err)
	}
	card, err := dbScrumCardToAPI(stored)
	if err != nil {
		return nil, err
	}
	playQueue, err := s.scrumPlayQueuePayload(r.Context(), projectID)
	if err != nil {
		return nil, err
	}
	channelPage, err := scrumChannelMessagePageFor(card, scrumChannelDefaultPageSize, "")
	if err != nil {
		return nil, err
	}
	card.Chat = channelPage.Messages
	card.ChatCount = channelPage.Total
	card.ConsoleLog = ""
	board.Cards = []ScrumCard{}
	ctx := &scrumModalRenderContext{
		Card:                card,
		Board:               board,
		ProjectID:           projectID,
		Tab:                 normalizeScrumModalTab(tab),
		PlayQueue:           playQueue,
		ChannelBeforeCursor: channelPage.BeforeCursor,
		ChannelHasMore:      channelPage.HasMore,
	}
	if ctx.Tab == "" {
		ctx.Tab = "card"
	}
	if root := strings.TrimSpace(board.ProjectDirectory); root != "" {
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("resolve project directory %q: %w", root, err)
		}
		ctx.Files, ctx.Dirs, err = scrumListProjectFiles(abs)
		if err != nil {
			return nil, err
		}
	}
	modelResolved, modelErr := s.resolvedModelsForProjectCard(r.Context(), projectID, card)
	if modelErr != nil {
		return nil, modelErr
	}
	ctx.ModelFields, err = scrumConfigFieldsFromList(modelResolved["fields"])
	if err != nil {
		return nil, fmt.Errorf("decode resolved model fields: %w", err)
	}
	ctx.ModelSource = scrumConfigString(modelResolved, "source")
	ctx.ModelOverrides, err = modelConfigStringMap(card.ModelConfig)
	if err != nil {
		return nil, err
	}
	agentResolved, agentErr := s.resolvedAgentsForProjectCard(r.Context(), projectID, card)
	if agentErr != nil {
		return nil, agentErr
	}
	ctx.AgentFields, err = scrumConfigFieldsFromList(agentResolved["fields"])
	if err != nil {
		return nil, fmt.Errorf("decode resolved agent fields: %w", err)
	}
	ctx.AgentSource = scrumConfigString(agentResolved, "source")
	ctx.AgentSystem = scrumConfigString(agentResolved, "system")
	if ctx.AgentSystem == "" {
		return nil, fmt.Errorf("resolved agent system is empty")
	}
	ctx.AgentOverrides, err = agentConfigStringMap(card.AgentConfig)
	if err != nil {
		return nil, err
	}
	recipeOffset, err := exactChannelQueryInteger(r, "recipe_offset", 0, 0, 1<<30)
	if err != nil {
		return nil, err
	}
	recipePage, err := omni.LoadRecipePage(s.recipeRoot(), dataSourceUIPageSize, recipeOffset)
	if err != nil {
		return nil, fmt.Errorf("load Scrum recipes: %w", err)
	}
	ctx.Recipes = recipePage.Recipes
	ctx.RecipeOffset = recipePage.Offset
	ctx.RecipeHasMore = recipePage.HasMore
	if s.repo == nil || projectID <= 0 {
		return nil, fmt.Errorf("PostgreSQL project repository is required for Scrum modal context")
	}
	project, err := s.repo.GetProject(r.Context(), projectID)
	if err != nil {
		return nil, fmt.Errorf("load Scrum project %d: %w", projectID, err)
	}
	ctx.ProjectRecipeID = strings.TrimSpace(project.RecipeID)
	ctx.ProjectRecipe, err = jsonRawObjectMap(project.Recipe)
	if err != nil {
		return nil, fmt.Errorf("decode project recipe: %w", err)
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

func scrumConfigFieldsFromList(raw any) ([]scrumConfigField, error) {
	items, ok := raw.([]map[string]any)
	if !ok {
		return nil, fmt.Errorf("fields must be []map[string]any, received %T", raw)
	}
	fields := make([]scrumConfigField, 0, len(items))
	for index, item := range items {
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
				text, ok := option.(string)
				if !ok || strings.TrimSpace(text) == "" {
					return nil, fmt.Errorf("field %d option must be a non-empty string", index)
				}
				field.Options = append(field.Options, text)
			}
		case nil:
		default:
			return nil, fmt.Errorf("field %d options must be a string array, received %T", index, opts)
		}
		if field.Key == "" || field.Label == "" {
			return nil, fmt.Errorf("field %d requires key and label", index)
		}
		fields = append(fields, field)
	}
	return fields, nil
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

func modelConfigStringMap(raw json.RawMessage) (map[string]string, error) {
	cfg, err := modelconfig.FromJSON(raw)
	if err != nil {
		return nil, err
	}
	if len(cfg) == 0 {
		return map[string]string{}, nil
	}
	return cfg.ToMap(), nil
}

func agentConfigStringMap(raw json.RawMessage) (map[string]string, error) {
	cfg, err := agentconfig.FromJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("parse card agent configuration: %w", err)
	}
	if len(cfg) == 0 {
		return map[string]string{}, nil
	}
	return cfg.ToMap(), nil
}

func jsonRawObjectMap(raw json.RawMessage) (map[string]any, error) {
	if len(raw) <= 2 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return nil, fmt.Errorf("expected JSON object, received null")
	}
	return out, nil
}

func scrumListProjectFiles(root string) (files, dirs []string, err error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, nil, fmt.Errorf("project root is required")
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
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
	if err != nil {
		return nil, nil, fmt.Errorf("list project files under %q: %w", root, err)
	}
	return files, dirs, nil
}
