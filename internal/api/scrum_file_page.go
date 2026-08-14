package api

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/hostbridge"
)

const scrumFilePageSize = 50

func (s *Server) scrumProjectFilePage(
	r *http.Request,
	projectRoot string,
	path string,
	offset int,
) (*hostbridge.BrowseResult, error) {
	if r == nil || s == nil || s.repo == nil {
		return nil, fmt.Errorf("request and PostgreSQL are required for Scrum file paging")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("Scrum project file path is required")
	}
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return nil, fmt.Errorf("Scrum project file root is required")
	}
	opts := hostbridge.BrowseOptions{
		Limit: scrumFilePageSize, Offset: offset,
		ExtraRoots: []string{projectRoot}, RequiredRoot: projectRoot,
	}
	if projectPathAccessibleLocally(projectRoot) {
		result, err := hostbridge.ListDirectory(path, opts)
		if err != nil {
			return nil, fmt.Errorf("browse Scrum project files: %w", err)
		}
		if err := validateScrumFilePage(*result, opts); err != nil {
			return nil, err
		}
		return result, nil
	}
	if client := s.hostBridgeClient(); client != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		mappedRoot, ok := mapWorkspacePathForHostBridge(projectRoot)
		if !ok {
			return nil, fmt.Errorf("Scrum host-only project requires exact WORKSPACE_ROOT to HOST_WORKSPACE_PATH mapping")
		}
		rel, err := filepath.Rel(filepath.Clean(projectRoot), filepath.Clean(path))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("Scrum host-only file path escaped the project root")
		}
		resolved := filepath.Join(mappedRoot, rel)
		opts.RequiredRoot = mappedRoot
		result, err := client.Browse(ctx, resolved, opts)
		if err != nil {
			return nil, fmt.Errorf("browse Scrum project files through host bridge: %w", err)
		}
		if err := validateScrumFilePage(*result, opts); err != nil {
			return nil, err
		}
		return result, nil
	}
	return nil, fmt.Errorf("Scrum project file path is not accessible locally and host bridge is unavailable")
}

func validateScrumFilePage(result hostbridge.BrowseResult, opts hostbridge.BrowseOptions) error {
	if result.Limit != opts.Limit || result.Offset != opts.Offset || len(result.Entries) > opts.Limit {
		return fmt.Errorf("Scrum file page does not match the requested bounds")
	}
	if result.HasPrevious != (result.Offset > 0) || result.PreviousOffset < 0 ||
		(result.HasPrevious && result.PreviousOffset >= result.Offset) {
		return fmt.Errorf("Scrum file page returned invalid previous-page authority")
	}
	if result.HasMore != (result.NextOffset > result.Offset) {
		return fmt.Errorf("Scrum file page returned invalid next-page authority")
	}
	return nil
}
