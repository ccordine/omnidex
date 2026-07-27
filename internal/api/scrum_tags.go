package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/scrumcardllm"
)

func (s *Server) handleScrumTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 40)
	tags, err := s.collectScrumTagCatalog(r.Context(), r, query, limit)
	if err != nil {
		status := http.StatusInternalServerError
		if s.repo == nil {
			status = http.StatusServiceUnavailable
		} else if strings.Contains(err.Error(), "project_id") {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

func (s *Server) handleScrumCardTagsSuggest(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	card, board, projectID, err := s.scrumGetCard(r, cardID)
	if err != nil {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}
	if s.repo == nil || projectID <= 0 {
		writeError(w, http.StatusServiceUnavailable, "tag suggestion requires a project database")
		return
	}
	cfg, err := s.scrumCoachConfig(card.CoachConfig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	job, updated, err := s.enqueueScrumCardLLMJob(r.Context(), projectID, card, scrumcardllm.ActionTagsSuggest, cfg.Model, "", scrumcardllm.TicketRequest{})
	if err != nil {
		writeScrumCardLLMEnqueueError(w, err)
		return
	}
	writeScrumCardLLMQueued(w, job, updated, fmt.Sprintf("Queued tag suggestion job #%d for %s", job.ID, board.Name))
}

func scrumBoardContext(board ScrumBoard) scrumcardllm.BoardContext {
	return scrumcardllm.BoardContext{
		Name:             board.Name,
		ProjectDirectory: board.ProjectDirectory,
	}
}

func scrumCardContext(card ScrumCard) scrumcardllm.CardContext {
	out := scrumcardllm.CardContext{
		ID:          card.ID,
		Title:       card.Title,
		Description: card.Description,
		Column:      card.Column,
		RefFiles:    append([]string(nil), card.RefFiles...),
		Tags:        append([]string(nil), card.Tags...),
		CardPrompt:  card.CardPrompt,
		CardTicket:  card.CardTicket,
	}
	for _, item := range card.Checklist {
		out.Checklist = append(out.Checklist, scrumcardllm.ChecklistItem{Text: item.Text, Done: item.Done})
	}
	for _, item := range card.TestCriteria {
		out.TestCriteria = append(out.TestCriteria, scrumcardllm.ChecklistItem{Text: item.Text, Done: item.Done})
	}
	return out
}

func (s *Server) collectScrumTagCatalog(ctx context.Context, r *http.Request, query string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 40
	}
	if s.repo == nil {
		return nil, fmt.Errorf("postgres repository is required for Scrum tags")
	}
	projectID, err := s.resolveProjectID(r)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	add := func(values ...string) {
		for _, value := range values {
			tag := strings.ToLower(strings.TrimSpace(value))
			if tag == "" {
				continue
			}
			if query != "" && !strings.Contains(tag, query) {
				continue
			}
			seen[tag] = struct{}{}
		}
	}

	facets, err := s.repo.ListMemoryTags(ctx, 200)
	if err != nil {
		return nil, fmt.Errorf("list memory tags: %w", err)
	}
	for _, facet := range facets {
		add(facet.Name)
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("load Scrum project: %w", err)
	}
	var settings map[string]any
	if len(project.Settings) > 0 {
		if err := json.Unmarshal(project.Settings, &settings); err != nil {
			return nil, fmt.Errorf("decode Scrum project settings: %w", err)
		}
	}
	if raw, ok := settings["tags"].([]any); ok {
		for _, item := range raw {
			if text, ok := item.(string); ok {
				add(text)
			}
		}
	}
	cards, err := s.repo.ListScrumCards(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list Scrum cards for tags: %w", err)
	}
	for _, card := range cards {
		var tags []string
		if len(card.Tags) > 0 {
			if err := json.Unmarshal(card.Tags, &tags); err != nil {
				return nil, fmt.Errorf("decode Scrum card %s tags: %w", card.ID, err)
			}
		}
		add(tags...)
	}

	out := make([]string, 0, len(seen))
	for tag := range seen {
		out = append(out, tag)
	}
	sort.Strings(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
