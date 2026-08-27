package worker

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/station"
)

func modelRoutingFromJobMetadata(metadata json.RawMessage, base ModelRouting) (ModelRouting, error) {
	if len(metadata) == 0 {
		return base, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(metadata, &payload); err != nil {
		return ModelRouting{}, fmt.Errorf("parse job model routing metadata: %w", err)
	}
	if err := rejectRemovedJobModelAliases(payload); err != nil {
		return ModelRouting{}, err
	}
	cfg := modelconfig.Config{}
	if raw, ok := payload["model_config"]; ok {
		bytes, err := json.Marshal(raw)
		if err != nil {
			return ModelRouting{}, fmt.Errorf("encode job model config: %w", err)
		}
		cfg, err = modelconfig.FromJSON(bytes)
		if err != nil {
			return ModelRouting{}, fmt.Errorf("parse job model config: %w", err)
		}
	}
	if len(cfg) == 0 {
		return base, nil
	}
	baseRouting := modelconfig.Routing{
		Stations: base.Stations, RoleplaySemanticModel: base.RoleplaySemanticModel,
	}
	applied := modelconfig.Apply(baseRouting, cfg)
	return ModelRouting{
		Stations: applied.Stations, RoleplaySemanticModel: applied.RoleplaySemanticModel,
	}, nil
}

func rejectRemovedJobModelAliases(payload map[string]any) error {
	for _, key := range []string{
		"model_plan",
		"model_analyze",
		"model_response",
		"model_search",
		"model_tagger",
		"model_verify",
		"model_memory",
		"model_execute",
		"model_source_review",
		"model_source",
		"model_fragment",
	} {
		if _, exists := payload[key]; exists {
			return fmt.Errorf("job model routing metadata %s was removed; configure an exact station field inside model_config", key)
		}
	}
	return nil
}

func stationModel(routing ModelRouting, id station.ID) (string, error) {
	if err := id.Validate(); err != nil {
		return "", err
	}
	if configured := strings.TrimSpace(routing.Stations[id]); configured != "" {
		return configured, nil
	}
	return "", fmt.Errorf("semantic station %q has no configured model", id)
}

func (s *Service) requiredStationModel(routing ModelRouting, id station.ID) (string, error) {
	if s == nil {
		return "", fmt.Errorf("semantic station %q requires an active worker service", id)
	}
	return stationModel(routing, id)
}

func (s *Service) requiredRoleplaySemanticModel(routing ModelRouting) (string, error) {
	if s == nil {
		return "", fmt.Errorf("roleplay semantic stations require an active worker service")
	}
	if configured := strings.TrimSpace(routing.RoleplaySemanticModel); configured != "" {
		return configured, nil
	}
	return "", fmt.Errorf("roleplay semantic model is not configured")
}
