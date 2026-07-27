package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const projectPlanningMessagePageSize = 50

func (s *Server) handleProjectPlanningChat(w http.ResponseWriter, r *http.Request, projectID int64) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "projects require database")
		return
	}
	project, err := s.repo.GetProject(r.Context(), projectID)
	if err != nil {
		writeProjectError(w, err)
		return
	}
	config, err := s.repo.GetProjectPlanningConfig(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("load project planning config: %v", err))
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetProjectPlanningChat(w, r, project, config)
	case http.MethodPatch:
		s.handlePatchProjectPlanningChat(w, r, projectID, config)
	case http.MethodPost:
		s.handlePostProjectPlanningChat(w, r, project, config)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGetProjectPlanningChat(w http.ResponseWriter, r *http.Request, project model.Project, config ProjectPlanningChatConfig) {
	limit, beforeID, err := parseProjectPlanningPage(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	page, err := s.repo.ListProjectPlanningMessages(r.Context(), project.ID, limit, beforeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("load project planning messages: %v", err))
		return
	}
	drafts, err := s.repo.ListProjectPlanningDrafts(r.Context(), project.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("load project planning drafts: %v", err))
		return
	}
	resolved, err := s.resolvedModelsForProjectCard(r.Context(), project.ID, ScrumCard{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("resolve project planning models: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, projectPlanningStatePayload(page, config, drafts, map[string]any{
		"resolved_models":    resolved,
		"web_search_enabled": s.webSearchEnabled,
	}))
}

func (s *Server) handlePatchProjectPlanningChat(w http.ResponseWriter, r *http.Request, projectID int64, current ProjectPlanningChatConfig) {
	var request struct {
		Config *ProjectPlanningChatConfig `json:"config"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if err := requireJSONEOF(decoder, "project planning config request"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if request.Config == nil {
		writeError(w, http.StatusBadRequest, "config is required")
		return
	}
	config := mergeProjectPlanningConfig(current, *request.Config)
	config, err := validateProjectPlanningChatConfig(config)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	config, err = s.repo.UpdateProjectPlanningConfig(r.Context(), projectID, config)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("save project planning config: %v", err))
		return
	}
	realtimePublished, realtimeError := s.publishProjectPlanningUpdated(projectID, "config_updated")
	writeJSON(w, http.StatusOK, map[string]any{
		"config":             config,
		"realtime_published": realtimePublished,
		"realtime_error":     realtimeError,
	})
}

func (s *Server) handlePostProjectPlanningChat(w http.ResponseWriter, r *http.Request, project model.Project, current ProjectPlanningChatConfig) {
	var request struct {
		Message string                     `json:"message"`
		Mode    string                     `json:"mode"`
		Config  *ProjectPlanningChatConfig `json:"config"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if err := requireJSONEOF(decoder, "project planning request"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	request.Message = strings.TrimSpace(request.Message)
	mode, err := normalizeProjectPlanningMode(request.Message, request.Mode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if request.Message == "" && mode != "scan" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	config := current
	if request.Config != nil {
		config = mergeProjectPlanningConfig(current, *request.Config)
	}
	config, err = validateProjectPlanningChatConfig(config)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	page, err := s.repo.ListProjectPlanningMessages(r.Context(), project.ID, projectPlanningMessagePageSize, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("load project planning history: %v", err))
		return
	}
	board, err := s.scrumBoardFromProject(r.Context(), project.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("load project planning board: %v", err))
		return
	}
	if err := s.refreshScrumFlowMetricsForBoard(r.Context(), project.ID, &board); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	memoryQuery := firstNonEmpty(request.Message, project.Name, project.Description)
	memoryLines, err := s.projectPlanningMemoryContext(r.Context(), project, memoryQuery)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	mapLines, err := s.projectPlanningMapContext(r.Context(), project)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	researchLines, researchUsed, err := s.projectPlanningResearchContext(r.Context(), request.Message, mode)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	system, err := projectPlanningModeSystem(mode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	history := projectPlanningMessagesToAPI(page.Messages)
	userPrompt := buildProjectPlanningUserPrompt(project, board, config, mode, request.Message, memoryLines, mapLines, researchLines, history)
	modelName, err := s.projectPlanningModel(project, config)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rawReply, err := s.scrumCoachLLMGenerate(r.Context(), llmContextSourceProjectPlanning, modelName, system, userPrompt, llmContextTelemetryMeta{
		ProjectID: project.ID,
		Metadata:  map[string]any{"mode": mode},
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	response, err := parseProjectPlanningLLMResponse(rawReply)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	drafts, batchID, err := newProjectPlanningDrafts(project.ID, response.CardDrafts, mode)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	userMessage := request.Message
	if mode == "scan" {
		userMessage = ""
	}
	committed, err := s.repo.CommitProjectPlanningResponse(r.Context(), project.ID, queue.ProjectPlanningCommit{
		Config:           config,
		UserMessage:      userMessage,
		AssistantMessage: response.Reply,
		Drafts:           drafts,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, queue.ErrProjectPlanningConflict) {
			status = http.StatusConflict
		}
		writeError(w, status, fmt.Sprintf("commit project planning response: %v", err))
		return
	}
	realtimePublished, realtimeError := s.publishProjectPlanningUpdated(project.ID, "response_committed")
	payload := projectPlanningStatePayload(committed.Messages, config, committed.Drafts, map[string]any{
		"reply":              response.Reply,
		"suggestions":        response.Suggestions,
		"card_drafts":        response.CardDrafts,
		"batch_id":           batchID,
		"research_used":      researchUsed,
		"mode":               mode,
		"model":              modelName,
		"realtime_published": realtimePublished,
		"realtime_error":     realtimeError,
	})
	writeJSON(w, http.StatusOK, payload)
}

func mergeProjectPlanningConfig(current, requested ProjectPlanningChatConfig) ProjectPlanningChatConfig {
	current.Model = strings.TrimSpace(requested.Model)
	if mode := strings.TrimSpace(requested.ReasoningMode); mode != "" {
		current.ReasoningMode = mode
	}
	return current
}
