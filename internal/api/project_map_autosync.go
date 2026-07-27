package api

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

const projectMapAutoSyncTimeout = 2 * time.Minute

// SyncProjectMapForJobAsync rescans the authoritative project after a terminal job.
func (s *Server) SyncProjectMapForJobAsync(jobID int64) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("project map auto-sync requires a PostgreSQL repository")
	}
	if jobID <= 0 {
		return fmt.Errorf("project map auto-sync requires a positive job ID")
	}
	if s.lifecycleContext == nil {
		return ErrRealtimeLifecycleUnavailable
	}
	if err := s.lifecycleContext.Err(); err != nil {
		return fmt.Errorf("project map auto-sync lifecycle ended: %w", err)
	}
	go func() {
		ctx, cancel := context.WithTimeout(s.lifecycleContext, projectMapAutoSyncTimeout)
		defer cancel()
		projectID, err := s.syncProjectMapForJob(ctx, jobID)
		if err != nil {
			log.Printf("project map auto-sync job=%d: %v", jobID, err)
			return
		}
		if projectID > 0 {
			log.Printf("project map auto-sync job=%d project=%d ok", jobID, projectID)
		}
	}()
	return nil
}

func (s *Server) syncProjectMapByID(ctx context.Context, projectID int64) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("project map sync requires a PostgreSQL repository")
	}
	if ctx == nil {
		return fmt.Errorf("project map sync requires a context")
	}
	if projectID <= 0 {
		return fmt.Errorf("project map sync requires a positive project ID")
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	_, err = s.scanProjectCodebaseMap(ctx, project)
	return err
}

func (s *Server) syncProjectMapForJob(ctx context.Context, jobID int64) (int64, error) {
	if s == nil || s.repo == nil {
		return 0, fmt.Errorf("project map sync requires a PostgreSQL repository")
	}
	if ctx == nil {
		return 0, fmt.Errorf("project map sync requires a context")
	}
	if jobID <= 0 {
		return 0, fmt.Errorf("project map sync requires a positive job ID")
	}
	details, err := s.repo.GetJobDetails(ctx, jobID)
	if err != nil {
		return 0, err
	}
	switch details.Job.Status {
	case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
	default:
		return 0, nil
	}

	projectID, err := s.repo.JobProjectID(ctx, jobID)
	if err != nil {
		return 0, fmt.Errorf("load project authority for job %d: %w", jobID, err)
	}
	if projectID <= 0 {
		return 0, nil
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return 0, err
	}
	if _, err := s.scanProjectCodebaseMap(ctx, project); err != nil {
		return 0, err
	}
	return project.ID, nil
}
