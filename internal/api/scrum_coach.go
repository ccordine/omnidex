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

func (s *Server) scrumCoachLLMGenerate(ctx context.Context, source, modelName, system, user string, meta llmContextTelemetryMeta) (string, error) {
	if s.llmClient == nil {
		return "", fmt.Errorf("no llm client configured")
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return "", fmt.Errorf("Scrum coach model is required")
	}
	prompt := strings.TrimSpace(system + "\n\n" + user)
	promptChars := llmPromptCharCount(system, user)
	generated, err := s.llmClient.Generate(ctx, modelName, prompt)
	s.recordLLMContextUsage(ctx, source, modelName, s.llmProviderName(), meta, promptChars, len(prompt), false, 0, err)
	return generated, err
}

func coachModeSystem(mode string) (string, error) {
	base := strings.Join([]string{
		"You are the Omni card coach — a meta-planning assistant for a single scrum card.",
		"You help refine scope, break work down, draft card ticket prompts, and tag work for memory.",
		"You never execute code or modify the project directly.",
		"Respond with JSON only (no markdown fences):",
		`{"reply":"markdown conversation","suggestions":[{"level":"info|warn|tip","text":"..."}],"card_tags":["tag"],"project_tags":["tag"],"card_prompt":"optional prompt for card ticket generation","memory_notes":[{"content":"...","tags":["tag"]}]}`,
	}, "\n")
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "scan":
		return base + "\nMode: scan — review the draft card fields and emit proactive suggestions (scope, missing acceptance criteria, unclear deps). Keep reply brief.", nil
	case "plan":
		return base + "\nMode: plan — help structure the card: milestones, checklist items, risks, and what to defer to other cards.", nil
	case "research":
		return base + "\nMode: research — suggest what to look up, which files to attach, questions to answer before execution. No code changes.", nil
	case "card-ticket":
		return base + "\nMode: card-ticket — craft a strong card_prompt the user can review before generating a ticket. Populate card_prompt field richly.", nil
	case "chat":
		return base + "\nMode: chat — collaborative card planning dialogue.", nil
	default:
		return "", fmt.Errorf("unsupported Scrum coach mode %q", mode)
	}
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

func (s *Server) coachMemoryContext(ctx context.Context, card ScrumCard, project model.Project, query string) ([]string, error) {
	if s == nil || s.repo == nil || s.llmClient == nil {
		return nil, fmt.Errorf("Scrum coach memory requires PostgreSQL and an embedding client")
	}
	if ctx == nil {
		return nil, fmt.Errorf("Scrum coach memory requires a context")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("Scrum coach memory query is required")
	}
	tags := append([]string{}, card.Tags...)
	tags = append(tags, fmt.Sprintf("project:%d", project.ID), "scrum", "card-coach")
	embedding, err := s.llmClient.Embedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed Scrum coach memory query: %w", err)
	}
	if len(embedding) == 0 {
		return nil, fmt.Errorf("embed Scrum coach memory query: provider returned an empty vector")
	}
	matches, err := s.repo.FindRelevantMemory(ctx, embedding, tags, 6)
	if err != nil {
		return nil, fmt.Errorf("find Scrum coach memory: %w", err)
	}
	lines := make([]string, 0, len(matches))
	for _, match := range matches {
		if strings.TrimSpace(match.Content) == "" {
			continue
		}
		lines = append(lines, match.Content)
	}
	return lines, nil
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
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if err := requireJSONEOF(decoder, "Scrum coach request"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	mode, err := normalizeCoachMode(req.Message, req.Mode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Mode = mode

	card, board, projectID, err := s.scrumGetCard(r, cardID)
	if err != nil {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}
	cfg, err := s.scrumCoachConfig(card.CoachConfig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if req.Mode == "config" {
		writeJSON(w, http.StatusOK, map[string]any{"coach_config": cfg})
		return
	}
	if req.Mode == "scan" && strings.TrimSpace(req.Message) == "" && !cfg.AutoScan {
		writeJSON(w, http.StatusOK, map[string]any{
			"card":        card,
			"suggestions": []ScrumCoachSuggestion{},
			"mode":        req.Mode,
			"model":       cfg.Model,
		})
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

	if s.repo == nil || projectID <= 0 {
		writeError(w, http.StatusServiceUnavailable, "Scrum coach requires a project database")
		return
	}
	project, err := s.repo.GetProject(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("load Scrum coach project: %v", err))
		return
	}

	memoryQuery := firstNonEmpty(req.Message, card.Title, card.Description)
	memoryLines, err := s.coachMemoryContext(r.Context(), card, project, memoryQuery)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	system, err := coachModeSystem(req.Mode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
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
	parsed, err := parseCoachLLMResponse(rawReply)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

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
	card.CoachConfig, err = coachConfigToRaw(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	updated, err := s.persistScrumCard(r, projectID, card)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	memoryStored, err := s.storeScrumCoachMemoryNotes(r.Context(), projectID, card, parsed.MemoryNotes)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("card %s was updated but coach memory persistence failed: %v", card.ID, err))
		return
	}
	if len(parsed.ProjectTags) > 0 {
		if err := s.mergeProjectTags(r.Context(), projectID, parsed.ProjectTags); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("card %s was updated but project tag persistence failed: %v", card.ID, err))
			return
		}
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

func (s *Server) storeScrumCoachMemoryNotes(
	ctx context.Context,
	projectID int64,
	card ScrumCard,
	notes []ScrumCoachMemoryNote,
) (int, error) {
	if len(notes) == 0 {
		return 0, nil
	}
	if s == nil || s.repo == nil || s.llmClient == nil {
		return 0, fmt.Errorf("Scrum coach memory requires PostgreSQL and an embedding client")
	}
	if ctx == nil || projectID <= 0 || strings.TrimSpace(card.ID) == "" {
		return 0, fmt.Errorf("Scrum coach memory requires context, project, and card authority")
	}
	stored := 0
	for i, note := range notes {
		content := strings.TrimSpace(note.Content)
		if content == "" {
			return stored, fmt.Errorf("memory note %d content is required", i)
		}
		noteTags := mergeTags(note.Tags, card.Tags, []string{"scrum", card.ID, fmt.Sprintf("project:%d", projectID)})
		embedding, err := s.llmClient.Embedding(ctx, content)
		if err != nil {
			return stored, fmt.Errorf("embed memory note %d: %w", i, err)
		}
		if len(embedding) == 0 {
			return stored, fmt.Errorf("embed memory note %d: provider returned an empty vector", i)
		}
		if _, err := s.repo.AddMemoryChunk(ctx, "scrum-coach", model.MemoryKindReference, content, noteTags, embedding); err != nil {
			return stored, fmt.Errorf("store memory note %d: %w", i, err)
		}
		stored++
	}
	return stored, nil
}

func (s *Server) handleScrumCardCoachConfig(w http.ResponseWriter, r *http.Request, cardID string) {
	card, _, projectID, err := s.scrumGetCard(r, cardID)
	if err != nil {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg, err := s.scrumCoachConfig(card.CoachConfig)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"coach_config": cfg})
	case http.MethodPut:
		var req ScrumCoachConfig
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if err := requireJSONEOF(decoder, "Scrum coach config request"); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		req, err = validateScrumCoachConfig(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		card.CoachConfig, err = coachConfigToRaw(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
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

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
