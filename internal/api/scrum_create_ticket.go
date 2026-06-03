package api

import (
	"context"
	"encoding/json"
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

func normalizeScrumCreateTicketColumn(raw string) string {
	switch normalizeScrumColumn(strings.TrimSpace(raw)) {
	case "backlog", "ready", "assigned":
		return normalizeScrumColumn(strings.TrimSpace(raw))
	default:
		return "backlog"
	}
}

func loadScrumCreateTicketConfig(settings json.RawMessage) ScrumCreateTicketConfig {
	cfg := defaultScrumCreateTicketConfig()
	if len(settings) == 0 {
		return cfg
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(settings, &payload); err != nil {
		return cfg
	}
	raw, ok := payload[scrumCreateTicketKey]
	if !ok || len(raw) == 0 {
		return cfg
	}
	_ = json.Unmarshal(raw, &cfg)
	cfg.Column = normalizeScrumCreateTicketColumn(cfg.Column)
	return cfg
}

func (s *Server) scrumCreateTicketConfig(ctx context.Context, projectID int64) ScrumCreateTicketConfig {
	if s.repo == nil || projectID <= 0 {
		return defaultScrumCreateTicketConfig()
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return defaultScrumCreateTicketConfig()
	}
	return loadScrumCreateTicketConfig(project.Settings)
}

func (s *Server) saveScrumCreateTicketConfig(ctx context.Context, project model.Project, cfg ScrumCreateTicketConfig) error {
	var settings map[string]any
	if len(project.Settings) > 0 {
		_ = json.Unmarshal(project.Settings, &settings)
	}
	if settings == nil {
		settings = map[string]any{}
	}
	cfg.Column = normalizeScrumCreateTicketColumn(cfg.Column)
	settings[scrumCreateTicketKey] = cfg
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	settingsJSON := json.RawMessage(raw)
	patch := model.ProjectPatch{Settings: &settingsJSON}
	_, err = s.repo.UpdateProject(ctx, project.ID, patch)
	return err
}
