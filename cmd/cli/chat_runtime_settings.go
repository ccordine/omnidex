package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/agentconfig"
)

func setActiveChatModel(metadata map[string]any, agentCfg *cliAgentRuntimeConfig, model string) (string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", fmt.Errorf("model is required")
	}
	if agentCfg != nil {
		switch agentCfg.AgentSystemOverride() {
		case agentconfig.SystemCursor, agentconfig.SystemCodex:
			if err := agentCfg.SetActiveAgentModel(model); err != nil {
				return "", err
			}
			return "external agent model set to " + model, nil
		case agentconfig.SystemOmnidex:
			setNativeOmnidexModel(metadata, model)
			return "Omnidex plan/analyze/response/verify models set to " + model, nil
		}
	}
	return "", fmt.Errorf("active agent is inherited from core defaults; run /agent omnidex, /agent cursor, or /agent codex before /model")
}

func setNativeOmnidexModel(metadata map[string]any, model string) {
	if metadata == nil {
		return
	}
	for _, key := range []string{"model_plan", "model_analyze", "model_response", "model_verify"} {
		metadata[key] = strings.TrimSpace(model)
	}
}

func setChatMetadataOverride(metadata map[string]any, key, value string) (bool, error) {
	if metadata == nil {
		return false, fmt.Errorf("metadata map is nil")
	}
	key = normalizeRuntimeConfigKey(key)
	value = strings.TrimSpace(value)
	switch key {
	case "reasoning", "reasoning_level":
		normalized, err := normalizeOneOf("reasoning_level", value, []string{"auto", "fast", "deep"})
		if err != nil {
			return true, err
		}
		metadata["reasoning_level"] = normalized
	case "web", "web_search":
		normalized, err := normalizeModeValue("web_search", value, map[string]string{
			"auto": "auto", "on": "force", "force": "force", "off": "off",
		})
		if err != nil {
			return true, err
		}
		metadata["web_search"] = normalized
	case "workspace", "workspace_scan":
		normalized, err := normalizeModeValue("workspace_scan", value, map[string]string{
			"auto": "auto", "on": "on", "force": "on", "off": "off",
		})
		if err != nil {
			return true, err
		}
		metadata["workspace_scan"] = normalized
	case "autonomy", "autonomy_mode":
		normalized, err := normalizeModeValue("autonomy_mode", value, map[string]string{
			"auto": "auto", "on": "on", "true": "on", "enabled": "on", "off": "off", "false": "off", "disabled": "off", "strict": "off",
		})
		if err != nil {
			return true, err
		}
		metadata["autonomy_mode"] = normalized
	case "approval", "approval_mode":
		normalized, err := normalizeModeValue("approval_mode", value, map[string]string{
			"auto": "auto", "on": "force", "force": "force", "off": "off",
		})
		if err != nil {
			return true, err
		}
		metadata["approval_mode"] = normalized
	case "verify", "verification", "verification_mode":
		normalized, err := normalizeModeValue("verification_mode", value, map[string]string{
			"auto": "auto", "on": "force", "force": "force", "off": "off",
		})
		if err != nil {
			return true, err
		}
		metadata["verification_mode"] = normalized
	case "verify_iterations", "verification_iterations":
		count, err := strconv.Atoi(value)
		if err != nil || count < 1 || count > 4 {
			return true, fmt.Errorf("verification_iterations must be 1-4")
		}
		metadata["verification_iterations"] = count
	case "model_plan", "model_analyze", "model_response", "model_search", "model_tagger", "model_verify", "model_memory":
		if value == "" {
			return true, fmt.Errorf("%s requires a non-empty model", key)
		}
		metadata[key] = value
	default:
		return false, nil
	}
	return true, nil
}

func chatRuntimeSettingsSummary(metadata map[string]any, agentCfg *cliAgentRuntimeConfig) string {
	lines := []string{}
	if agentCfg != nil {
		lines = append(lines, agentCfg.Summary())
	}
	for _, key := range []string{
		"reasoning_level",
		"web_search",
		"workspace_scan",
		"autonomy_mode",
		"approval_mode",
		"verification_mode",
		"verification_iterations",
		"model_plan",
		"model_analyze",
		"model_response",
		"model_search",
		"model_tagger",
		"model_verify",
		"model_memory",
	} {
		if value, ok := metadata[key]; ok {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" && text != "<nil>" {
				lines = append(lines, key+"="+text)
			}
		}
	}
	if len(lines) == 0 {
		return "settings: core defaults"
	}
	return strings.Join(lines, "\n")
}

func normalizeModeValue(key, value string, allowed map[string]string) (string, error) {
	clean := strings.ToLower(strings.TrimSpace(value))
	if normalized, ok := allowed[clean]; ok {
		return normalized, nil
	}
	keys := make([]string, 0, len(allowed))
	for key := range allowed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return "", fmt.Errorf("%s must be one of: %s", key, strings.Join(keys, "|"))
}
