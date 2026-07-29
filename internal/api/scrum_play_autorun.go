package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

const scrumPlayAutoRunTimeout = 2 * time.Minute
const jobOutputRealtimeWindow = 250 * time.Millisecond

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
	coalescer, err := s.ensureJobOutputCoalescer()
	if err != nil {
		log.Printf("job output final flush rejected job=%d: %v", jobID, err)
	} else {
		coalescer.FlushNow(jobID)
	}
	s.publishJobProgress(jobID, realtimeJobFinished, "Job finished; reconciling final server state")
	if err := s.RefreshScrumPlayQueueForJobAsync(jobID); err != nil {
		log.Printf("Scrum play queue scheduling rejected job=%d: %v", jobID, err)
	}
}

// OnJobOutputAsync streams in-flight job output into scrum card state and realtime.
func (s *Server) OnJobOutputAsync(jobID int64, delta string) {
	if s == nil || s.repo == nil || jobID <= 0 {
		log.Printf("job-output hook rejected job=%d: server, repository, and positive job ID are required", jobID)
		return
	}
	if delta == "" {
		return
	}
	coalescer, err := s.ensureJobOutputCoalescer()
	if err != nil {
		log.Printf("job output realtime coalescer rejected job=%d: %v", jobID, err)
		return
	}
	if err := coalescer.Signal(jobID); err != nil {
		log.Printf("job output realtime signal rejected job=%d: %v", jobID, err)
	}
}

func (s *Server) ensureJobOutputCoalescer() (*jobOutputCoalescer, error) {
	if s.lifecycleContext == nil {
		return nil, ErrRealtimeLifecycleUnavailable
	}
	s.jobOutputOnce.Do(func() {
		s.jobOutputCoalescer = newJobOutputCoalescer(jobOutputRealtimeWindow, s.flushJobOutput)
		go func() {
			<-s.lifecycleContext.Done()
			s.jobOutputCoalescer.Stop()
		}()
	})
	if s.jobOutputCoalescer == nil {
		return nil, fmt.Errorf("job output coalescer initialization failed")
	}
	return s.jobOutputCoalescer, nil
}

func (s *Server) flushJobOutput(jobID int64) {
	if s.lifecycleContext == nil {
		log.Printf("job output flush rejected job=%d: %v", jobID, ErrRealtimeLifecycleUnavailable)
		return
	}
	s.publishJobProgress(jobID, realtimeJobOutput, "Agent produced new output")
	ctx, cancel := context.WithTimeout(s.lifecycleContext, 15*time.Second)
	defer cancel()
	if err := s.refreshScrumCardOutputForJob(ctx, jobID); err != nil {
		log.Printf("scrum card output refresh job=%d: %v", jobID, err)
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
	details, err := s.repo.GetJobDetails(ctx, jobID)
	if err != nil {
		return err
	}
	switch details.Job.Status {
	case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
	default:
		return nil
	}
	ref, err := parseScrumJobReference(details.Job.Metadata)
	if err != nil {
		return fmt.Errorf("job %d metadata: %w", jobID, err)
	}
	if !ref.IsScrum {
		return nil
	}
	projectID, err := s.authoritativeScrumJobProjectID(ctx, jobID, ref)
	if err != nil {
		return err
	}
	return s.refreshScrumPlayQueueForProject(ctx, projectID, "job finished")
}

func (s *Server) refreshScrumCardOutputForJob(ctx context.Context, jobID int64) error {
	details, err := s.repo.GetJobDetails(ctx, jobID)
	if err != nil {
		return err
	}
	ref, err := parseScrumJobReference(details.Job.Metadata)
	if err != nil {
		return fmt.Errorf("job %d metadata: %w", jobID, err)
	}
	if !ref.IsScrum {
		return nil
	}
	projectID, err := s.authoritativeScrumJobProjectID(ctx, jobID, ref)
	if err != nil {
		return err
	}
	cardID := ref.CardID
	dbCard, err := s.repo.GetScrumCard(ctx, projectID, cardID)
	if err != nil {
		return err
	}
	card, err := dbScrumCardToAPI(dbCard)
	if err != nil {
		return fmt.Errorf("decode Scrum card %q for job %d output: %w", cardID, jobID, err)
	}
	updated := card
	if synced, ok := syncRunningJobChannelChat(updated, details); ok {
		updated = synced
	}
	if synced, ok := syncRunningJobConsoleLog(updated, details); ok {
		updated = synced
	}
	if !scrumCardChannelChanged(card, updated) && strings.TrimSpace(card.ConsoleLog) == strings.TrimSpace(updated.ConsoleLog) {
		return nil
	}
	saved, err := s.persistScrumCardFromContext(ctx, projectID, updated)
	if err != nil {
		return err
	}
	s.publishScrumCardUpdate(ctx, projectID, saved, "agent output")
	return nil
}

func (s *Server) refreshScrumPlayQueueForProject(ctx context.Context, projectID int64, reason string) error {
	board, err := s.scrumBoardFromProject(ctx, projectID)
	if err != nil {
		return err
	}
	r := scrumRequestForProject(ctx, projectID)
	_, err = s.refreshScrumPlayQueue(r, projectID, board)
	return err
}
