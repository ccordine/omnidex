package api

import (
	"testing"

	"github.com/gryph/omnidex/internal/codex"
	"github.com/gryph/omnidex/internal/codingagent"
)

func TestCodingAgentOptionsUseActualCatalogAndIndependentRoles(t *testing.T) {
	models := []codex.Model{{ID: "model-a", Efforts: []codex.EffortOption{{Value: "ultra"}, {Value: "low"}}, ServiceTiers: []codex.ServiceTier{{ID: "priority"}}}, {ID: "model-b", Efforts: []codex.EffortOption{{Value: "max"}}}}
	configuration := codingagent.Configuration{Mode: codingagent.Agents,
		Planner:      codingagent.Route{Provider: "codex", Model: "model-a", Effort: "ultra", ServiceTier: "priority"},
		LedgerParser: codingagent.Route{Provider: "codex", Model: "model-a", Effort: "low"},
		Executor:     codingagent.Route{Provider: "codex", Model: "model-b", Effort: "max", ServiceTier: "default"},
		Reviewer:     codingagent.Route{Provider: "codex", Model: "model-a", Effort: "low"}}
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

func TestLocalParserOptionsDoNotGrantNativeRolesOrCodexOptions(t *testing.T) {
	native := codingagent.Route{Provider: "codex", Model: "native"}
	configuration := codingagent.Configuration{Mode: codingagent.Agents, Planner: native, Executor: native, Reviewer: native,
		LedgerParser: codingagent.Route{Provider: "ollama", Model: "installed-local:7b"}}
	models := []codex.Model{{ID: "native"}}
	if err := validateCodingAgentModelOptions(configuration, models); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"planner", "executor", "reviewer", "effort", "speed"} {
		invalid := configuration
		switch role {
		case "planner":
			invalid.Planner = configuration.LedgerParser
		case "executor":
			invalid.Executor = configuration.LedgerParser
		case "reviewer":
			invalid.Reviewer = configuration.LedgerParser
		case "effort":
			invalid.LedgerParser.Effort = "low"
		case "speed":
			invalid.LedgerParser.ServiceTier = "default"
		}
		if err := validateCodingAgentModelOptions(invalid, models); err == nil {
			t.Fatalf("local parser acquired unsupported %s authority", role)
		}
	}
}
