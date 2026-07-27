package agentconfig

import (
	"strings"
)

const (
	SystemOmnidex = "omnidex"
	SystemCursor  = "cursor"
	SystemCodex   = "codex"
)

// WorkspaceSettingsKey is the workspace_settings row for global agent defaults (DB overrides env file).
const WorkspaceSettingsKey = "workspace_agent_config"

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
		Key:         "cursor_model",
		Label:       "Cursor model",
		Description: "Model passed to the Cursor SDK thread when Cursor executes the card.",
		EnvKeys:     []string{"OMNI_CURSOR_MODEL"},
	},
	{
		Key:         "codex_model",
		Label:       "Codex model",
		Description: "Model passed to the Codex SDK thread when Codex executes the card.",
		EnvKeys:     []string{"OMNI_CODEX_MODEL"},
	},
	{
		Key:         "codex_reasoning_effort",
		Label:       "Codex reasoning effort",
		Description: "Codex SDK modelReasoningEffort. Use minimal/low for fast mode, higher values for deeper coding passes.",
		EnvKeys:     []string{"OMNI_CODEX_REASONING_EFFORT", "OMNI_CODEX_MODEL_REASONING_EFFORT"},
		Options:     []string{"minimal", "low", "medium", "high", "xhigh"},
	},
	{
		Key:         "codex_sandbox_mode",
		Label:       "Codex sandbox",
		Description: "Codex SDK sandboxMode for filesystem access.",
		EnvKeys:     []string{"OMNI_CODEX_SANDBOX_MODE"},
		Options:     []string{"read-only", "workspace-write", "danger-full-access"},
	},
	{
		Key:         "codex_approval_policy",
		Label:       "Codex approval policy",
		Description: "Codex SDK approvalPolicy for tool and command approval.",
		EnvKeys:     []string{"OMNI_CODEX_APPROVAL_POLICY"},
		Options:     []string{"never", "on-request", "on-failure", "untrusted"},
	},
	{
		Key:         "codex_network_access",
		Label:       "Codex network access",
		Description: "Codex SDK networkAccessEnabled for the thread.",
		EnvKeys:     []string{"OMNI_CODEX_NETWORK_ACCESS"},
		Options:     []string{"true", "false"},
	},
	{
		Key:         "codex_web_search_mode",
		Label:       "Codex web search",
		Description: "Codex SDK webSearchMode for web-search behavior.",
		EnvKeys:     []string{"OMNI_CODEX_WEB_SEARCH_MODE"},
		Options:     []string{"disabled", "cached", "live"},
	},
}

type Config map[string]string

func Merge(layers ...Config) Config {
	out := Config{}
	for _, layer := range layers {
		for key, value := range layer {
			if strings.TrimSpace(value) != "" {
				out[key] = strings.TrimSpace(value)
			}
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
	sys := c.Get("agent_system")
	if sys == "" {
		return SystemOmnidex
	}
	return sys
}

func (c Config) IsExternal() bool {
	sys := c.System()
	return sys == SystemCursor || sys == SystemCodex
}

func (c Config) CodexModel() string {
	return c.Get("codex_model")
}

func (c Config) CursorModel() string {
	return c.Get("cursor_model")
}

func (c Config) CodexReasoningEffort() string {
	return c.Get("codex_reasoning_effort")
}

func (c Config) CodexSandboxMode() string {
	return c.Get("codex_sandbox_mode")
}

func (c Config) CodexApprovalPolicy() string {
	return c.Get("codex_approval_policy")
}

func (c Config) CodexNetworkAccess() string {
	return c.Get("codex_network_access")
}

func (c Config) CodexWebSearchMode() string {
	return c.Get("codex_web_search_mode")
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

func lookupMap(values map[string]string, keys []string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values[key]); value != "" {
			return value
		}
	}
	return ""
}
