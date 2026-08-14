package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func (s *Server) resolveProjectID(r *http.Request) (int64, error) {
	if s.repo == nil {
		return 0, fmt.Errorf("database unavailable")
	}
	if r == nil || r.URL == nil {
		return 0, fmt.Errorf("project_id is required")
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return 0, fmt.Errorf("decode project query: %w", err)
	}
	raw, present := oneQueryValue(values, "project_id")
	if !present {
		if items, exists := values["project_id"]; exists && len(items) != 1 {
			return 0, fmt.Errorf("project_id must occur exactly once")
		}
		return 0, fmt.Errorf("project_id is required")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != raw {
		return 0, fmt.Errorf("project_id must be one canonical positive integer")
	}
	return id, nil
}

func (s *Server) scrumBoardMetadataFromProject(ctx context.Context, projectID int64) (ScrumBoard, error) {
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return ScrumBoard{}, err
	}
	return ScrumBoard{
		ID:               fmt.Sprintf("project_%d", projectID),
		Name:             project.Name,
		ProjectDirectory: project.Location,
		Columns:          append([]string(nil), scrumColumns...),
		Cards:            []ScrumCard{},
		UpdatedAt:        project.UpdatedAt.UTC().Format(time.RFC3339),
	}, nil
}
