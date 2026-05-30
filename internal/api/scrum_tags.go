package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
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
	cfg := parseScrumCoachConfig(card.CoachConfig)
	modelName := cfg.Model
	if modelName == "" {
		modelName = "qwen3:4b-thinking"
	}

	existing := s.collectScrumTagCatalog(r.Context(), r, "", 80)
	existingLine := "Known tags: " + strings.Join(existing, ", ")
	contextLines := []string{
		"Scrum card: " + card.Title,
		"Description: " + card.Description,
		"Project: " + board.Name,
		existingLine,
		"Current card tags: " + strings.Join(card.Tags, ", "),
	}
	for _, item := range card.TestCriteria {
		if strings.TrimSpace(item.Text) != "" {
			contextLines = append(contextLines, "Test: "+item.Text)
		}
	}
	system := strings.Join([]string{
		"You suggest concise lowercase tags for scrum cards and project memory.",
		"Tags should describe domain, tech stack, feature area, and work type.",
		"Respond with JSON only (no markdown fences):",
		`{"tags":["tag-one","tag-two"],"notes":"brief rationale"}`,
	}, "\n")
	raw, err := s.scrumCoachLLMGenerate(r.Context(), modelName, system, strings.Join(contextLines, "\n"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	suggested := []string{}
	notes := ""
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		var payload struct {
			Tags  []string `json:"tags"`
			Notes string   `json:"notes"`
		}
		if err := json.Unmarshal([]byte(raw[start:end+1]), &payload); err == nil {
			suggested = payload.Tags
			notes = payload.Notes
		}
	}
	if len(suggested) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"card":  card,
			"tags":  []string{},
			"notes": notes,
		})
		return
	}
	card.Tags = mergeTags(card.Tags, suggested)
	updated, err := s.persistScrumCard(r, projectID, card)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.repo != nil && projectID > 0 {
		_ = s.mergeProjectTags(r.Context(), projectID, suggested)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"card":  updated,
		"tags":  suggested,
		"notes": notes,
	})
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
