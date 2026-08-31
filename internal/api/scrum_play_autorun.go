package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const scrumPlayAutoRunTimeout = 2 * time.Minute

func scrumRequestFromContext(ctx context.Context) *http.Request {
	if ctx == nil {
		panic("Scrum request requires a non-nil context")
	}
	return (&http.Request{URL: &url.URL{}}).WithContext(ctx)
}

func scrumRequestForProject(ctx context.Context, projectID int64) *http.Request {
	r := scrumRequestFromContext(ctx)
	if projectID <= 0 {
		return r
	}
	q := r.URL.Query()
	q.Set("project_id", strconv.FormatInt(projectID, 10))
	r.URL.RawQuery = q.Encode()
	return r
}

// RefreshScrumPlayQueueForProjectAsync reconciles play queue state and advances global auto-work.
func (s *Server) RefreshScrumPlayQueueForProjectAsync(projectID int64) error {
	return s.refreshScrumPlayQueueForProjectAsync(projectID, "project refresh", true)
}

// ReconcileScrumPlayQueueForProjectAsync reconciles jobs without starting new auto-work.
func (s *Server) ReconcileScrumPlayQueueForProjectAsync(projectID int64) error {
	return s.refreshScrumPlayQueueForProjectAsync(projectID, "project reconcile", false)
}

func (s *Server) refreshScrumPlayQueueForProjectAsync(projectID int64, reason string, advanceAutoWork bool) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("Scrum play queue refresh requires a PostgreSQL repository")
	}
	if projectID <= 0 {
		return fmt.Errorf("Scrum play queue refresh requires a positive project ID")
	}
	if s.lifecycleContext == nil {
		return ErrRealtimeLifecycleUnavailable
	}
	if err := s.lifecycleContext.Err(); err != nil {
		return fmt.Errorf("Scrum play queue refresh lifecycle ended: %w", err)
	}
	go func() {
		ctx, cancel := context.WithTimeout(s.lifecycleContext, scrumPlayAutoRunTimeout)
		defer cancel()
		if !advanceAutoWork {
			ctx = context.WithValue(ctx, scrumAutoWorkHandoffSuppressedKey{}, true)
		}
		if err := s.refreshScrumPlayQueueForProject(ctx, projectID, reason); err != nil {
			s.publishScrumRealtimeFailure(projectID, reason, err)
			return
		}
		if advanceAutoWork {
			if err := s.refreshScrumAutoWork(ctx); err != nil {
				s.publishScrumRealtimeFailure(projectID, "global auto-work after "+reason, err)
			}
		}
	}()
	return nil
}

func (s *Server) refreshScrumPlayQueueForProject(ctx context.Context, projectID int64, reason string) error {
	board, err := s.scrumBoardMetadataFromProject(ctx, projectID)
	if err != nil {
		return err
	}
	r := scrumRequestForProject(ctx, projectID)
	_, err = s.refreshScrumPlayQueue(r, projectID, board)
	return err
}
