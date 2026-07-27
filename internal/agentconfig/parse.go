package agentconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

func FromEnv() (Config, error) {
	out := Config{}
	for _, field := range Fields {
		if value := lookupEnv(field.EnvKeys); value != "" {
			out[field.Key] = value
		}
	}
	if err := Validate(out); err != nil {
		return nil, fmt.Errorf("invalid agent environment configuration: %w", err)
	}
	return out, nil
}

func FromEnvFileValues(values map[string]string) (Config, error) {
	out := Config{}
	for _, field := range Fields {
		if value := lookupMap(values, field.EnvKeys); value != "" {
			out[field.Key] = value
		}
	}
	if err := Validate(out); err != nil {
		return nil, fmt.Errorf("invalid agent environment file configuration: %w", err)
	}
	return out, nil
}

func FromStringMap(values map[string]string) (Config, error) {
	out := Config{}
	for key, value := range values {
		out[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := Validate(out); err != nil {
		return nil, err
	}
	return out, nil
}

func FromJSON(raw json.RawMessage) (Config, error) {
	if len(raw) == 0 {
		return Config{}, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("agent_config must be a JSON object: %w", err)
	}
	if payload == nil {
		return nil, fmt.Errorf("agent_config must be a JSON object, received null")
	}
	out := Config{}
	for key, encoded := range payload {
		var value string
		if err := json.Unmarshal(encoded, &value); err != nil {
			return nil, fmt.Errorf("agent_config.%s must be a string: %w", key, err)
		}
		out[key] = strings.TrimSpace(value)
	}
	if err := Validate(out); err != nil {
		return nil, err
	}
	return out, nil
}

func FromSettingsJSON(raw json.RawMessage) (Config, error) {
	if len(raw) == 0 {
		return Config{}, nil
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(raw, &settings); err != nil {
		return nil, fmt.Errorf("project settings must be a JSON object: %w", err)
	}
	if settings == nil {
		return nil, fmt.Errorf("project settings must be a JSON object, received null")
	}
	nested, ok := settings["agent_config"]
	if !ok {
		return Config{}, nil
	}
	cfg, err := FromJSON(nested)
	if err != nil {
		return nil, fmt.Errorf("project settings: %w", err)
	}
	return cfg, nil
}

func FromJobMetadata(raw json.RawMessage) (Config, error) {
	if len(raw) == 0 {
		return Config{"agent_system": SystemOmnidex}, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("job metadata must be a JSON object: %w", err)
	}
	if payload == nil {
		return nil, fmt.Errorf("job metadata must be a JSON object, received null")
	}
	for _, removed := range []string{"execution_agent", "agent_strict"} {
		if _, ok := payload[removed]; ok {
			return nil, fmt.Errorf("job metadata field %q was removed; use agent_config.agent_system", removed)
		}
	}
	nested, ok := payload["agent_config"]
	if !ok {
		return Config{"agent_system": SystemOmnidex}, nil
	}
	cfg, err := FromJSON(nested)
	if err != nil {
		return nil, fmt.Errorf("job metadata: %w", err)
	}
	if cfg.Get("agent_system") == "" {
		return nil, fmt.Errorf("job metadata agent_config.agent_system is required")
	}
	return cfg, nil
}

func Validate(cfg Config) error {
	known := make(map[string]Field, len(Fields))
	for _, field := range Fields {
		known[field.Key] = field
	}
	keys := make([]string, 0, len(cfg))
	for key := range cfg {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		field, ok := known[key]
		if !ok {
			return fmt.Errorf("unknown agent_config field %q", key)
		}
		value := cfg[key]
		if value != strings.TrimSpace(value) || value == "" {
			return fmt.Errorf("agent_config.%s must be a non-empty trimmed string", key)
		}
		if len(field.Options) > 0 && !contains(field.Options, value) {
			return fmt.Errorf("agent_config.%s must be one of %s, received %q", key, strings.Join(field.Options, ", "), value)
		}
	}
	return nil
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func lookupEnv(keys []string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
