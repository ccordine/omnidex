package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/omni"
	"github.com/gryph/omnidex/internal/queue"
)

func (s *Server) recipeRoot() string {
	root := strings.TrimSpace(os.Getenv("OMNI_RECIPE_ROOT"))
	if root == "" {
		for _, candidate := range []string{"recipes", "../recipes", "../../recipes", "../../../recipes"} {
			if stat, err := os.Stat(candidate); err == nil && stat.IsDir() {
				if abs, err := filepath.Abs(candidate); err == nil {
					return abs
				}
			}
		}
		root = "recipes"
	}
	if abs, err := filepath.Abs(root); err == nil {
		return abs
	}
	return root
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "projects require database")
		return
	}
	switch r.Method {
	case http.MethodGet:
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		projects, err := s.repo.ListProjects(r.Context(), limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		activeID, _ := s.repo.GetActiveProjectID(r.Context())
		items := make([]map[string]any, 0, len(projects))
		for _, project := range projects {
			items = append(items, s.projectSummary(r.Context(), project, activeID))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"projects":            items,
			"active_project_id":   activeID,
		})
	case http.MethodPost:
		var req struct {
			Name        string          `json:"name"`
			Location    string          `json:"location"`
			Description string          `json:"description"`
			RecipeID    string          `json:"recipe_id"`
			Recipe      json.RawMessage `json:"recipe"`
			Activate    bool            `json:"activate"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		location, err := queue.NormalizeProjectLocation(req.Location)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if stat, err := os.Stat(location); err != nil || !stat.IsDir() {
			writeError(w, http.StatusBadRequest, "location must be an existing directory")
			return
		}
		recipe := req.Recipe
		if len(recipe) == 0 && strings.TrimSpace(req.RecipeID) != "" {
			recipe, _ = s.loadCatalogRecipeJSON(req.RecipeID)
		}
		project, err := s.repo.CreateProject(r.Context(), req.Name, location, req.Description, req.RecipeID, recipe)
		if err != nil {
			if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
				project, err = s.repo.GetProjectByLocation(r.Context(), location)
				if err != nil {
					writeError(w, http.StatusConflict, "project location already exists")
					return
				}
			} else {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		project, err = s.initializeProjectState(r.Context(), project)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if req.Activate {
			_ = s.repo.SetActiveProjectID(r.Context(), project.ID)
		}
		activeID, _ := s.repo.GetActiveProjectID(r.Context())
		writeJSON(w, http.StatusCreated, map[string]any{
			"project":           s.projectSummary(r.Context(), project, activeID),
			"active_project_id": activeID,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleProjectByID(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "projects require database")
		return
	}
	id, action := splitProjectPath(r.URL.Path)
	if id <= 0 {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if action == "activate" {
		s.handleProjectActivate(w, r, id)
		return
	}
	if action == "survey" {
		s.handleProjectSurvey(w, r, id)
		return
	}
	if action == "map" || action == "map/scan" {
		s.handleProjectMap(w, r, id, action)
		return
	}
	switch r.Method {
	case http.MethodGet:
		project, err := s.repo.GetProject(r.Context(), id)
		if err != nil {
			writeProjectError(w, err)
			return
		}
		activeID, _ := s.repo.GetActiveProjectID(r.Context())
		payload := map[string]any{"project": s.projectSummary(r.Context(), project, activeID)}
		if resolved, err := s.resolvedModelsForProjectCard(r.Context(), id, ScrumCard{}); err == nil {
			payload["model_config"] = resolved
		}
		if agentResolved, err := s.resolvedAgentsForProjectCard(r.Context(), id, ScrumCard{}); err == nil {
			payload["agent_config"] = agentResolved
		}
		writeJSON(w, http.StatusOK, payload)
	case http.MethodPatch:
		var req struct {
			Name         *string          `json:"name"`
			Location     *string          `json:"location"`
			Description  *string          `json:"description"`
			RecipeID     *string          `json:"recipe_id"`
			Recipe       *json.RawMessage `json:"recipe"`
			ProjectState *string          `json:"project_state"`
			Settings     *json.RawMessage `json:"settings"`
			ModelConfig  *json.RawMessage `json:"model_config"`
			AgentConfig  *json.RawMessage `json:"agent_config"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		patch := model.ProjectPatch{
			Name:         req.Name,
			Location:     req.Location,
			Description:  req.Description,
			RecipeID:     req.RecipeID,
			Recipe:       req.Recipe,
			ProjectState: req.ProjectState,
			Settings:     req.Settings,
		}
		if req.ModelConfig != nil || req.AgentConfig != nil {
			current, err := s.repo.GetProject(r.Context(), id)
			if err != nil {
				writeProjectError(w, err)
				return
			}
			settings := current.Settings
			if req.ModelConfig != nil {
				modelConfig, err := modelConfigPatchFromRequest(*req.ModelConfig)
				if err != nil {
					writeError(w, http.StatusBadRequest, err.Error())
					return
				}
				settings, err = mergeProjectModelConfig(settings, modelConfig)
				if err != nil {
					writeError(w, http.StatusInternalServerError, err.Error())
					return
				}
			}
			if req.AgentConfig != nil {
				agentConfig, err := agentConfigPatchFromRequest(*req.AgentConfig)
				if err != nil {
					writeError(w, http.StatusBadRequest, err.Error())
					return
				}
				settings, err = mergeProjectAgentConfig(settings, agentConfig)
				if err != nil {
					writeError(w, http.StatusInternalServerError, err.Error())
					return
				}
			}
			patch.Settings = &settings
		}
		if patch.Location != nil {
			location, err := queue.NormalizeProjectLocation(*patch.Location)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if stat, err := os.Stat(location); err != nil || !stat.IsDir() {
				writeError(w, http.StatusBadRequest, "location must be an existing directory")
				return
			}
			patch.Location = &location
		}
		project, err := s.repo.UpdateProject(r.Context(), id, patch)
		if err != nil {
			writeProjectError(w, err)
			return
		}
		activeID, _ := s.repo.GetActiveProjectID(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{"project": s.projectSummary(r.Context(), project, activeID)})
	case http.MethodDelete:
		if err := s.repo.DeleteProject(r.Context(), id); err != nil {
			writeProjectError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleProjectActivate(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.repo.SetActiveProjectID(r.Context(), id); err != nil {
		writeProjectError(w, err)
		return
	}
	project, err := s.repo.GetProject(r.Context(), id)
	if err != nil {
		writeProjectError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"active_project_id": id,
		"project":           s.projectSummary(r.Context(), project, id),
	})
}

func (s *Server) handleProjectSurvey(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	project, err := s.repo.GetProject(r.Context(), id)
	if err != nil {
		writeProjectError(w, err)
		return
	}
	project, err = s.initializeProjectState(r.Context(), project)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	activeID, _ := s.repo.GetActiveProjectID(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"project": s.projectSummary(r.Context(), project, activeID)})
}

func (s *Server) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "workspace requires database")
		return
	}
	switch r.Method {
	case http.MethodGet:
		activeID, err := s.repo.GetActiveProjectID(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		payload := map[string]any{"active_project_id": activeID}
		if activeID > 0 {
			if project, err := s.repo.GetProject(r.Context(), activeID); err == nil {
				payload["project"] = s.projectSummary(r.Context(), project, activeID)
			}
		}
		writeJSON(w, http.StatusOK, payload)
	case http.MethodPut:
		var req struct {
			ActiveProjectID int64 `json:"active_project_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if req.ActiveProjectID <= 0 {
			if err := s.repo.ClearActiveProjectID(r.Context()); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"active_project_id": 0})
			return
		}
		if err := s.repo.SetActiveProjectID(r.Context(), req.ActiveProjectID); err != nil {
			writeProjectError(w, err)
			return
		}
		project, _ := s.repo.GetProject(r.Context(), req.ActiveProjectID)
		writeJSON(w, http.StatusOK, map[string]any{
			"active_project_id": req.ActiveProjectID,
			"project":           s.projectSummary(r.Context(), project, req.ActiveProjectID),
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func splitProjectPath(path string) (id int64, action string) {
	path = strings.TrimPrefix(path, "/v1/projects/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return 0, ""
	}
	id, _ = strconv.ParseInt(parts[0], 10, 64)
	if len(parts) > 1 {
		action = strings.Join(parts[1:], "/")
	}
	return id, action
}

func writeProjectError(w http.ResponseWriter, err error) {
	if errors.Is(err, queue.ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

func extractSettingsModelConfig(settings json.RawMessage) json.RawMessage {
	cfg := modelconfig.FromSettingsJSON(settings)
	if len(cfg) == 0 {
		return json.RawMessage(`{}`)
	}
	out, _ := json.Marshal(cfg.ToMap())
	return out
}

func (s *Server) projectSummary(ctx context.Context, project model.Project, activeID int64) map[string]any {
	jobs, _ := s.repo.CountProjectJobs(ctx, project.ID)
	cards, _ := s.repo.CountProjectCards(ctx, project.ID)
	return map[string]any{
		"id":             project.ID,
		"name":           project.Name,
		"location":       project.Location,
		"description":    project.Description,
		"recipe_id":      project.RecipeID,
		"recipe":         jsonRawOrObject(project.Recipe),
		"project_state":  project.ProjectState,
		"settings":       jsonRawOrObject(project.Settings),
		"model_config":   jsonRawOrObject(extractSettingsModelConfig(project.Settings)),
		"agent_config":   jsonRawOrObject(extractSettingsAgentConfig(project.Settings)),
		"last_seen_at":   project.LastSeenAt,
		"created_at":     project.CreatedAt,
		"updated_at":     project.UpdatedAt,
		"job_count":      jobs,
		"card_count":     cards,
		"is_active":      activeID > 0 && activeID == project.ID,
	}
}

func jsonRawOrObject(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func (s *Server) initializeProjectState(ctx context.Context, project model.Project) (model.Project, error) {
	if strings.TrimSpace(project.ProjectState) != "" {
		return project, nil
	}
	survey := omni.BuildWorksiteSurvey(project.Location)
	state := strings.TrimSpace(survey.ProjectState)
	if state == "" {
		return project, nil
	}
	patch := model.ProjectPatch{ProjectState: &state}
	return s.repo.UpdateProject(ctx, project.ID, patch)
}

func (s *Server) loadCatalogRecipeJSON(recipeID string) (json.RawMessage, error) {
	recipes, err := omni.LoadRecipes(s.recipeRoot())
	if err != nil {
		return nil, err
	}
	for _, recipe := range recipes {
		if recipe.ID == recipeID {
			return json.Marshal(recipe)
		}
	}
	return json.RawMessage(`{}`), nil
}

func (s *Server) resolveProjectID(r *http.Request) (int64, error) {
	if s.repo == nil {
		return 0, fmt.Errorf("database unavailable")
	}
	raw := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return 0, fmt.Errorf("invalid project_id")
		}
		return id, nil
	}
	id, err := s.repo.GetActiveProjectID(r.Context())
	if err != nil {
		return 0, err
	}
	if id <= 0 {
		return 0, fmt.Errorf("no active project selected")
	}
	return id, nil
}

func (s *Server) scrumBoardFromProject(ctx context.Context, projectID int64) (ScrumBoard, error) {
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return ScrumBoard{}, err
	}
	cards, err := s.repo.ListScrumCards(ctx, projectID)
	if err != nil {
		return ScrumBoard{}, err
	}
	board := ScrumBoard{
		ID:               fmt.Sprintf("project_%d", projectID),
		Name:             project.Name,
		ProjectDirectory: project.Location,
		Columns:          append([]string(nil), scrumColumns...),
		Cards:            make([]ScrumCard, 0, len(cards)),
		UpdatedAt:        project.UpdatedAt.UTC().Format(time.RFC3339),
	}
	for _, card := range cards {
		board.Cards = append(board.Cards, dbScrumCardToAPI(card))
	}
	return board, nil
}

func dbScrumCardToAPI(card queue.DBScrumCard) ScrumCard {
	out := ScrumCard{
		ID:          card.ID,
		Title:       card.Title,
		Description: card.Description,
		Column:      card.Column,
		JobID:       card.JobID,
		ConsoleLog:  card.ConsoleLog,
		PlayState:   card.PlayState,
		QueueOrder:  card.QueueOrder,
		ModelConfig: card.ModelConfig,
		AgentConfig: card.AgentConfig,
		CreatedAt:   card.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   card.UpdatedAt.UTC().Format(time.RFC3339),
		Checklist:   []ScrumChecklistItem{},
		RefFiles:    []string{},
		Chat:        []ScrumChatMessage{},
	}
	_ = json.Unmarshal(card.Checklist, &out.Checklist)
	_ = json.Unmarshal(card.RefFiles, &out.RefFiles)
	_ = json.Unmarshal(card.Chat, &out.Chat)
	return out
}

func apiScrumCardToPatch(card ScrumCard) map[string]any {
	checklist, _ := json.Marshal(card.Checklist)
	refFiles, _ := json.Marshal(card.RefFiles)
	chat, _ := json.Marshal(card.Chat)
	modelConfig := card.ModelConfig
	if len(modelConfig) == 0 {
		modelConfig = json.RawMessage(`{}`)
	}
	agentConfig := card.AgentConfig
	if len(agentConfig) == 0 {
		agentConfig = json.RawMessage(`{}`)
	}
	return map[string]any{
		"title":        card.Title,
		"description":  card.Description,
		"column":       card.Column,
		"checklist":    json.RawMessage(checklist),
		"ref_files":    json.RawMessage(refFiles),
		"chat":         json.RawMessage(chat),
		"model_config": modelConfig,
		"agent_config": agentConfig,
		"job_id":       card.JobID,
		"console_log":  card.ConsoleLog,
		"play_state":   card.PlayState,
		"queue_order":  card.QueueOrder,
	}
}
