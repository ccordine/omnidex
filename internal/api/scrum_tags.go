package api

import (
	"context"
	"encoding/json"
	"fmt"
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
	writeRemovedInferenceAction(w, "Scrum card tag suggestion")
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
	cardTags, err := s.repo.ListScrumCardTags(ctx, projectID, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list Scrum cards for tags: %w", err)
	}
	add(cardTags...)

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
