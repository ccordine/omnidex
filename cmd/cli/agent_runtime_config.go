package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/agentconfig"
)

type cliAgentRuntimeConfig struct {
	values map[string]string
}

func newCLIAgentRuntimeConfig() *cliAgentRuntimeConfig {
	return &cliAgentRuntimeConfig{values: map[string]string{}}
}

func (c *cliAgentRuntimeConfig) Set(key, value string) error {
	if c == nil {
		return fmt.Errorf("agent runtime config is nil")
	}
	key = normalizeRuntimeConfigKey(key)
	value = strings.TrimSpace(value)
	if key == "" {
		return fmt.Errorf("agent setting key is required")
	}
	if isResetRuntimeValue(value) {
		delete(c.values, key)
		return nil
	}
	switch key {
	case "agent_system":
		system, err := normalizeCLIAgentSystem(value)
		if err != nil {
			return err
		}
		c.values[key] = system
	case "agent_model":
		return c.SetActiveAgentModel(value)
	case "agent_strict":
		normalized, err := normalizeBoolString(value)
		if err != nil {
			return fmt.Errorf("agent_strict must be true or false")
		}
		c.values[key] = normalized
	case "cursor_model", "codex_model":
		if value == "" {
			return fmt.Errorf("%s requires a non-empty model", key)
		}
		c.values[key] = value
	case "codex_reasoning_effort":
		normalized, err := normalizeOneOf(key, value, []string{"minimal", "low", "medium", "high", "xhigh"})
		if err != nil {
			return err
		}
		c.values[key] = normalized
	case "codex_sandbox_mode":
		normalized, err := normalizeOneOf(key, value, []string{"read-only", "workspace-write", "danger-full-access"})
		if err != nil {
			return err
		}
		c.values[key] = normalized
	case "codex_approval_policy":
		normalized, err := normalizeOneOf(key, value, []string{"never", "on-request", "on-failure", "untrusted"})
		if err != nil {
			return err
		}
		c.values[key] = normalized
	case "codex_network_access":
		normalized, err := normalizeBoolString(value)
		if err != nil {
			return fmt.Errorf("codex_network_access must be true or false")
		}
		c.values[key] = normalized
	case "codex_web_search_mode":
		normalized, err := normalizeOneOf(key, value, []string{"disabled", "cached", "live"})
		if err != nil {
			return err
		}
		c.values[key] = normalized
	default:
		return fmt.Errorf("unknown agent setting %q", key)
	}
	return nil
}

func (c *cliAgentRuntimeConfig) Clear() {
	if c == nil {
		return
	}
	c.values = map[string]string{}
}

func (c *cliAgentRuntimeConfig) SetActiveAgentModel(model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("agent model requires a non-empty model")
	}
	switch c.AgentSystemOverride() {
	case agentconfig.SystemCursor:
		c.values["cursor_model"] = model
	case agentconfig.SystemCodex:
		c.values["codex_model"] = model
	case "":
		return fmt.Errorf("active agent is inherited from core defaults; run /agent omnidex, /agent cursor, or /agent codex before /model")
	case agentconfig.SystemOmnidex:
		return fmt.Errorf("Omnidex model routing is role-based; use /model for native model routing or /set model_response <model>")
	default:
		return fmt.Errorf("unsupported active agent %q", c.AgentSystemOverride())
	}
	return nil
}

func (c *cliAgentRuntimeConfig) AgentSystemOverride() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.values["agent_system"])
}

func (c *cliAgentRuntimeConfig) ToMap() map[string]string {
	out := map[string]string{}
	if c == nil {
		return out
	}
	for key, value := range c.values {
		if strings.TrimSpace(value) != "" {
			out[key] = strings.TrimSpace(value)
		}
	}
	return out
}

func (c *cliAgentRuntimeConfig) ApplyToMetadata(metadata map[string]any) {
	if metadata == nil || c == nil {
		return
	}
	values := c.ToMap()
	if len(values) == 0 {
		delete(metadata, "instance_agent_config")
		return
	}
	metadata["instance_agent_config"] = values
}

func (c *cliAgentRuntimeConfig) Summary() string {
	values := c.ToMap()
	if len(values) == 0 {
		return "agent override: core default"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values[key])
	}
	return "agent override: " + strings.Join(parts, ", ")
}

func normalizeRuntimeConfigKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, "-", "_")
	switch key {
	case "agent":
		return "agent_system"
	case "cursor":
		return "cursor_model"
	case "codex":
		return "codex_model"
	case "codex_reasoning":
		return "codex_reasoning_effort"
	case "codex_sandbox":
		return "codex_sandbox_mode"
	case "codex_approval":
		return "codex_approval_policy"
	case "codex_network":
		return "codex_network_access"
	case "codex_web_search":
		return "codex_web_search_mode"
	}
	return key
}

func normalizeCLIAgentSystem(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("agent_system is required")
	}
	system := agentconfig.Config{"agent_system": value}.System()
	switch system {
	case agentconfig.SystemOmnidex, agentconfig.SystemCursor, agentconfig.SystemCodex:
		return system, nil
	default:
		return "", fmt.Errorf("agent_system must be omnidex, cursor, or codex")
	}
}

func normalizeOneOf(key, value string, allowed []string) (string, error) {
	clean := strings.ToLower(strings.TrimSpace(value))
	for _, item := range allowed {
		if clean == item {
			return clean, nil
		}
	}
	return "", fmt.Errorf("%s must be one of: %s", key, strings.Join(allowed, "|"))
}

func normalizeBoolString(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "enabled":
		return "true", nil
	case "0", "false", "no", "off", "disabled":
		return "false", nil
	default:
		return "", fmt.Errorf("expected boolean")
	}
}

func isResetRuntimeValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "reset", "clear", "unset":
		return true
	default:
		return false
	}
}
