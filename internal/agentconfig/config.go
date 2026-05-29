package agentconfig

import (
	"encoding/json"
	"os"
	"strings"
)

const (
	SystemOmnidex = "omnidex"
	SystemCursor  = "cursor"
	SystemCodex   = "codex"
)

type Field struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	EnvKeys     []string `json:"env_keys"`
	Options     []string `json:"options,omitempty"`
}

var Fields = []Field{
	{
		Key:         "agent_system",
		Label:       "Execution agent",
		Description: "Which agent executes work: Omnidex (local stack), Cursor SDK, or Codex SDK. Project/card context still applies.",
		EnvKeys:     []string{"OMNI_ARCHITECT_AGENT", "OMNI_AGENT_SYSTEM"},
		Options:     []string{"omnidex", "cursor", "codex"},
	},
	{
		Key:         "agent_strict",
		Label:       "Strict external agent",
		Description: "When using Cursor or Codex, do not fall back to Omnidex if the external agent is unavailable or fails.",
		EnvKeys:     []string{"OMNI_AGENT_STRICT"},
		Options:     []string{"true", "false"},
	},
}

type Config map[string]string

func FromEnv() Config {
	out := Config{}
	for _, field := range Fields {
		if value := lookupEnv(field.EnvKeys); value != "" {
			out[field.Key] = value
		}
	}
	if sys := normalizeSystem(out.Get("agent_system")); sys != "" {
		out["agent_system"] = sys
	}
	return out
}

func FromJSON(raw json.RawMessage) Config {
	out := Config{}
	if len(raw) == 0 {
		return out
	}
	var payload map[string]string
	if err := json.Unmarshal(raw, &payload); err != nil {
		var generic map[string]any
		if err := json.Unmarshal(raw, &generic); err != nil {
			return out
		}
		for key, value := range generic {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				out[key] = strings.TrimSpace(text)
			}
		}
	} else {
		for key, value := range payload {
			if strings.TrimSpace(value) != "" {
				out[key] = strings.TrimSpace(value)
			}
		}
	}
	if _, ok := out["agent_system"]; ok {
		if sys := normalizeSystem(out.Get("agent_system")); sys != "" {
			out["agent_system"] = sys
		}
	}
	return out
}

func FromSettingsJSON(raw json.RawMessage) Config {
	if len(raw) == 0 {
		return Config{}
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(raw, &settings); err != nil {
		return Config{}
	}
	if nested, ok := settings["agent_config"]; ok {
		return FromJSON(nested)
	}
	return Config{}
}

func FromJobMetadata(raw json.RawMessage) Config {
	if len(raw) == 0 {
		return Config{}
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Config{}
	}
	if nested, ok := payload["agent_config"]; ok {
		bytes, err := json.Marshal(nested)
		if err != nil {
			return Config{}
		}
		return FromJSON(bytes)
	}
	return Config{}
}

func Merge(layers ...Config) Config {
	out := Config{}
	for _, layer := range layers {
		for key, value := range layer {
			if strings.TrimSpace(value) != "" {
				out[key] = strings.TrimSpace(value)
			}
		}
	}
	if _, ok := out["agent_system"]; ok {
		if sys := normalizeSystem(out.Get("agent_system")); sys != "" {
			out["agent_system"] = sys
		}
	}
	return out
}

func (c Config) Get(key string) string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c[key])
}

func (c Config) System() string {
	sys := normalizeSystem(c.Get("agent_system"))
	if sys == "" {
		return SystemOmnidex
	}
	return sys
}

func (c Config) IsExternal() bool {
	sys := c.System()
	return sys == SystemCursor || sys == SystemCodex
}

func (c Config) IsStrict() bool {
	return parseBool(c.Get("agent_strict"))
}

func (c Config) ExternalAgentName() string {
	switch c.System() {
	case SystemCursor:
		return "cursor_sdk"
	case SystemCodex:
		return "codex_sdk"
	default:
		return ""
	}
}

func (c Config) ToMap() map[string]string {
	out := map[string]string{}
	for key, value := range c {
		if strings.TrimSpace(value) != "" {
			out[key] = strings.TrimSpace(value)
		}
	}
	if sys := normalizeSystem(out["agent_system"]); sys != "" {
		out["agent_system"] = sys
	} else if _, ok := out["agent_system"]; ok {
		delete(out, "agent_system")
	}
	return out
}

func (c Config) FieldList(envValues map[string]string) []map[string]any {
	if envValues == nil {
		envValues = map[string]string{}
	}
	items := make([]map[string]any, 0, len(Fields))
	for _, field := range Fields {
		value := c.Get(field.Key)
		if value == "" {
			value = lookupMap(envValues, field.EnvKeys)
		}
		if value == "" {
			value = lookupEnv(field.EnvKeys)
		}
		if field.Key == "agent_system" {
			value = normalizeSystem(value)
			if value == "" {
				value = SystemOmnidex
			}
		}
		items = append(items, map[string]any{
			"key":         field.Key,
			"label":       field.Label,
			"description": field.Description,
			"env_keys":    field.EnvKeys,
			"options":     field.Options,
			"value":       value,
		})
	}
	return items
}

func normalizeSystem(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "none", "local", "omnidex", "default":
		return SystemOmnidex
	case "cursor", "cursor_sdk":
		return SystemCursor
	case "codex", "codex_sdk":
		return SystemCodex
	default:
		return value
	}
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "strict":
		return true
	default:
		return false
	}
}

func lookupEnv(keys []string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func lookupMap(values map[string]string, keys []string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values[key]); value != "" {
			return value
		}
	}
	return ""
}
