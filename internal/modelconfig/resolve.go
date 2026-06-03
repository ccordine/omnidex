package modelconfig

import "strings"

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// AnalyzerModel returns the configured analysis model with sensible fallbacks.
func AnalyzerModel(cfg Config, fallbacks ...string) string {
	return firstNonEmpty(append([]string{
		cfg.Get("analyzer_model"),
		cfg.Get("reasoning_model"),
		cfg.Get("planner_model"),
		cfg.Get("default_model"),
	}, fallbacks...)...)
}

// PlannerTicketModel returns the model used for scrum card planning tickets.
func PlannerTicketModel(cfg Config, fallbacks ...string) string {
	return firstNonEmpty(append([]string{
		cfg.Get("planner_model"),
		cfg.Get("reasoning_model"),
		cfg.Get("thinking_model"),
		cfg.Get("default_model"),
	}, fallbacks...)...)
}
