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
	tags := s.collectScrumTagCatalog(r.Context(), r, query, limit)
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
	if s.repo != nil && projectID > 0 {
		cfg := parseScrumCoachConfig(card.CoachConfig)
		job, updated, err := s.enqueueScrumCardLLMJob(r.Context(), projectID, card, scrumcardllm.ActionTagsSuggest, cfg.Model, "", scrumcardllm.TicketRequest{})
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeScrumCardLLMQueued(w, job, updated, fmt.Sprintf("Queued tag suggestion job #%d for %s", job.ID, board.Name))
		return
	}
	updated, suggested, notes, err := s.runScrumCardTagsSuggestSync(r, board, projectID, card)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"card":  updated,
		"tags":  suggested,
		"notes": notes,
	})
}

func (s *Server) runScrumCardTagsSuggestSync(r *http.Request, board ScrumBoard, projectID int64, card ScrumCard) (ScrumCard, []string, string, error) {
	cfg := parseScrumCoachConfig(card.CoachConfig)
	knownTags := s.collectScrumTagCatalog(r.Context(), r, "", 80)
	system, user := scrumcardllm.TagsSuggestPrompts(scrumBoardContext(board), scrumCardContext(card), knownTags)
	result, err := scrumcardllm.RunTagsSuggest(r.Context(), s.llmClient, cfg.Model, system, user)
	if err != nil {
		return ScrumCard{}, nil, "", err
	}
	if len(result.Suggested) == 0 {
		return card, []string{}, result.Notes, nil
	}
	card.Tags = mergeTags(card.Tags, result.Suggested)
	updated, err := s.persistScrumCard(r, projectID, card)
	if err != nil {
		return ScrumCard{}, nil, "", err
	}
	if s.repo != nil && projectID > 0 {
		_ = s.mergeProjectTags(r.Context(), projectID, result.Suggested)
	}
	return updated, result.Suggested, result.Notes, nil
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

func (s *Server) collectScrumTagCatalog(ctx context.Context, r *http.Request, query string, limit int) []string {
	if limit <= 0 {
		limit = 40
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

	if s.repo != nil {
		facets, err := s.repo.ListMemoryTags(ctx, 200)
		if err == nil {
			for _, facet := range facets {
				add(facet.Name)
			}
		}
		projectID, err := s.resolveProjectID(r)
		if err == nil && projectID > 0 {
			if project, err := s.repo.GetProject(ctx, projectID); err == nil {
				var settings map[string]any
				if len(project.Settings) > 0 {
					_ = json.Unmarshal(project.Settings, &settings)
				}
				if raw, ok := settings["tags"].([]any); ok {
					for _, item := range raw {
						if text, ok := item.(string); ok {
							add(text)
						}
					}
				}
			}
			if cards, err := s.repo.ListScrumCards(ctx, projectID); err == nil {
				for _, card := range cards {
					var tags []string
					_ = json.Unmarshal(card.Tags, &tags)
					add(tags...)
				}
			}
		}
	}

	if s.scrumStore != nil {
		board := s.scrumStore.Board()
		for _, card := range board.Cards {
			add(card.Tags...)
		}
	}

	out := make([]string, 0, len(seen))
	for tag := range seen {
		out = append(out, tag)
	}
	sort.Strings(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
