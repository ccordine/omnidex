package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/scrum"
)

type ScrumCoachConfig struct {
	Enabled  bool   `json:"enabled"`
	AutoScan bool   `json:"auto_scan"`
	Model    string `json:"model"`
}

type ScrumCoachSuggestion struct {
	Level string `json:"level"`
	Text  string `json:"text"`
}

type ScrumCoachMemoryNote struct {
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

type ScrumCoachLLMResponse struct {
	Reply        string                 `json:"reply"`
	Suggestions  []ScrumCoachSuggestion   `json:"suggestions"`
	CardTags     []string               `json:"card_tags"`
	ProjectTags  []string               `json:"project_tags"`
	CardPrompt   string                 `json:"card_prompt"`
	MemoryNotes  []ScrumCoachMemoryNote `json:"memory_notes"`
}

func defaultScrumCoachConfig() ScrumCoachConfig {
	return ScrumCoachConfig{
		Enabled:  true,
		AutoScan: true,
		Model:    "qwen3:4b-thinking",
	}
}

func parseScrumCoachConfig(raw json.RawMessage) ScrumCoachConfig {
	cfg := defaultScrumCoachConfig()
	if len(raw) == 0 {
		return cfg
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return cfg
	}
	if v, ok := payload["enabled"].(bool); ok {
		cfg.Enabled = v
	}
	if v, ok := payload["auto_scan"].(bool); ok {
		cfg.AutoScan = v
	}
	if v, ok := payload["model"].(string); ok && strings.TrimSpace(v) != "" {
		cfg.Model = strings.TrimSpace(v)
	}
	return cfg
}

func coachConfigToRaw(cfg ScrumCoachConfig) json.RawMessage {
	out, _ := json.Marshal(map[string]any{
		"enabled":   cfg.Enabled,
		"auto_scan": cfg.AutoScan,
		"model":     cfg.Model,
	})
	return out
}

func (s *Server) scrumCoachLLMGenerate(ctx context.Context, source, modelName, system, user string, meta llmContextTelemetryMeta) (string, error) {
	if s.llmClient == nil {
		return "", fmt.Errorf("no llm client configured")
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		modelName = firstNonEmpty(s.ollamaDefaultModel, "qwen3:4b-thinking")
	}
	prompt := strings.TrimSpace(system + "\n\n" + user)
	promptChars := llmPromptCharCount(system, user)
	generated, err := s.llmClient.Generate(ctx, modelName, prompt)
	s.recordLLMContextUsage(ctx, source, modelName, s.llmProviderName(), meta, promptChars, len(prompt), false, 0, err)
	return generated, err
}

func coachModeSystem(mode string) string {
	base := strings.Join([]string{
		"You are the Omni card coach — a meta-planning assistant for a single scrum card.",
		"You help refine scope, break work down, draft card ticket prompts, and tag work for memory.",
		"You never execute code or modify the project directly.",
		"Respond with JSON only (no markdown fences):",
		`{"reply":"markdown conversation","suggestions":[{"level":"info|warn|tip","text":"..."}],"card_tags":["tag"],"project_tags":["tag"],"card_prompt":"optional prompt for card ticket generation","memory_notes":[{"content":"...","tags":["tag"]}]}`,
	}, "\n")
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "scan":
		return base + "\nMode: scan — review the draft card fields and emit proactive suggestions (scope, missing acceptance criteria, unclear deps). Keep reply brief."
	case "plan":
		return base + "\nMode: plan — help structure the card: milestones, checklist items, risks, and what to defer to other cards."
	case "research":
		return base + "\nMode: research — suggest what to look up, which files to attach, questions to answer before execution. No code changes."
	case "card-ticket":
		return base + "\nMode: card-ticket — craft a strong card_prompt the user can review before generating a ticket. Populate card_prompt field richly."
	default:
		return base + "\nMode: chat — collaborative card planning dialogue."
	}
}

func parseCoachLLMResponse(raw string) ScrumCoachLLMResponse {
	raw = strings.TrimSpace(raw)
	out := ScrumCoachLLMResponse{Reply: raw}
	if raw == "" {
		return out
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		var parsed ScrumCoachLLMResponse
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

func buildCoachUserPrompt(card ScrumCard, board ScrumBoard, project model.Project, mode, message string, snapshot map[string]string, memoryLines []string) string {
	lines := []string{
		"Project: " + project.Name,
		"Project directory: " + board.ProjectDirectory,
		"Card column: " + card.Column,
	}
	if len(card.Tags) > 0 {
		lines = append(lines, "Card tags: "+strings.Join(card.Tags, ", "))
	}
	title := strings.TrimSpace(snapshot["title"])
	if title == "" {
		title = card.Title
	}
	desc := snapshot["description"]
	if desc == "" {
		desc = card.Description
	}
	lines = append(lines, "Title: "+title, "Description: "+desc)
	if checklist := snapshot["checklist"]; checklist != "" {
		lines = append(lines, "Checklist (draft):", checklist)
	} else if len(card.Checklist) > 0 {
		items := make([]scrum.ChecklistItem, 0, len(card.Checklist))
		for _, item := range card.Checklist {
			items = append(items, scrum.ChecklistItem{ID: item.ID, Text: item.Text, Done: item.Done})
		}
		if formatted := scrum.FormatChecklist(items); formatted != "" {
			lines = append(lines, "Checklist:", formatted)
		}
	}
	if strings.TrimSpace(card.CardTicket) != "" {
		lines = append(lines, "Card ticket draft:", card.CardTicket)
	}
	if strings.TrimSpace(card.CardPrompt) != "" {
		lines = append(lines, "Card prompt draft:", card.CardPrompt)
	}
	lines = appendScrumCardContextLines(lines, card)
	if len(memoryLines) > 0 {
		lines = append(lines, "Relevant memory:", strings.Join(memoryLines, "\n---\n"))
	}
	for _, msg := range card.PlanningChat {
		lines = append(lines, msg.Role+": "+msg.Content)
	}
	if strings.TrimSpace(message) != "" {
		lines = append(lines, "user: "+strings.TrimSpace(message))
	}
	lines = append(lines, "Mode: "+mode)
	return strings.Join(lines, "\n")
}

func (s *Server) coachMemoryContext(ctx context.Context, card ScrumCard, project model.Project, query string) []string {
	if s.repo == nil || s.llmClient == nil {
		return nil
	}
	tags := append([]string{}, card.Tags...)
	tags = append(tags, fmt.Sprintf("project:%d", project.ID), "scrum", "card-coach")
	embedding, err := s.llmClient.Embedding(ctx, query)
	if err != nil {
		embedding = nil
	}
	matches, err := s.repo.FindRelevantMemory(ctx, embedding, tags, 6)
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

func (s *Server) handleScrumCardCoach(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Message  string            `json:"message"`
		Mode     string            `json:"mode"`
		Snapshot map[string]string `json:"snapshot"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	req.Mode = normalizeCoachMode(req.Message, req.Mode)

	card, board, projectID, err := s.scrumGetCard(r, cardID)
	if err != nil {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}
	cfg := parseScrumCoachConfig(card.CoachConfig)
	if req.Mode == "config" {
		writeJSON(w, http.StatusOK, map[string]any{"coach_config": cfg})
		return
	}
	if !cfg.Enabled && req.Mode != "scan" {
		writeJSON(w, http.StatusOK, map[string]any{
			"card":    card,
			"reply":   "Card coach is disabled. Enable it in the coach panel.",
			"enabled": false,
		})
		return
	}

	project := model.Project{Name: "project"}
	if s.repo != nil && projectID > 0 {
		if loaded, err := s.repo.GetProject(r.Context(), projectID); err == nil {
			project = loaded
		}
	}

	memoryQuery := firstNonEmpty(req.Message, card.Title, card.Description)
	memoryLines := s.coachMemoryContext(r.Context(), card, project, memoryQuery)
	system := coachModeSystem(req.Mode)
	userPrompt := buildCoachUserPrompt(card, board, project, req.Mode, req.Message, req.Snapshot, memoryLines)

	rawReply, err := s.scrumCoachLLMGenerate(r.Context(), llmContextSourceScrumCoach, cfg.Model, system, userPrompt, llmContextTelemetryMeta{
		ProjectID: projectID,
		CardID:    card.ID,
		Metadata:  map[string]any{"mode": req.Mode},
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	parsed := parseCoachLLMResponse(rawReply)

	if req.Message != "" && req.Mode != "scan" {
		card.PlanningChat = append(card.PlanningChat, ScrumChatMessage{
			Role:      "user",
			Content:   req.Message,
			CreatedAt: nowRFC3339(),
		})
	}
	if strings.TrimSpace(parsed.Reply) != "" {
		card.PlanningChat = append(card.PlanningChat, ScrumChatMessage{
			Role:      "assistant",
			Content:   parsed.Reply,
			CreatedAt: nowRFC3339(),
		})
	}
	if len(parsed.CardTags) > 0 {
		card.Tags = mergeTags(card.Tags, parsed.CardTags)
	}
	if strings.TrimSpace(parsed.CardPrompt) != "" {
		card.CardPrompt = strings.TrimSpace(parsed.CardPrompt)
	}
	card.CoachConfig = coachConfigToRaw(cfg)

	updated, err := s.persistScrumCard(r, projectID, card)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	memoryStored := 0
	if s.repo != nil && s.llmClient != nil {
		for _, note := range parsed.MemoryNotes {
			content := strings.TrimSpace(note.Content)
			if content == "" {
				continue
			}
			noteTags := mergeTags(note.Tags, card.Tags, []string{"scrum", card.ID, fmt.Sprintf("project:%d", projectID)})
			embedding, _ := s.llmClient.Embedding(r.Context(), content)
			if _, err := s.repo.AddMemoryChunk(r.Context(), "scrum-coach", model.MemoryKindReference, content, noteTags, embedding); err == nil {
				memoryStored++
			}
		}
	}
	if s.repo != nil && projectID > 0 && len(parsed.ProjectTags) > 0 {
		_ = s.mergeProjectTags(r.Context(), projectID, parsed.ProjectTags)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"card":          updated,
		"reply":         parsed.Reply,
		"suggestions":   parsed.Suggestions,
		"card_prompt":   updated.CardPrompt,
		"memory_stored": memoryStored,
		"mode":          req.Mode,
		"model":         cfg.Model,
	})
}

func (s *Server) handleScrumCardCoachConfig(w http.ResponseWriter, r *http.Request, cardID string) {
	card, _, projectID, err := s.scrumGetCard(r, cardID)
	if err != nil {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"coach_config": parseScrumCoachConfig(card.CoachConfig)})
	case http.MethodPut:
		var req ScrumCoachConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if strings.TrimSpace(req.Model) == "" {
			req.Model = defaultScrumCoachConfig().Model
		}
		card.CoachConfig = coachConfigToRaw(req)
		updated, err := s.persistScrumCard(r, projectID, card)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"card": updated, "coach_config": req})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func normalizeCoachMode(message, mode string) string {
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
		case "/card-ticket", "/card":
			return "card-ticket"
		case "/scan":
			return "scan"
		}
	}
	if mode == "" {
		return "chat"
	}
	return mode
}

func mergeTags(existing []string, sets ...[]string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	add := func(items []string) {
		for _, item := range items {
			item = strings.TrimSpace(strings.ToLower(item))
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
	}
	add(existing)
	for _, set := range sets {
		add(set)
	}
	return out
}

func (s *Server) mergeProjectTags(ctx context.Context, projectID int64, tags []string) error {
	if s.repo == nil || projectID <= 0 || len(tags) == 0 {
		return nil
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
	existing := []string{}
	if raw, ok := settings["tags"].([]any); ok {
		for _, item := range raw {
			if text, ok := item.(string); ok {
				existing = append(existing, text)
			}
		}
	}
	settings["tags"] = mergeTags(existing, tags)
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	settingsJSON := json.RawMessage(raw)
	patch := model.ProjectPatch{Settings: &settingsJSON}
	_, err = s.repo.UpdateProject(ctx, projectID, patch)
	return err
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
