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
		items := make([]map[string]any, 0, len(projects))
		for _, project := range projects {
			summary, err := s.projectSummary(r.Context(), project)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			items = append(items, summary)
		}
		writeJSON(w, http.StatusOK, map[string]any{"projects": items})
	case http.MethodPost:
		var req struct {
			Name        string          `json:"name"`
			Location    string          `json:"location"`
			Description string          `json:"description"`
			RecipeID    string          `json:"recipe_id"`
			Recipe      json.RawMessage `json:"recipe"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		location, err := s.validateProjectLocation(r.Context(), req.Location)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		recipe := req.Recipe
		if len(recipe) == 0 && strings.TrimSpace(req.RecipeID) != "" {
			recipe, err = s.loadCatalogRecipeJSON(req.RecipeID)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		project, err := s.repo.CreateProject(r.Context(), req.Name, location, req.Description, req.RecipeID, recipe)
		if err != nil {
			if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
				writeError(w, http.StatusConflict, "project location already exists")
				return
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
		summary, err := s.projectSummary(r.Context(), project)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("project %d was created but its summary failed: %v", project.ID, err))
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"project": summary})
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
	if action == "survey" {
		s.handleProjectSurvey(w, r, id)
		return
	}
	if action == "play" {
		s.handleProjectPlay(w, r, id)
		return
	}
	if action == "pause" {
		s.handleProjectPause(w, r, id)
		return
	}
	if action == "map" || action == "map/scan" {
		s.handleProjectMap(w, r, id, action)
		return
	}
	if action == "git" {
		s.handleProjectGit(w, r, id)
		return
	}
	if action == "planning-chat" {
		s.handleProjectPlanningChat(w, r, id)
		return
	}
	if action == "planning-chat/drafts" {
		s.handleProjectPlanningDrafts(w, r, id)
		return
	}
	if action == "debugger" || action == "debugger/" || action == "debugger/run" {
		s.handleProjectDebugger(w, r, id, action)
		return
	}
	switch r.Method {
	case http.MethodGet:
		project, err := s.repo.GetProject(r.Context(), id)
		if err != nil {
			writeProjectError(w, err)
			return
		}
		summary, err := s.projectSummary(r.Context(), project)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		resolved, err := s.resolvedModelsForProjectCard(r.Context(), id, ScrumCard{})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		agentResolved, err := s.resolvedAgentsForProjectCard(r.Context(), id, ScrumCard{})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		payload := map[string]any{"project": summary, "model_config": resolved, "agent_config": agentResolved}
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
			location, err := s.validateProjectLocation(r.Context(), *patch.Location)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			patch.Location = &location
		}
		project, err := s.repo.UpdateProject(r.Context(), id, patch)
		if err != nil {
			writeProjectError(w, err)
			return
		}
		summary, err := s.projectSummary(r.Context(), project)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("project %d was updated but its summary failed: %v", project.ID, err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"project": summary})
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
	summary, err := s.projectSummary(r.Context(), project)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("project %d was surveyed but its summary failed: %v", project.ID, err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": summary})
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

func extractSettingsModelConfig(settings json.RawMessage) (json.RawMessage, error) {
	return extractSettingsJSONObject(settings, "model_config")
}

func (s *Server) projectSummary(ctx context.Context, project model.Project) (map[string]any, error) {
	jobs, err := s.repo.CountProjectJobs(ctx, project.ID)
	if err != nil {
		return nil, fmt.Errorf("count jobs for project %d: %w", project.ID, err)
	}
	cards, err := s.repo.CountProjectCards(ctx, project.ID)
	if err != nil {
		return nil, fmt.Errorf("count cards for project %d: %w", project.ID, err)
	}
	recipe, err := jsonRawOrObject(project.Recipe)
	if err != nil {
		return nil, fmt.Errorf("decode project %d recipe: %w", project.ID, err)
	}
	settings, err := jsonRawOrObject(project.Settings)
	if err != nil {
		return nil, fmt.Errorf("decode project %d settings: %w", project.ID, err)
	}
	modelConfigRaw, err := extractSettingsModelConfig(project.Settings)
	if err != nil {
		return nil, fmt.Errorf("decode project %d model config: %w", project.ID, err)
	}
	modelConfig, err := jsonRawOrObject(modelConfigRaw)
	if err != nil {
		return nil, err
	}
	agentConfigRaw, err := extractSettingsAgentConfig(project.Settings)
	if err != nil {
		return nil, fmt.Errorf("decode project %d agent config: %w", project.ID, err)
	}
	agentConfig, err := jsonRawOrObject(agentConfigRaw)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":            project.ID,
		"name":          project.Name,
		"location":      project.Location,
		"description":   project.Description,
		"recipe_id":     project.RecipeID,
		"recipe":        recipe,
		"project_state": project.ProjectState,
		"settings":      settings,
		"model_config":  modelConfig,
		"agent_config":  agentConfig,
		"last_seen_at":  project.LastSeenAt,
		"created_at":    project.CreatedAt,
		"updated_at":    project.UpdatedAt,
		"job_count":     jobs,
		"card_count":    cards,
	}, nil
}

func jsonRawOrObject(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func extractSettingsJSONObject(settings json.RawMessage, key string) (json.RawMessage, error) {
	if len(settings) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(settings, &root); err != nil {
		return nil, err
	}
	raw, ok := root[key]
	if !ok || len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("%s must be a JSON object: %w", key, err)
	}
	if object == nil {
		return nil, fmt.Errorf("%s must be a JSON object", key)
	}
	return raw, nil
}

func (s *Server) initializeProjectState(ctx context.Context, project model.Project) (model.Project, error) {
	if strings.TrimSpace(project.ProjectState) != "" {
		return project, nil
	}
	survey, err := omni.BuildWorksiteSurvey(project.Location)
	if err != nil {
		return project, fmt.Errorf("inspect project workspace: %w", err)
	}
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
	return nil, fmt.Errorf("recipe %q was not found", strings.TrimSpace(recipeID))
}

func (s *Server) resolveProjectID(r *http.Request) (int64, error) {
	if s.repo == nil {
		return 0, fmt.Errorf("database unavailable")
	}
	if r == nil || r.URL == nil {
		return 0, fmt.Errorf("project_id is required")
	}
	raw := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return 0, fmt.Errorf("invalid project_id")
		}
		return id, nil
	}
	return 0, fmt.Errorf("project_id is required")
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
		apiCard, err := dbScrumCardToAPI(card)
		if err != nil {
			return ScrumBoard{}, fmt.Errorf("decode Scrum card %q: %w", card.ID, err)
		}
		board.Cards = append(board.Cards, apiCard)
	}
	return board, nil
}

func dbScrumCardToAPI(card queue.DBScrumCard) (ScrumCard, error) {
	out := ScrumCard{
		ID:           card.ID,
		Title:        card.Title,
		Description:  card.Description,
		Column:       card.Column,
		JobID:        card.JobID,
		TagsJobID:    card.TagsJobID,
		TicketJobID:  card.TicketJobID,
		ConsoleLog:   card.ConsoleLog,
		PlayState:    card.PlayState,
		QueueOrder:   card.QueueOrder,
		BoardOrder:   card.BoardOrder,
		CardTicket:   card.CardTicket,
		CardPrompt:   card.CardPrompt,
		RecipeID:     card.RecipeID,
		Recipe:       card.Recipe,
		CoachConfig:  card.CoachConfig,
		ModelConfig:  card.ModelConfig,
		AgentConfig:  card.AgentConfig,
		CreatedAt:    card.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:    card.UpdatedAt.UTC().Format(time.RFC3339),
		Checklist:    []ScrumChecklistItem{},
		RefFiles:     []string{},
		Chat:         []ScrumChatMessage{},
		PlanningChat: []ScrumChatMessage{},
		Tags:         []string{},
		TestCriteria: []ScrumChecklistItem{},
		FlowMetrics:  card.FlowMetrics,
	}
	fields := []struct {
		name   string
		raw    json.RawMessage
		target any
	}{
		{"checklist", card.Checklist, &out.Checklist},
		{"ref_files", card.RefFiles, &out.RefFiles},
		{"chat", card.Chat, &out.Chat},
		{"planning_chat", card.PlanningChat, &out.PlanningChat},
		{"tags", card.Tags, &out.Tags},
		{"test_criteria", card.TestCriteria, &out.TestCriteria},
	}
	for _, field := range fields {
		if err := json.Unmarshal(field.raw, field.target); err != nil {
			return ScrumCard{}, fmt.Errorf("%s must contain valid typed JSON: %w", field.name, err)
		}
	}
	for name, raw := range map[string]json.RawMessage{
		"model_config": card.ModelConfig,
		"agent_config": card.AgentConfig,
		"recipe":       card.Recipe,
		"coach_config": card.CoachConfig,
		"flow_metrics": card.FlowMetrics,
	} {
		var object map[string]any
		if err := json.Unmarshal(raw, &object); err != nil || object == nil {
			if err == nil {
				err = fmt.Errorf("expected JSON object")
			}
			return ScrumCard{}, fmt.Errorf("%s must contain valid typed JSON: %w", name, err)
		}
	}
	return out, nil
}

func apiScrumCardToPatch(card ScrumCard) (map[string]any, error) {
	checklist, err := json.Marshal(card.Checklist)
	if err != nil {
		return nil, fmt.Errorf("encode Scrum card checklist: %w", err)
	}
	refFiles, err := json.Marshal(card.RefFiles)
	if err != nil {
		return nil, fmt.Errorf("encode Scrum card reference files: %w", err)
	}
	chat, err := json.Marshal(card.Chat)
	if err != nil {
		return nil, fmt.Errorf("encode Scrum card chat: %w", err)
	}
	modelConfig := card.ModelConfig
	if len(modelConfig) == 0 {
		modelConfig = json.RawMessage(`{}`)
	}
	agentConfig := card.AgentConfig
	if len(agentConfig) == 0 {
		agentConfig = json.RawMessage(`{}`)
	}
	recipe := card.Recipe
	if len(recipe) == 0 {
		recipe = json.RawMessage(`{}`)
	}
	tags, err := json.Marshal(card.Tags)
	if err != nil {
		return nil, fmt.Errorf("encode Scrum card tags: %w", err)
	}
	planningChat, err := json.Marshal(card.PlanningChat)
	if err != nil {
		return nil, fmt.Errorf("encode Scrum card planning chat: %w", err)
	}
	testCriteria, err := json.Marshal(card.TestCriteria)
	if err != nil {
		return nil, fmt.Errorf("encode Scrum card test criteria: %w", err)
	}
	coachConfig := card.CoachConfig
	if len(coachConfig) == 0 {
		coachConfig = json.RawMessage(`{}`)
	}
	for name, raw := range map[string]json.RawMessage{
		"model_config": modelConfig,
		"agent_config": agentConfig,
		"recipe":       recipe,
		"coach_config": coachConfig,
	} {
		if !json.Valid(raw) {
			return nil, fmt.Errorf("Scrum card %s must contain valid JSON", name)
		}
	}
	return map[string]any{
		"title":         sanitizeScrumChannelText(card.Title),
		"description":   sanitizeScrumChannelText(card.Description),
		"column":        card.Column,
		"checklist":     json.RawMessage(sanitizeScrumChannelBytes(checklist)),
		"ref_files":     json.RawMessage(sanitizeScrumChannelBytes(refFiles)),
		"chat":          json.RawMessage(sanitizeScrumChannelBytes(chat)),
		"planning_chat": json.RawMessage(sanitizeScrumChannelBytes(planningChat)),
		"tags":          json.RawMessage(sanitizeScrumChannelBytes(tags)),
		"test_criteria": json.RawMessage(sanitizeScrumChannelBytes(testCriteria)),
		"coach_config":  json.RawMessage(sanitizeScrumChannelBytes(coachConfig)),
		"model_config":  json.RawMessage(sanitizeScrumChannelBytes(modelConfig)),
		"agent_config":  json.RawMessage(sanitizeScrumChannelBytes(agentConfig)),
		"card_ticket":   sanitizeScrumChannelText(card.CardTicket),
		"card_prompt":   sanitizeScrumChannelText(card.CardPrompt),
		"recipe_id":     sanitizeScrumChannelText(card.RecipeID),
		"recipe":        json.RawMessage(sanitizeScrumChannelBytes(recipe)),
		"job_id":        card.JobID,
		"tags_job_id":   card.TagsJobID,
		"ticket_job_id": card.TicketJobID,
		"console_log":   sanitizeScrumChannelText(card.ConsoleLog),
		"play_state":    card.PlayState,
		"queue_order":   card.QueueOrder,
		"board_order":   card.BoardOrder,
	}, nil
}

func (s *Server) validateProjectLocation(ctx context.Context, raw string) (string, error) {
	location, err := queue.NormalizeProjectLocation(raw)
	if err != nil {
		return "", err
	}
	if stat, err := os.Stat(location); err == nil {
		if stat.IsDir() {
			return location, nil
		}
		return "", fmt.Errorf("location must be an existing directory")
	}
	client := s.hostBridgeClient()
	if client == nil {
		return "", fmt.Errorf("location must be an existing directory")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result, err := client.Browse(ctx, location)
	if err != nil {
		return "", fmt.Errorf("location must be an existing directory")
	}
	if result == nil || strings.TrimSpace(result.Path) == "" {
		return "", fmt.Errorf("location must be an existing directory")
	}
	return filepath.Clean(result.Path), nil
}
