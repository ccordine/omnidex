package worker

import (
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func contextsToMap(contexts []model.StepContext) map[string]string {
	sorted := append([]model.StepContext(nil), contexts...)
	sort.Slice(sorted, func(left, right int) bool {
		return sorted[left].ID < sorted[right].ID
	})
	out := make(map[string]string, len(sorted))
	for _, contextValue := range sorted {
		out[contextValue.Key] = contextValue.Value
	}
	return out
}

func collectContextValuesByKey(contexts []model.StepContext, keys ...string) []string {
	acceptedKeys := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key = strings.TrimSpace(key); key != "" {
			acceptedKeys[key] = struct{}{}
		}
	}
	seenValues := map[string]struct{}{}
	values := make([]string, 0, len(contexts))
	for _, contextValue := range contexts {
		if _, accepted := acceptedKeys[contextValue.Key]; !accepted {
			continue
		}
		value := strings.TrimSpace(contextValue.Value)
		if value == "" {
			continue
		}
		if _, duplicate := seenValues[value]; duplicate {
			continue
		}
		seenValues[value] = struct{}{}
		values = append(values, value)
	}
	return values
}

func pipelinePhaseForAction(action string) string {
	switch strings.TrimPrefix(strings.ToLower(strings.TrimSpace(action)), "v3_") {
	case "intent_parse", "capability_audit", "planning", "workspace_research", "memory_retrieval", "external_research":
		return "planning"
	case "verification", "memory_review", "finalize":
		return "review"
	default:
		return "execution"
	}
}
