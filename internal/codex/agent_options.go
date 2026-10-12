package codex

import "github.com/gryph/omnidex/internal/codingagent"

// Empty effort uses the selected model's own default. Standard speed is explicit
// so a global Codex Fast preference cannot override a frozen per-role choice.
func applyAgentInferenceOptions(parameters map[string]any, route codingagent.Route) {
	if route.Effort != "" {
		parameters["effort"] = route.Effort
	}
	parameters["serviceTier"] = "default"
	if route.ServiceTier != "" {
		parameters["serviceTier"] = route.ServiceTier
	}
}
