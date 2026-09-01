package worker

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/station"
)

func modelRoutingFromJobMetadata(metadata json.RawMessage) (ModelRouting, error) {
	if len(metadata) == 0 {
		return ModelRouting{}, fmt.Errorf("job model routing metadata is required")
	}
	var payload struct {
		ModelConfig json.RawMessage `json:"model_config"`
	}
	if err := json.Unmarshal(metadata, &payload); err != nil {
		return ModelRouting{}, fmt.Errorf("parse job model routing metadata: %w", err)
	}
	if len(payload.ModelConfig) == 0 {
		return ModelRouting{}, fmt.Errorf("job model routing metadata requires model_config")
	}
	cfg, err := modelconfig.FromJSON(payload.ModelConfig)
	if err != nil {
		return ModelRouting{}, fmt.Errorf("parse job model config: %w", err)
	}
	return cfg.Routing(), nil
}

func codingScopeModeFromJobMetadata(metadata json.RawMessage) (model.CodingScopeMode, error) {
	if len(metadata) == 0 {
		return "", fmt.Errorf("job coding scope metadata is required")
	}
	var payload struct {
		CodingScopeMode model.CodingScopeMode `json:"coding_scope_mode"`
	}
	if err := json.Unmarshal(metadata, &payload); err != nil {
		return "", fmt.Errorf("parse job coding scope metadata: %w", err)
	}
	if err := payload.CodingScopeMode.Validate(); err != nil {
		return "", fmt.Errorf("parse job coding scope mode: %w", err)
	}
	return payload.CodingScopeMode, nil
}

func stationModel(routing ModelRouting, id station.ID) (string, error) {
	if err := id.Validate(); err != nil {
		return "", err
	}
	configured, exists := routing.Stations[id]
	if !exists {
		return "", fmt.Errorf("semantic station %q has no configured model", id)
	}
	if configured == "" || configured != strings.TrimSpace(configured) ||
		!utf8.ValidString(configured) || strings.ContainsRune(configured, '\x00') {
		return "", fmt.Errorf("semantic station %q must configure one exact canonical model name", id)
	}
	return configured, nil
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
