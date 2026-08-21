package config

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/station"
)

func TestLoadContextSieveStationModelsFromExactEnvironmentKeys(t *testing.T) {
	t.Setenv("OMNI_CONTEXT_SEARCH_TERMS_MODEL", "terms-model")
	t.Setenv("OMNI_CONTEXT_RELEVANCE_MODEL", "relevance-model")
	t.Setenv("OMNI_CONTEXT_MINIFICATION_MODEL", "minification-model")

	models := loadStationModels(Config{})
	wants := map[station.ID]string{
		station.ContextSearchTerms:  "terms-model",
		station.ContextRelevance:    "relevance-model",
		station.ContextMinification: "minification-model",
	}
	for id, want := range wants {
		if got := models[id]; got != want {
			t.Fatalf("station %s model=%q want %q", id, got, want)
		}
	}
	t.Setenv("OMNI_CONVERSATION_CONTEXT_SELECTION_MODEL", "retired-model")
	t.Setenv("OMNI_MEMORY_CONTEXT_SELECTION_MODEL", "retired-model")
	t.Setenv("OMNI_ROLEPLAY_NARRATIVE_CONTINUITY_MODEL", "retired-model")
	models = loadStationModels(Config{})
	for id := range models {
		if id == station.ID("conversation_context_selection") ||
			id == station.ID("memory_context_selection") ||
			id == station.ID("roleplay_narrative_continuity") {
			t.Fatalf("retired context selector %q still has runtime model routing", id)
		}
	}
}

func TestLoadContextRelevanceProviderIsExplicitAndValidated(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("OMNI_CONTEXT_RELEVANCE_PROVIDER", ContextRelevanceProviderBrowserWebGPU)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ContextRelevanceProvider != ContextRelevanceProviderBrowserWebGPU {
		t.Fatalf("provider=%q", cfg.ContextRelevanceProvider)
	}

	t.Setenv("OMNI_CONTEXT_RELEVANCE_PROVIDER", "automatic")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "OMNI_CONTEXT_RELEVANCE_PROVIDER") {
		t.Fatalf("error=%v", err)
	}
}

func TestRetiredContextStationModelEnvironmentKeysFailLoudly(t *testing.T) {
	for _, key := range []string{
		"OMNI_CONVERSATION_CONTEXT_SELECTION_MODEL",
		"OMNI_MEMORY_CONTEXT_SELECTION_MODEL",
		"OMNI_ROLEPLAY_NARRATIVE_CONTINUITY_MODEL",
	} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "retired-model")
			if err := validateTypedEnvironment(); err == nil {
				t.Fatalf("retired environment key %s was silently ignored", key)
			}
		})
	}
}
