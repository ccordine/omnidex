package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/scrum"
	"github.com/gryph/omnidex/internal/scrumcardllm"
)

const scrumPlayAutoRunTimeout = 2 * time.Minute

func scrumRequestFromContext(ctx context.Context) *http.Request {
	if ctx == nil {
		ctx = context.Background()
	}
	return (&http.Request{}).WithContext(ctx)
}

// OnJobFinishedAsync handles post-job side effects without requiring the web UI.
func (s *Server) OnJobFinishedAsync(jobID int64) {
	s.SyncProjectMapForJobAsync(jobID)
	s.RefreshScrumPlayQueueForJobAsync(jobID)
}

// RefreshScrumPlayQueueForJobAsync advances scrum play state after a terminal job.
func (s *Server) RefreshScrumPlayQueueForJobAsync(jobID int64) {
	if s == nil || s.repo == nil || jobID <= 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), scrumPlayAutoRunTimeout)
		defer cancel()
		if err := s.refreshScrumPlayQueueForJob(ctx, jobID); err != nil {
			log.Printf("scrum play queue refresh job=%d: %v", jobID, err)
		}
	}()
}

// RefreshScrumPlayQueueForProjectAsync reconciles play queue state and kicks auto-work.
func (s *Server) RefreshScrumPlayQueueForProjectAsync(projectID int64) {
	if s == nil || s.repo == nil || projectID <= 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), scrumPlayAutoRunTimeout)
		defer cancel()
		if err := s.refreshScrumPlayQueueForProject(ctx, projectID, "project refresh"); err != nil {
			log.Printf("scrum play queue refresh project=%d: %v", projectID, err)
		}
	}()
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
	if !isScrumPlayQueueJob(details.Job.Metadata) {
		return nil
	}
	projectID, _ := resolveJobProjectRef(ctx, s.repo, details.Job)
	if projectID <= 0 {
		return nil
	}
	return s.refreshScrumPlayQueueForProject(ctx, projectID, "job finished")
}

func (s *Server) refreshScrumPlayQueueForProject(ctx context.Context, projectID int64, reason string) error {
	board, err := s.scrumBoardFromProject(ctx, projectID)
	if err != nil {
		return err
	}
	r := scrumRequestFromContext(ctx)
	refreshed, err := s.refreshScrumPlayQueue(r, projectID, board)
	if err != nil {
		return err
	}
	s.publishScrumBoardRefresh(ctx, projectID, reason, refreshed)
	return nil
}

func isScrumPlayQueueJob(metadataJSON []byte) bool {
	if len(metadataJSON) == 0 {
		return false
	}
	if scrum.IsScrumJob(metadataJSON) || scrumcardllm.IsJobMetadata(metadataJSON) {
		return true
	}
	var payload map[string]any
	if err := json.Unmarshal(metadataJSON, &payload); err != nil {
		return false
	}
	source, _ := payload["source"].(string)
	return source == "scrum_card_llm"
}
