package api

import (
	"testing"

	"github.com/gryph/omnidex/internal/codex"
	"github.com/gryph/omnidex/internal/codingagent"
)

func TestCodingAgentOptionsUseActualCatalogAndIndependentRoles(t *testing.T) {
	models := []codex.Model{{ID: "model-a", Efforts: []codex.EffortOption{{Value: "ultra"}, {Value: "low"}}, ServiceTiers: []codex.ServiceTier{{ID: "priority"}}}, {ID: "model-b", Efforts: []codex.EffortOption{{Value: "max"}}}}
	configuration := codingagent.Configuration{Mode: codingagent.Agents,
		Planner:  codingagent.Route{Provider: "codex", Model: "model-a", Effort: "ultra", ServiceTier: "priority"},
		Executor: codingagent.Route{Provider: "codex", Model: "model-b", Effort: "max", ServiceTier: "default"},
		Reviewer: codingagent.Route{Provider: "codex", Model: "model-a", Effort: "low"}}
	if err := validateCodingAgentModelOptions(configuration, models); err != nil {
		t.Fatal(err)
	}
	for _, route := range []codingagent.Route{{Provider: "codex", Model: "unknown"}, {Provider: "codex", Model: "model-b", Effort: "ultra"}, {Provider: "codex", Model: "model-b", ServiceTier: "priority"}, {Provider: "other", Model: "model-a"}} {
		invalid := configuration
		invalid.Executor = route
		if err := validateCodingAgentModelOptions(invalid, models); err == nil {
			t.Fatalf("unsupported role configuration accepted: %+v", route)
		}
	}
}
