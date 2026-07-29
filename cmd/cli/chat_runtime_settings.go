package main

import (
	"fmt"
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
