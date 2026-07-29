package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

type ScrumCoachConfig struct {
	Enabled  bool   `json:"enabled"`
	AutoScan bool   `json:"auto_scan"`
	Model    string `json:"model"`
}

type ScrumCoachSuggestion struct {
	Level string `json:"level"`
	Text  string `json:"text"`
}

type ScrumCoachMemoryNote struct {
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

type ScrumCoachLLMResponse struct {
	Reply       string                 `json:"reply"`
	Suggestions []ScrumCoachSuggestion `json:"suggestions"`
	CardTags    []string               `json:"card_tags"`
	ProjectTags []string               `json:"project_tags"`
	CardPrompt  string                 `json:"card_prompt"`
	MemoryNotes []ScrumCoachMemoryNote `json:"memory_notes"`
}

func defaultScrumCoachConfig(modelName string) ScrumCoachConfig {
	return ScrumCoachConfig{
		Enabled:  true,
		AutoScan: false,
		Model:    strings.TrimSpace(modelName),
	}
}

func parseScrumCoachConfig(raw json.RawMessage, inheritedModel string) (ScrumCoachConfig, error) {
	cfg := defaultScrumCoachConfig(inheritedModel)
	if len(bytes.TrimSpace(raw)) == 0 {
		return cfg, nil
	}
	var stored struct {
		Enabled  *bool   `json:"enabled"`
		AutoScan *bool   `json:"auto_scan"`
		Model    *string `json:"model"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&stored); err != nil {
		return ScrumCoachConfig{}, fmt.Errorf("parse Scrum coach config: %w", err)
	}
	if err := requireJSONEOF(decoder, "Scrum coach config"); err != nil {
		return ScrumCoachConfig{}, err
	}
	if stored.Enabled != nil {
		cfg.Enabled = *stored.Enabled
	}
	if stored.AutoScan != nil {
		cfg.AutoScan = *stored.AutoScan
	}
	if stored.Model != nil {
		cfg.Model = strings.TrimSpace(*stored.Model)
	}
	return validateScrumCoachConfig(cfg)
}

func (s *Server) scrumCoachConfig(raw json.RawMessage) (ScrumCoachConfig, error) {
	cfg, err := parseScrumCoachConfig(raw, s.configuredDefaultLLMModel())
	if err != nil {
		return ScrumCoachConfig{}, fmt.Errorf("resolve Scrum coach config: %w", err)
	}
	return cfg, nil
}

func coachConfigToRaw(cfg ScrumCoachConfig) (json.RawMessage, error) {
	cfg, err := validateScrumCoachConfig(cfg)
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("encode Scrum coach config: %w", err)
	}
	return out, nil
}

func validateScrumCoachConfig(cfg ScrumCoachConfig) (ScrumCoachConfig, error) {
	cfg.Model = strings.TrimSpace(cfg.Model)
	if cfg.Model == "" {
		return ScrumCoachConfig{}, fmt.Errorf("Scrum coach model is required")
	}
	return cfg, nil
}

func parseCoachLLMResponse(raw string) (ScrumCoachLLMResponse, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ScrumCoachLLMResponse{}, fmt.Errorf("Scrum coach returned an empty response")
	}
	var out ScrumCoachLLMResponse
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&out); err != nil {
		return ScrumCoachLLMResponse{}, fmt.Errorf("decode Scrum coach response: %w", err)
	}
	if err := requireJSONEOF(decoder, "Scrum coach response"); err != nil {
		return ScrumCoachLLMResponse{}, err
	}
	out.Reply = strings.TrimSpace(out.Reply)
	out.CardPrompt = strings.TrimSpace(out.CardPrompt)
	out.CardTags = mergeTags(nil, out.CardTags)
	out.ProjectTags = mergeTags(nil, out.ProjectTags)
	for i := range out.Suggestions {
		out.Suggestions[i].Level = strings.ToLower(strings.TrimSpace(out.Suggestions[i].Level))
		out.Suggestions[i].Text = strings.TrimSpace(out.Suggestions[i].Text)
		switch out.Suggestions[i].Level {
		case "info", "warn", "tip":
		default:
			return ScrumCoachLLMResponse{}, fmt.Errorf("Scrum coach suggestion %d has unsupported level %q", i, out.Suggestions[i].Level)
		}
		if out.Suggestions[i].Text == "" {
			return ScrumCoachLLMResponse{}, fmt.Errorf("Scrum coach suggestion %d text is required", i)
		}
	}
	for i := range out.MemoryNotes {
		out.MemoryNotes[i].Content = strings.TrimSpace(out.MemoryNotes[i].Content)
		if out.MemoryNotes[i].Content == "" {
			return ScrumCoachLLMResponse{}, fmt.Errorf("Scrum coach memory note %d content is required", i)
		}
		out.MemoryNotes[i].Tags = mergeTags(nil, out.MemoryNotes[i].Tags)
	}
	if out.Reply == "" && len(out.Suggestions) == 0 && len(out.CardTags) == 0 &&
		len(out.ProjectTags) == 0 && out.CardPrompt == "" && len(out.MemoryNotes) == 0 {
		return ScrumCoachLLMResponse{}, fmt.Errorf("Scrum coach response contains no usable content")
	}
	return out, nil
}

func requireJSONEOF(decoder *json.Decoder, label string) error {
	var trailing any
	if err := decoder.Decode(&trailing); !isJSONEOF(err) {
		if err == nil {
			return fmt.Errorf("%s contains trailing JSON", label)
		}
		return fmt.Errorf("%s trailing data: %w", label, err)
	}
	return nil
}

func isJSONEOF(err error) bool {
	return err == io.EOF
}

func normalizeCoachMode(mode string) (string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" || mode == "chat" {
		return "chat", nil
	}
	switch mode {
	case "scan", "plan", "research", "card-ticket", "config":
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported Scrum coach mode %q", mode)
	}
}

func (s *Server) mergeProjectTags(ctx context.Context, projectID int64, tags []string) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("project tags require a PostgreSQL repository")
	}
	if ctx == nil {
		return fmt.Errorf("project tags require a context")
	}
	if projectID <= 0 {
		return fmt.Errorf("project tags require a positive project ID")
	}
	if len(tags) == 0 {
		return nil
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	settings := map[string]json.RawMessage{}
	if len(project.Settings) > 0 {
		if err := json.Unmarshal(project.Settings, &settings); err != nil {
			return fmt.Errorf("parse project settings: %w", err)
		}
	}
	existing := []string{}
	if raw, ok := settings["tags"]; ok {
		if err := json.Unmarshal(raw, &existing); err != nil {
			return fmt.Errorf("project settings tags must be a string array: %w", err)
		}
	}
	merged, err := json.Marshal(mergeTags(existing, tags))
	if err != nil {
		return err
	}
	settings["tags"] = merged
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	settingsJSON := json.RawMessage(raw)
	_, err = s.repo.UpdateProject(ctx, projectID, model.ProjectPatch{Settings: &settingsJSON})
	return err
}
