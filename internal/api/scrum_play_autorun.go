package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gryph/omnidex/internal/model"
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

// OnJobFinishedAsync handles post-job side effects without requiring the web UI.
func (s *Server) OnJobFinishedAsync(jobID int64) {
	if s == nil {
		log.Printf("job-finished hook rejected job=%d: server is nil", jobID)
		return
	}
	if s.repo == nil {
		log.Printf("job-finished hook rejected job=%d: PostgreSQL repository is unavailable", jobID)
		return
	}
	if jobID <= 0 {
		log.Printf("job-finished hook rejected job=%d: job ID must be positive", jobID)
		return
	}
	s.publishJobProgress(jobID, realtimeJobFinished, "Job finished; reconciling final server state")
	if err := s.RefreshScrumPlayQueueForJobAsync(jobID); err != nil {
		log.Printf("Scrum play queue scheduling rejected job=%d: %v", jobID, err)
	}
}

// RefreshScrumPlayQueueForJobAsync advances scrum play state after a terminal job.
func (s *Server) RefreshScrumPlayQueueForJobAsync(jobID int64) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("Scrum play queue refresh requires a PostgreSQL repository")
	}
	if jobID <= 0 {
		return fmt.Errorf("Scrum play queue refresh requires a positive job ID")
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
		if err := s.refreshScrumPlayQueueForJob(ctx, jobID); err != nil {
			log.Printf("scrum play queue refresh job=%d: %v", jobID, err)
			return
		}
		if err := s.refreshScrumAutoWork(ctx); err != nil {
			log.Printf("scrum global auto-work refresh after job=%d: %v", jobID, err)
		}
	}()
	return nil
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

func (s *Server) refreshScrumPlayQueueForJob(ctx context.Context, jobID int64) error {
	details, err := s.repo.CurrentJobDetails(ctx, jobID)
	if err != nil {
		return err
	}
	switch details.Job.Status {
	case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
	default:
		return nil
	}
	if details.Job.Pipeline != model.PipelineScrum {
		return nil
	}
	projectID, err := s.repo.JobProjectID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load project authority for Scrum job %d: %w", jobID, err)
	}
	if projectID <= 0 {
		return fmt.Errorf("Scrum job %d is missing its durable project authority", jobID)
	}
	return s.refreshScrumPlayQueueForProject(ctx, projectID, "job finished")
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
