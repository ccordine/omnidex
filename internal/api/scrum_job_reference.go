package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/queue"
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
	if !utf8.Valid(metadataJSON) {
		return scrumJobReference{}, fmt.Errorf("job metadata must be valid UTF-8")
	}
	if err := exactjson.ValidateUniqueObject(metadataJSON, "job metadata"); err != nil {
		return scrumJobReference{}, fmt.Errorf("parse job metadata: %w", err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(metadataJSON, &payload); err != nil {
		return scrumJobReference{}, fmt.Errorf("parse job metadata: %w", err)
	}
	for key := range payload {
		for _, registered := range []string{"source", "project_id", "scrum_card_id"} {
			if key != registered && strings.EqualFold(key, registered) {
				return scrumJobReference{}, fmt.Errorf(
					"job metadata contains inexact alias %q for %q", key, registered,
				)
			}
		}
	}
	rawSource, ok := payload["source"]
	if !ok {
		return scrumJobReference{}, nil
	}
	var source string
	if err := json.Unmarshal(rawSource, &source); err != nil {
		return scrumJobReference{}, fmt.Errorf("job metadata source must be a string: %w", err)
	}
	switch source {
	case scrumPlayJobSource:
		for _, key := range []string{
			"action", "scrum_action", "scrum_card_action", "scrum_raw_play", "omnidex_no_delegate",
			"agent_config", "agent_config_source", "instance_agent_config", "external_agents_used",
			"execution_agent", "agent_strict", "recipe_id", "recipe",
		} {
			if _, retired := payload[key]; retired {
				return scrumJobReference{}, fmt.Errorf(
					"Scrum job metadata selector %q was retired and is not runtime authority", key,
				)
			}
		}
		projectID, err := requiredPositiveMetadataInt64(payload, "project_id")
		if err != nil {
			return scrumJobReference{}, err
		}
		cardID, err := requiredCanonicalScrumCardID(payload, "scrum_card_id")
		if err != nil {
			return scrumJobReference{}, err
		}
		return scrumJobReference{IsScrum: true, ProjectID: projectID, CardID: cardID}, nil
	case "scrum_card_llm":
		return scrumJobReference{}, fmt.Errorf("job metadata source %q was retired", source)
	default:
		if strings.EqualFold(strings.TrimSpace(source), scrumPlayJobSource) ||
			strings.EqualFold(strings.TrimSpace(source), "scrum_card_llm") {
			return scrumJobReference{}, fmt.Errorf("job metadata source %q is not exact registered authority", source)
		}
		return scrumJobReference{}, nil
	}
}

func requiredPositiveMetadataInt64(payload map[string]json.RawMessage, key string) (int64, error) {
	raw, ok := payload[key]
	if !ok {
		return 0, fmt.Errorf("%s is required for a Scrum job", key)
	}
	value, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || value <= 0 || strconv.FormatInt(value, 10) != string(raw) {
		return 0, fmt.Errorf("%s must be one canonical positive integer", key)
	}
	return value, nil
}

func requiredCanonicalScrumCardID(payload map[string]json.RawMessage, key string) (string, error) {
	raw, ok := payload[key]
	if !ok {
		return "", fmt.Errorf("%s is required for a Scrum job", key)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%s must be a string: %w", key, err)
	}
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsRune(value, '\x00') ||
		len(value) > queue.MaxScrumCardIDBytes {
		return "", fmt.Errorf(
			"%s must be one canonical Scrum card ID bounded to %d bytes",
			key,
			queue.MaxScrumCardIDBytes,
		)
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
