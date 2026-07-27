package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/scrumcardllm"
)

const scrumPlayJobSource = "omni-scrum"

type scrumJobReference struct {
	IsScrum   bool
	ProjectID int64
	CardID    string
}

func parseScrumJobReference(metadataJSON []byte) (scrumJobReference, error) {
	if len(bytes.TrimSpace(metadataJSON)) == 0 {
		return scrumJobReference{}, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(metadataJSON, &payload); err != nil {
		return scrumJobReference{}, fmt.Errorf("parse job metadata: %w", err)
	}
	rawSource, ok := payload["source"]
	if !ok {
		return scrumJobReference{}, nil
	}
	var source string
	if err := json.Unmarshal(rawSource, &source); err != nil {
		return scrumJobReference{}, fmt.Errorf("job metadata source must be a string: %w", err)
	}
	source = strings.TrimSpace(source)
	switch source {
	case scrumPlayJobSource:
		projectID, err := requiredPositiveMetadataInt64(payload, "project_id")
		if err != nil {
			return scrumJobReference{}, err
		}
		cardID, err := requiredMetadataString(payload, "scrum_card_id")
		if err != nil {
			return scrumJobReference{}, err
		}
		return scrumJobReference{IsScrum: true, ProjectID: projectID, CardID: cardID}, nil
	case scrumcardllm.JobSource:
		meta, err := scrumcardllm.ParseJobReference(metadataJSON)
		if err != nil {
			return scrumJobReference{}, fmt.Errorf("parse Scrum card LLM metadata: %w", err)
		}
		return scrumJobReference{IsScrum: true, ProjectID: meta.ProjectID, CardID: meta.CardID}, nil
	default:
		return scrumJobReference{}, nil
	}
}

func requiredPositiveMetadataInt64(payload map[string]json.RawMessage, key string) (int64, error) {
	raw, ok := payload[key]
	if !ok {
		return 0, fmt.Errorf("%s is required for a Scrum job", key)
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("%s must be a positive integer: %w", key, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return value, nil
}

func requiredMetadataString(payload map[string]json.RawMessage, key string) (string, error) {
	raw, ok := payload[key]
	if !ok {
		return "", fmt.Errorf("%s is required for a Scrum job", key)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%s must be a string: %w", key, err)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required for a Scrum job", key)
	}
	return value, nil
}

func (s *Server) authoritativeScrumJobProjectID(ctx context.Context, jobID int64, ref scrumJobReference) (int64, error) {
	if s == nil || s.repo == nil {
		return 0, fmt.Errorf("Scrum job reconciliation requires a PostgreSQL repository")
	}
	if ctx == nil {
		return 0, fmt.Errorf("Scrum job reconciliation requires a context")
	}
	if jobID <= 0 {
		return 0, fmt.Errorf("Scrum job reconciliation requires a positive job ID")
	}
	if !ref.IsScrum || ref.ProjectID <= 0 {
		return 0, fmt.Errorf("job %d does not contain a valid Scrum project reference", jobID)
	}
	projectID, err := s.repo.JobProjectID(ctx, jobID)
	if err != nil {
		return 0, fmt.Errorf("load project authority for Scrum job %d: %w", jobID, err)
	}
	if projectID <= 0 {
		return 0, fmt.Errorf("Scrum job %d is missing its durable project_id", jobID)
	}
	if projectID != ref.ProjectID {
		return 0, fmt.Errorf(
			"Scrum job %d project mismatch: jobs.project_id=%d metadata.project_id=%d",
			jobID,
			projectID,
			ref.ProjectID,
		)
	}
	return projectID, nil
}
