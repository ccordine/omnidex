package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gryph/omnidex/internal/model"
)

type projectMutationCommitState string

const (
	projectMutationCommitted         projectMutationCommitState = "committed"
	projectMutationCommittedDegraded projectMutationCommitState = "committed_degraded"
)

type projectMutationResponse struct {
	CommitState    projectMutationCommitState `json:"commit_state"`
	Project        map[string]any             `json:"project"`
	OperationError string                     `json:"operation_error,omitempty"`
}

type projectDeleteResponse struct {
	CommitState       projectMutationCommitState `json:"commit_state"`
	ProjectID         int64                      `json:"project_id"`
	ExpectedUpdatedAt string                     `json:"expected_updated_at"`
	Deleted           bool                       `json:"deleted"`
}

func extractSettingsModelConfig(settings json.RawMessage) (json.RawMessage, error) {
	return extractSettingsJSONObject(settings, "model_config")
}

func (s *Server) projectSummary(ctx context.Context, project model.Project) (map[string]any, error) {
	jobs, err := s.repo.CountProjectJobs(ctx, project.ID)
	if err != nil {
		return nil, fmt.Errorf("count jobs for project %d: %w", project.ID, err)
	}
	cards, err := s.repo.CountProjectCards(ctx, project.ID)
	if err != nil {
		return nil, fmt.Errorf("count cards for project %d: %w", project.ID, err)
	}
	summary := projectCoreProjection(project)
	summary["job_count"] = jobs
	summary["card_count"] = cards
	return summary, nil

}

func projectCoreProjection(project model.Project) map[string]any {
	return map[string]any{
		"id":            project.ID,
		"name":          project.Name,
		"location":      project.Location,
		"description":   project.Description,
		"project_state": project.ProjectState,
		"last_seen_at":  project.LastSeenAt,
		"created_at":    project.CreatedAt,
		"updated_at":    project.UpdatedAt,
	}
}

func writeCommittedProjectMutation(
	w http.ResponseWriter,
	project model.Project,
	failedEffect string,
	err error,
) {
	if err == nil {
		panic("committed project mutation failure requires an error")
	}
	detail := fmt.Sprintf("project %d was committed but its %s failed: %v", project.ID, failedEffect, err)
	log.Printf("Project mutation committed with a degraded post-state: %s", detail)
	writeJSON(w, http.StatusMultiStatus, projectMutationResponse{
		CommitState:    projectMutationCommittedDegraded,
		Project:        projectCoreProjection(project),
		OperationError: detail,
	})
}

func jsonRawOrObject(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func extractSettingsJSONObject(settings json.RawMessage, key string) (json.RawMessage, error) {
	if len(settings) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(settings, &root); err != nil {
		return nil, err
	}
	raw, ok := root[key]
	if !ok || len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("%s must be a JSON object: %w", key, err)
	}
	if object == nil {
		return nil, fmt.Errorf("%s must be a JSON object", key)
	}
	return raw, nil
}
