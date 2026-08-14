package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
)

func (s *Server) handleScrumTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query, err := decodeScrumTagCatalogQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tags, err := s.collectScrumTagCatalog(r.Context(), query.ProjectID, query.Search, query.Limit)
	if err != nil {
		status := http.StatusInternalServerError
		if s.repo == nil {
			status = http.StatusServiceUnavailable
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

func (s *Server) collectScrumTagCatalog(ctx context.Context, projectID int64, query string, limit int) ([]string, error) {
	if limit < 1 || limit > queue.MaxScrumTagPageSize {
		return nil, fmt.Errorf("Scrum tag catalog limit must be between 1 and %d", queue.MaxScrumTagPageSize)
	}
	if projectID <= 0 {
		return nil, fmt.Errorf("Scrum tag catalog requires a positive project ID")
	}
	if s.repo == nil {
		return nil, fmt.Errorf("postgres repository is required for Scrum tags")
	}
	foldedQuery := canonicalScrumTag(query)
	seen := map[string]struct{}{}
	add := func(values ...string) {
		for _, value := range values {
			tag := canonicalScrumTag(value)
			if tag == "" {
				continue
			}
			if foldedQuery != "" && !strings.Contains(tag, foldedQuery) {
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
	_, err = s.repo.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("load Scrum project: %w", err)
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
