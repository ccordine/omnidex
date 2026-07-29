package modelconfig

import (
	"fmt"
	"strings"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// AnalyzerModel resolves the configured analysis route and rejects an empty route.
func AnalyzerModel(cfg Config, runtimeDefault string) (string, error) {
	model := firstNonEmpty(
		cfg.Get("analyzer_model"),
		cfg.Get("reasoning_model"),
		cfg.Get("planner_model"),
		cfg.Get("default_model"),
		runtimeDefault,
	)
	if model == "" {
		return "", fmt.Errorf("analyzer model is not configured")
	}
	return model, nil
}

// PlannerTicketModel resolves the configured planning route and rejects an empty route.
func PlannerTicketModel(cfg Config, runtimeDefault string) (string, error) {
	model := firstNonEmpty(
		cfg.Get("planner_model"),
		cfg.Get("reasoning_model"),
		cfg.Get("default_model"),
		runtimeDefault,
	)
	if model == "" {
		return "", fmt.Errorf("planner ticket model is not configured")
	}
	return model, nil
}
