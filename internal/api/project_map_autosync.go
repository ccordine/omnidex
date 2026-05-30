package api

import (
	"context"
	"encoding/json"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const projectMapAutoSyncTimeout = 2 * time.Minute

// SyncProjectMapAsync rescans the codebase map in the background.
func (s *Server) SyncProjectMapAsync(projectID int64) {
	if s == nil || s.repo == nil || projectID <= 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), projectMapAutoSyncTimeout)
		defer cancel()
		if err := s.syncProjectMapByID(ctx, projectID); err != nil {
			log.Printf("project map auto-sync project=%d: %v", projectID, err)
		}
	}()
}

// SyncProjectMapForJobAsync rescans the project map after a terminal job when a workspace is known.
func (s *Server) SyncProjectMapForJobAsync(jobID int64) {
	if s == nil || s.repo == nil || jobID <= 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), projectMapAutoSyncTimeout)
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
}

func (s *Server) syncProjectMapByID(ctx context.Context, projectID int64) error {
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(project.Location) == "" {
		return nil
	}
	_, err = s.scanProjectCodebaseMap(ctx, project)
	return err
}

func (s *Server) syncProjectMapForJob(ctx context.Context, jobID int64) (int64, error) {
	details, err := s.repo.GetJobDetails(ctx, jobID)
	if err != nil {
		return 0, err
	}
	switch details.Job.Status {
	case model.JobStatusCompleted, model.JobStatusFailed:
	default:
		return 0, nil
	}

	projectID, location := resolveJobProjectRef(ctx, s.repo, details.Job)
	if location == "" && projectID <= 0 {
		return 0, nil
	}

	var project model.Project
	switch {
	case projectID > 0:
		project, err = s.repo.GetProject(ctx, projectID)
		if err != nil {
			return 0, err
		}
	case location != "":
		project, err = s.repo.GetProjectByLocation(ctx, location)
		if err != nil {
			return 0, err
		}
	}
	if strings.TrimSpace(project.Location) == "" {
		return 0, nil
	}
	if _, err := s.scanProjectCodebaseMap(ctx, project); err != nil {
		return 0, err
	}
	return project.ID, nil
}

func resolveJobProjectRef(ctx context.Context, repo *queue.Repository, job model.Job) (projectID int64, location string) {
	if repo != nil {
		if id, err := repo.JobProjectID(ctx, job.ID); err == nil && id > 0 {
			projectID = id
		}
	}
	location = jobWorkspaceLocation(job.Metadata)
	if projectID <= 0 {
		if id := metadataProjectID(job.Metadata); id > 0 {
			projectID = id
		}
	}
	return projectID, location
}

func metadataProjectID(metadataJSON []byte) int64 {
	if len(metadataJSON) == 0 {
		return 0
	}
	var payload map[string]any
	if err := json.Unmarshal(metadataJSON, &payload); err != nil {
		return 0
	}
	raw, ok := payload["project_id"]
	if !ok {
		return 0
	}
	switch v := raw.(type) {
	case float64:
		if v > 0 {
			return int64(v)
		}
	case int64:
		return v
	case int:
		if v > 0 {
			return int64(v)
		}
	}
	return 0
}

func jobWorkspaceLocation(metadataJSON []byte) string {
	if len(metadataJSON) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(metadataJSON, &payload); err != nil {
		return ""
	}
	for _, key := range []string{"project_directory", "client_cwd", "host_env_cwd", "workspace", "project_location"} {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		text, ok := raw.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		return filepath.Clean(text)
	}
	return ""
}
