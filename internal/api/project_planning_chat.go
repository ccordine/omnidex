package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/websearch"
)

type ProjectPlanningChatConfig struct {
	Model          string `json:"model"`
	ReasoningMode  string `json:"reasoning_mode"`
}

type ProjectPlanningCardDraft struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Column      string   `json:"column"`
	Checklist   []string `json:"checklist"`
}

type ProjectPlanningLLMResponse struct {
	Reply        string                     `json:"reply"`
	Suggestions  []ScrumCoachSuggestion       `json:"suggestions"`
	CardDrafts   []ProjectPlanningCardDraft `json:"card_drafts"`
	MemoryNotes  []ScrumCoachMemoryNote     `json:"memory_notes"`
	ResearchUsed bool                       `json:"research_used"`
}

func defaultProjectPlanningChatConfig() ProjectPlanningChatConfig {
	return ProjectPlanningChatConfig{
		Model:         "",
		ReasoningMode: "instant",
	}
}

func parseProjectPlanningChatConfig(raw any) ProjectPlanningChatConfig {
	cfg := defaultProjectPlanningChatConfig()
	if raw == nil {
		return cfg
	}
	payload, ok := raw.(map[string]any)
	if !ok {
		return cfg
	}
	if v, ok := payload["model"].(string); ok {
		cfg.Model = strings.TrimSpace(v)
	}
	if v, ok := payload["reasoning_mode"].(string); ok && strings.TrimSpace(v) != "" {
		cfg.ReasoningMode = strings.ToLower(strings.TrimSpace(v))
	}
	return cfg
}

func loadProjectPlanningChat(settings json.RawMessage) ([]ScrumChatMessage, ProjectPlanningChatConfig) {
	chat := []ScrumChatMessage{}
	cfg := defaultProjectPlanningChatConfig()
	if len(settings) == 0 {
		return chat, cfg
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(settings, &payload); err != nil {
		return chat, cfg
	}
	if raw, ok := payload["planning_chat"]; ok {
		_ = json.Unmarshal(raw, &chat)
	}
	if raw, ok := payload["planning_chat_config"]; ok {
		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err == nil {
			cfg = parseProjectPlanningChatConfig(decoded)
		}
	}
	return chat, cfg
}

func (s *Server) saveProjectPlanningChat(ctx context.Context, projectID int64, chat []ScrumChatMessage, cfg ProjectPlanningChatConfig) error {
	if s.repo == nil || projectID <= 0 {
		return fmt.Errorf("database unavailable")
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	var settings map[string]any
	if len(project.Settings) > 0 {
		_ = json.Unmarshal(project.Settings, &settings)
	}
	if settings == nil {
		settings = map[string]any{}
	}
	settings["planning_chat"] = chat
	settings["planning_chat_config"] = cfg
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	settingsJSON := json.RawMessage(raw)
	patch := model.ProjectPatch{Settings: &settingsJSON}
	_, err = s.repo.UpdateProject(ctx, projectID, patch)
	return err
}

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
	chat, cfg := loadProjectPlanningChat(project.Settings)
	draftQueue := loadPlanningDraftQueue(project.Settings)

	switch r.Method {
	case http.MethodGet:
		resolved, _ := s.resolvedModelsForProjectCard(r.Context(), projectID, ScrumCard{})
		writeJSON(w, http.StatusOK, map[string]any{
			"chat":               chat,
			"config":             cfg,
			"draft_queue":        draftQueue,
			"pending_count":      len(pendingPlanningDrafts(draftQueue)),
			"resolved_models":    resolved,
			"web_search_enabled": s.webSearchEnabled,
		})
	case http.MethodPatch:
		var req struct {
			Config *ProjectPlanningChatConfig `json:"config"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if req.Config != nil {
			if strings.TrimSpace(req.Config.ReasoningMode) != "" {
				cfg.ReasoningMode = strings.ToLower(strings.TrimSpace(req.Config.ReasoningMode))
			}
			cfg.Model = strings.TrimSpace(req.Config.Model)
		}
		if err := s.saveProjectPlanningChat(r.Context(), projectID, chat, cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"config": cfg})
	case http.MethodPost:
		var req struct {
			Message string                     `json:"message"`
			Mode    string                     `json:"mode"`
			Config  *ProjectPlanningChatConfig `json:"config"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		req.Message = strings.TrimSpace(req.Message)
		req.Mode = normalizeProjectPlanningMode(req.Message, req.Mode)
		if req.Config != nil {
			if strings.TrimSpace(req.Config.ReasoningMode) != "" {
				cfg.ReasoningMode = strings.ToLower(strings.TrimSpace(req.Config.ReasoningMode))
			}
			cfg.Model = strings.TrimSpace(req.Config.Model)
		}
		if req.Mode == "config" {
			writeJSON(w, http.StatusOK, map[string]any{"config": cfg, "chat": chat})
			return
		}
		if req.Message == "" && req.Mode != "scan" {
			writeError(w, http.StatusBadRequest, "message is required")
			return
		}

		board, _ := s.scrumBoardFromProject(r.Context(), projectID)
		s.refreshScrumFlowMetricsForBoard(r.Context(), projectID, &board)
		memoryQuery := firstNonEmpty(req.Message, project.Name, project.Description)
		memoryLines := s.projectPlanningMemoryContext(r.Context(), project, memoryQuery)
		mapLines := s.projectPlanningMapContext(r.Context(), project)
		researchLines, researchUsed := s.projectPlanningResearchContext(r.Context(), req.Message, req.Mode)

		system := projectPlanningModeSystem(req.Mode)
		userPrompt := buildProjectPlanningUserPrompt(project, board, cfg, req.Mode, req.Message, memoryLines, mapLines, researchLines, chat)

		modelName := s.projectPlanningModel(project, cfg)
		rawReply, err := s.scrumCoachLLMGenerate(r.Context(), modelName, system, userPrompt)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		parsed := parseProjectPlanningLLMResponse(rawReply)
		parsed.ResearchUsed = researchUsed || req.Mode == "batch"

		batchID := ""
		if len(parsed.CardDrafts) > 0 {
			batchID = fmt.Sprintf("batch_%d", time.Now().UnixNano())
			draftQueue = appendPlanningDrafts(draftQueue, parsed.CardDrafts, req.Mode, batchID)
		}

		if req.Message != "" && req.Mode != "scan" {
			chat = append(chat, ScrumChatMessage{
				Role:      "user",
				Content:   req.Message,
				CreatedAt: nowRFC3339(),
			})
		}
		if strings.TrimSpace(parsed.Reply) != "" {
			chat = append(chat, ScrumChatMessage{
				Role:      "assistant",
				Content:   parsed.Reply,
				CreatedAt: nowRFC3339(),
			})
		}

		if err := s.saveProjectPlanningChat(r.Context(), projectID, chat, cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if len(parsed.CardDrafts) > 0 {
			if err := s.savePlanningDraftQueue(r.Context(), projectID, draftQueue); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}

		memoryStored := 0
		if s.repo != nil && s.llmClient != nil {
			for _, note := range parsed.MemoryNotes {
				content := strings.TrimSpace(note.Content)
				if content == "" {
					continue
				}
				noteTags := mergeTags(note.Tags, []string{"scrum", "project-chat", fmt.Sprintf("project:%d", projectID)})
				embedding, _ := s.llmClient.Embedding(r.Context(), content)
				if _, err := s.repo.AddMemoryChunk(r.Context(), "project-planner", model.MemoryKindReference, content, noteTags, embedding); err == nil {
					memoryStored++
				}
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"chat":           chat,
			"config":         cfg,
			"reply":          parsed.Reply,
			"suggestions":    parsed.Suggestions,
			"card_drafts":    parsed.CardDrafts,
			"draft_queue":    draftQueue,
			"pending_count":  len(pendingPlanningDrafts(draftQueue)),
			"batch_id":       batchID,
			"memory_stored":  memoryStored,
			"research_used":  parsed.ResearchUsed,
			"mode":           req.Mode,
			"model":          modelName,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) projectPlanningModel(project model.Project, cfg ProjectPlanningChatConfig) string {
	if modelName := strings.TrimSpace(cfg.Model); modelName != "" {
		return modelName
	}
	resolved, _ := s.resolveModelConfig(project, ScrumCard{})
	if strings.EqualFold(strings.TrimSpace(cfg.ReasoningMode), "thinking") {
		return firstNonEmpty(
			resolved.Get("thinking_model"),
			resolved.Get("planner_model"),
			resolved.Get("default_model"),
			s.ollamaDefaultModel,
			"qwen3:4b-thinking",
		)
	}
	return firstNonEmpty(
		resolved.Get("default_model"),
		resolved.Get("planner_model"),
		s.ollamaDefaultModel,
		"qwen3:4b",
	)
}

func normalizeProjectPlanningMode(message, mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "" && mode != "chat" {
		return mode
	}
	if strings.HasPrefix(message, "/") {
		parts := strings.Fields(message)
		switch strings.ToLower(parts[0]) {
		case "/plan":
			return "plan"
		case "/research", "/researching":
			return "research"
		case "/scan":
			return "scan"
		case "/cards":
			return "cards"
		case "/batch":
			return "batch"
		}
	}
	if mode == "" {
		return "chat"
	}
	return mode
}

func projectPlanningModeSystem(mode string) string {
	base := strings.Join([]string{
		"You are the Omni project planner — a productivity assistant for an entire software project.",
		"You help discuss goals, refine the backlog, draft scrum cards, spot risks, and organize work.",
		"You never execute code, run builds, or modify files directly.",
		"When suggesting cards, populate card_drafts with concrete backlog items.",
		"Respond with JSON only (no markdown fences):",
		`{"reply":"markdown conversation","suggestions":[{"level":"info|warn|tip","text":"..."}],"card_drafts":[{"title":"...","description":"...","column":"backlog|ready|...","checklist":["..."]}],"memory_notes":[{"content":"...","tags":["tag"]}]}`,
	}, "\n")
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "scan":
		return base + "\nMode: scan — review the project board and emit proactive observations (stale cards, gaps, priorities). Keep reply brief."
	case "plan":
		return base + "\nMode: plan — structure upcoming work: milestones, card breakdowns, dependencies, and sequencing."
	case "research":
		return base + "\nMode: research — synthesize web research snippets and memory into actionable planning guidance. Cite uncertainties."
	case "cards":
		return base + "\nMode: cards — focus on drafting well-scoped scrum cards. Populate card_drafts richly."
	case "batch":
		return base + "\nMode: batch — research the topic, synthesize findings, then emit a batch of reviewable card_drafts (typically 3-8 items). Break work into setup, research, implementation, and verification cards when relevant. Prefer backlog unless the user asked to queue execution-ready work."
	default:
		return base + "\nMode: chat — collaborative project planning dialogue."
	}
}

func buildProjectPlanningUserPrompt(
	project model.Project,
	board ScrumBoard,
	cfg ProjectPlanningChatConfig,
	mode, message string,
	memoryLines, mapLines, researchLines []string,
	history []ScrumChatMessage,
) string {
	lines := []string{
		"Project: " + project.Name,
		"Directory: " + board.ProjectDirectory,
		"State: " + strings.TrimSpace(project.ProjectState),
	}
	if desc := strings.TrimSpace(project.Description); desc != "" {
		lines = append(lines, "Description: "+desc)
	}
	lines = append(lines, "Reasoning mode: "+cfg.ReasoningMode)
	lines = append(lines, "Board summary:")
	lines = append(lines, summarizeScrumBoard(board)...)
	lines = append(lines, summarizeScrumFlowBoard(board)...)
	if len(mapLines) > 0 {
		lines = append(lines, "Codebase map:", strings.Join(mapLines, "\n"))
	}
	if len(memoryLines) > 0 {
		lines = append(lines, "Relevant memory:", strings.Join(memoryLines, "\n---\n"))
	}
	if len(researchLines) > 0 {
		lines = append(lines, "Web research:", strings.Join(researchLines, "\n---\n"))
	}
	for _, msg := range history {
		lines = append(lines, msg.Role+": "+msg.Content)
	}
	if strings.TrimSpace(message) != "" {
		lines = append(lines, "user: "+strings.TrimSpace(message))
	}
	lines = append(lines, "Mode: "+mode)
	return strings.Join(lines, "\n")
}

func summarizeScrumBoard(board ScrumBoard) []string {
	if len(board.Cards) == 0 {
		return []string{"(no cards yet)"}
	}
	byColumn := map[string][]ScrumCard{}
	for _, card := range board.Cards {
		col := strings.TrimSpace(card.Column)
		if col == "" {
			col = "backlog"
		}
		byColumn[col] = append(byColumn[col], card)
	}
	out := make([]string, 0, len(board.Cards))
	for _, col := range board.Columns {
		cards := byColumn[col]
		if len(cards) == 0 {
			continue
		}
		out = append(out, fmt.Sprintf("[%s] %d cards", col, len(cards)))
		for _, card := range cards {
			line := "- " + strings.TrimSpace(card.Title)
			if card.PlayState == "running" {
				line += " (running)"
			}
			if desc := strings.TrimSpace(card.Description); desc != "" {
				line += ": " + trimForPrompt(desc, 120)
			}
			out = append(out, line)
		}
	}
	return out
}

func summarizeScrumFlowBoard(board ScrumBoard) []string {
	out := []string{}
	for _, card := range board.Cards {
		metrics := parseScrumFlowMetrics(card.FlowMetrics)
		if metrics.CompletionStatus != "likely_incomplete" && metrics.AssignedReturns == 0 && metrics.IncompleteScore < 25 {
			continue
		}
		line := fmt.Sprintf("- %s [%s] status=%s score=%d", strings.TrimSpace(card.Title), normalizeScrumColumn(card.Column), metrics.CompletionStatus, metrics.IncompleteScore)
		if metrics.AssignedReturns > 0 {
			line += fmt.Sprintf(" assigned_returns=%d", metrics.AssignedReturns)
		}
		if metrics.ChannelMessages+metrics.PlanningMessages > 0 {
			line += fmt.Sprintf(" messages=%d", metrics.ChannelMessages+metrics.PlanningMessages)
		}
		if len(metrics.Signals) > 0 {
			line += " signals: " + strings.Join(metrics.Signals, "; ")
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		return nil
	}
	return append([]string{"Flow metrics (incomplete / churn signals):"}, out...)
}

func trimForPrompt(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max]) + "…"
}

func (s *Server) projectPlanningMemoryContext(ctx context.Context, project model.Project, query string) []string {
	if s.repo == nil || s.llmClient == nil {
		return nil
	}
	tags := []string{fmt.Sprintf("project:%d", project.ID), "scrum", "project-chat"}
	embedding, err := s.llmClient.Embedding(ctx, query)
	if err != nil {
		embedding = nil
	}
	matches, err := s.repo.FindRelevantMemory(ctx, embedding, tags, 8)
	if err != nil {
		return nil
	}
	lines := make([]string, 0, len(matches))
	for _, match := range matches {
		if strings.TrimSpace(match.Content) == "" {
			continue
		}
		lines = append(lines, match.Content)
	}
	return lines
}

func (s *Server) projectPlanningMapContext(ctx context.Context, project model.Project) []string {
	location := strings.TrimSpace(project.Location)
	if location == "" {
		return nil
	}
	payload, err := s.loadProjectCodebaseMapPayload(ctx, location)
	if err != nil || payload == nil {
		return nil
	}
	exists, _ := payload["exists"].(bool)
	if !exists {
		return []string{"(codebase map not scanned yet)"}
	}
	lines := []string{}
	if root, ok := payload["root"].(string); ok && strings.TrimSpace(root) != "" {
		lines = append(lines, "root: "+root)
	}
	if count, ok := payload["file_count"].(float64); ok {
		lines = append(lines, fmt.Sprintf("files: %d", int(count)))
	}
	if modules, ok := payload["modules"].([]any); ok {
		for i, raw := range modules {
			if i >= 6 {
				break
			}
			mod, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			path, _ := mod["path"].(string)
			purpose, _ := mod["purpose"].(string)
			if path == "" {
				continue
			}
			line := path
			if purpose != "" {
				line += " — " + trimForPrompt(purpose, 100)
			}
			lines = append(lines, line)
		}
	}
	if entrypoints, ok := payload["entrypoints"].([]any); ok && len(entrypoints) > 0 {
		lines = append(lines, "entrypoints:")
		for i, raw := range entrypoints {
			if i >= 4 {
				break
			}
			entry, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if path, _ := entry["path"].(string); path != "" {
				lines = append(lines, "- "+path)
			}
		}
	}
	return lines
}

func (s *Server) projectPlanningResearchContext(ctx context.Context, message, mode string) ([]string, bool) {
	if !s.webSearchEnabled || s.llmClient == nil {
		return nil, false
	}
	if mode != "research" && mode != "batch" && !strings.HasPrefix(strings.ToLower(message), "/research") && !strings.HasPrefix(strings.ToLower(message), "/batch") {
		return nil, false
	}
	query := strings.TrimSpace(message)
	for _, prefix := range []string{"/batch", "/research", "/researching"} {
		if strings.HasPrefix(strings.ToLower(query), prefix) {
			query = strings.TrimSpace(query[len(prefix):])
			break
		}
	}
	if query == "" {
		query = strings.TrimSpace(message)
	}
	if query == "" {
		return nil, false
	}
	searcher := websearch.New(s.webSearchProviders, s.webSearchTimeout, 3000, 6000)
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	results, err := searcher.SearchAll(ctx, query)
	if err != nil {
		return []string{"(web search failed: " + trimForPrompt(err.Error(), 200) + ")"}, false
	}
	if len(results) == 0 {
		return []string{"(web search returned no results)"}, true
	}
	lines := make([]string, 0, len(results))
	for i, result := range results {
		if i >= 5 {
			break
		}
		snippet := strings.TrimSpace(result.Snippet)
		if snippet == "" {
			snippet = strings.TrimSpace(result.Content)
		}
		lines = append(lines, strings.Join([]string{
			"title: " + strings.TrimSpace(result.Title),
			"url: " + strings.TrimSpace(result.URL),
			"snippet: " + trimForPrompt(snippet, 400),
		}, "\n"))
	}
	return lines, true
}

func parseProjectPlanningLLMResponse(raw string) ProjectPlanningLLMResponse {
	raw = strings.TrimSpace(raw)
	out := ProjectPlanningLLMResponse{Reply: raw}
	if raw == "" {
		return out
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		var parsed ProjectPlanningLLMResponse
		if err := json.Unmarshal([]byte(raw[start:end+1]), &parsed); err == nil {
			if strings.TrimSpace(parsed.Reply) != "" {
				out = parsed
			} else if raw != "" {
				out.Reply = raw
			}
			return out
		}
	}
	return out
}
