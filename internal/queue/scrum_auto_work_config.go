package queue

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
)

const ScrumAutoWorkConfigKey = "scrum_auto_work"

type ScrumAutoWorkConfig struct {
	Enabled       bool     `json:"enabled"`
	SourceColumns []string `json:"source_columns"`
}

type exactScrumAutoWorkConfig struct {
	Enabled       *bool    `json:"enabled"`
	SourceColumns []string `json:"source_columns"`
}

func DefaultScrumAutoWorkConfig() ScrumAutoWorkConfig {
	return ScrumAutoWorkConfig{SourceColumns: []string{"assigned"}}
}

func DecodeScrumAutoWorkConfig(settings json.RawMessage) (ScrumAutoWorkConfig, error) {
	config := DefaultScrumAutoWorkConfig()
	if len(bytes.TrimSpace(settings)) == 0 {
		return config, nil
	}
	var root map[string]json.RawMessage
	if err := exactjson.ValidateUniqueObject(settings, "project settings"); err != nil {
		return ScrumAutoWorkConfig{}, err
	}
	if err := json.Unmarshal(settings, &root); err != nil || root == nil {
		if err == nil {
			err = fmt.Errorf("received null")
		}
		return ScrumAutoWorkConfig{}, fmt.Errorf("project settings must be one JSON object: %w", err)
	}
	for _, retired := range []string{"scrum_auto_play_through", "scrum_auto_review"} {
		if _, exists := root[retired]; exists {
			return ScrumAutoWorkConfig{}, fmt.Errorf("retired project setting %q has no compatibility path", retired)
		}
	}
	raw, present := root[ScrumAutoWorkConfigKey]
	if !present {
		return config, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ScrumAutoWorkConfig{}, fmt.Errorf("Scrum auto-work config must be an object")
	}
	if err := exactjson.ValidateObject(raw, exactScrumAutoWorkConfig{}, "stored Scrum auto-work config"); err != nil {
		return ScrumAutoWorkConfig{}, err
	}
	var stored exactScrumAutoWorkConfig
	if err := json.Unmarshal(raw, &stored); err != nil {
		return ScrumAutoWorkConfig{}, fmt.Errorf("decode stored Scrum auto-work config: %w", err)
	}
	if stored.Enabled == nil {
		return ScrumAutoWorkConfig{}, fmt.Errorf("stored Scrum auto-work enabled is required")
	}
	if stored.SourceColumns == nil {
		return ScrumAutoWorkConfig{}, fmt.Errorf("stored Scrum auto-work source_columns is required")
	}
	return ValidateScrumAutoWorkConfig(ScrumAutoWorkConfig{
		Enabled: *stored.Enabled, SourceColumns: stored.SourceColumns,
	})
}

func ValidateScrumAutoWorkConfig(config ScrumAutoWorkConfig) (ScrumAutoWorkConfig, error) {
	if len(config.SourceColumns) == 0 {
		return ScrumAutoWorkConfig{}, fmt.Errorf("Scrum auto-work requires at least one source column")
	}
	seen := make(map[string]struct{}, len(config.SourceColumns))
	columns := make([]string, 0, len(config.SourceColumns))
	for _, raw := range config.SourceColumns {
		column, err := ParseScrumCardColumn(raw)
		if err != nil {
			return ScrumAutoWorkConfig{}, err
		}
		switch column {
		case ScrumCardBacklog, ScrumCardReady, ScrumCardAssigned, ScrumCardInProgress, ScrumCardBlocked:
		default:
			return ScrumAutoWorkConfig{}, fmt.Errorf("Scrum auto-work source column %q is not registered", raw)
		}
		if _, duplicate := seen[raw]; duplicate {
			return ScrumAutoWorkConfig{}, fmt.Errorf("duplicate Scrum auto-work source column %q", raw)
		}
		seen[raw] = struct{}{}
		columns = append(columns, raw)
	}
	config.SourceColumns = columns
	return config, nil
}

func encodeScrumAutoWorkSettings(settings json.RawMessage, config ScrumAutoWorkConfig) (json.RawMessage, error) {
	if err := validateProjectSettings(settings); err != nil {
		return nil, fmt.Errorf("validate current project settings before Scrum auto-work mutation: %w", err)
	}
	validated, err := ValidateScrumAutoWorkConfig(config)
	if err != nil {
		return nil, err
	}
	if _, err := DecodeScrumAutoWorkConfig(settings); err != nil {
		return nil, err
	}
	root := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(settings)) > 0 {
		if err := json.Unmarshal(settings, &root); err != nil {
			return nil, err
		}
	}
	encoded, err := json.Marshal(validated)
	if err != nil {
		return nil, err
	}
	root[ScrumAutoWorkConfigKey] = encoded
	reencoded, err := json.Marshal(root)
	if err != nil {
		return nil, err
	}
	if err := validateProjectSettings(reencoded); err != nil {
		return nil, fmt.Errorf("validate re-encoded project settings after Scrum auto-work mutation: %w", err)
	}
	return reencoded, nil
}
