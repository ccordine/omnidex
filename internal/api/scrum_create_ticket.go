package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

const scrumCreateTicketKey = "scrum_create_ticket"

type ScrumCreateTicketConfig struct {
	Enabled bool   `json:"enabled"`
	Column  string `json:"column"`
}

func defaultScrumCreateTicketConfig() ScrumCreateTicketConfig {
	return ScrumCreateTicketConfig{Enabled: false, Column: "backlog"}
}

func validateScrumCreateTicketConfig(cfg ScrumCreateTicketConfig) (ScrumCreateTicketConfig, error) {
	if strings.TrimSpace(cfg.Column) == "" {
		cfg.Column = "backlog"
	}
	cfg.Column = normalizeScrumColumn(strings.TrimSpace(cfg.Column))
	switch cfg.Column {
	case "backlog", "ready", "assigned":
		return cfg, nil
	default:
		return ScrumCreateTicketConfig{}, fmt.Errorf("unsupported Scrum create-ticket column %q", cfg.Column)
	}
}

func loadScrumCreateTicketConfig(settings json.RawMessage) (ScrumCreateTicketConfig, error) {
	cfg := defaultScrumCreateTicketConfig()
	if len(bytes.TrimSpace(settings)) == 0 {
		return cfg, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(settings, &payload); err != nil {
		return ScrumCreateTicketConfig{}, fmt.Errorf("decode project settings for Scrum create-ticket config: %w", err)
	}
	raw, ok := payload[scrumCreateTicketKey]
	if !ok || len(raw) == 0 {
		return cfg, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ScrumCreateTicketConfig{}, fmt.Errorf("Scrum create-ticket config must be an object")
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return ScrumCreateTicketConfig{}, fmt.Errorf("decode Scrum create-ticket config: %w", err)
	}
	return validateScrumCreateTicketConfig(cfg)
}

func (s *Server) saveScrumCreateTicketConfig(ctx context.Context, project model.Project, cfg ScrumCreateTicketConfig) error {
	if s == nil || s.repo == nil || project.ID <= 0 {
		return fmt.Errorf("postgres repository and project are required to save Scrum create-ticket config")
	}
	validated, err := validateScrumCreateTicketConfig(cfg)
	if err != nil {
		return err
	}
	var settings map[string]any
	if len(project.Settings) > 0 {
		if err := json.Unmarshal(project.Settings, &settings); err != nil {
			return fmt.Errorf("decode project %d settings: %w", project.ID, err)
		}
	}
	if settings == nil {
		settings = map[string]any{}
	}
	settings[scrumCreateTicketKey] = validated
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	settingsJSON := json.RawMessage(raw)
	patch := model.ProjectPatch{Settings: &settingsJSON}
	_, err = s.repo.UpdateProject(ctx, project.ID, patch)
	return err
}
