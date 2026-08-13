package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/model"
)

func (s *Server) enrichJobMetadata(
	ctx context.Context,
	metadata []byte,
	card ScrumCard,
) ([]byte, []string, error) {
	if len(metadata) == 0 {
		metadata = []byte(`{}`)
	}
	var payload map[string]any
	if err := json.Unmarshal(metadata, &payload); err != nil {
		return metadata, nil, fmt.Errorf("metadata must be a JSON object")
	}
	if payload == nil {
		payload = map[string]any{}
	}
	project, card, err := s.loadMetadataProjectAndCard(ctx, payload, card)
	if err != nil {
		return metadata, nil, err
	}
	instance, err := parseInstanceAgentConfig(payload)
	if err != nil {
		return metadata, nil, err
	}

	var pulled []string
	if _, ok := payload["model_config"]; !ok {
		extra, modelPulled, err := s.modelConfigJobMetadata(ctx, project, card)
		if err != nil {
			return metadata, modelPulled, err
		}
		for key, value := range extra {
			payload[key] = value
		}
		pulled = modelPulled
	}
	if _, ok := payload["agent_config"]; !ok {
		extra, err := s.agentConfigJobMetadata(ctx, project, card, instance)
		if err != nil {
			return metadata, pulled, err
		}
		for key, value := range extra {
			payload[key] = value
		}
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return metadata, pulled, err
	}
	if _, err := agentconfig.FromJobMetadata(out); err != nil {
		return metadata, pulled, err
	}
	return out, pulled, nil
}

func (s *Server) loadMetadataProjectAndCard(
	ctx context.Context,
	payload map[string]any,
	card ScrumCard,
) (model.Project, ScrumCard, error) {
	project := model.Project{}
	projectID := metadataInt64(payload, "project_id")
	if projectID > 0 && s.repo != nil {
		loaded, err := s.repo.GetProject(ctx, projectID)
		if err != nil {
			return project, card, fmt.Errorf("load metadata project %d: %w", projectID, err)
		}
		project = loaded
	}
	if card.ID != "" {
		return project, card, nil
	}
	cardID := metadataString(payload, "scrum_card_id")
	if cardID == "" || projectID <= 0 || s.repo == nil {
		return project, card, nil
	}
	dbCard, err := s.repo.GetScrumCard(ctx, projectID, cardID)
	if err != nil {
		return project, card, fmt.Errorf("load metadata Scrum card %q: %w", cardID, err)
	}
	card, err = dbScrumCardToAPI(dbCard)
	if err != nil {
		return project, card, fmt.Errorf("decode metadata Scrum card %q: %w", cardID, err)
	}
	return project, card, nil
}

func parseInstanceAgentConfig(payload map[string]any) (agentconfig.Config, error) {
	raw, ok := payload["instance_agent_config"]
	if !ok || raw == nil {
		return agentconfig.Config{}, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return agentconfig.Config{}, fmt.Errorf("encode instance_agent_config: %w", err)
	}
	instance, err := agentconfig.FromJSON(encoded)
	if err != nil {
		return agentconfig.Config{}, fmt.Errorf("parse instance_agent_config: %w", err)
	}
	return instance, nil
}

func metadataInt64(payload map[string]any, key string) int64 {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return 0
	}
	switch value := raw.(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	case json.Number:
		parsed, _ := value.Int64()
		return parsed
	default:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(value)), 10, 64)
		return parsed
	}
}

func metadataString(payload map[string]any, key string) string {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}
