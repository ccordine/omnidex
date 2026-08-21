package modelconfig

import (
	"testing"

	"github.com/gryph/omnidex/internal/station"
)

func TestContextSieveFieldsLoadAndApplyToExactStations(t *testing.T) {
	t.Setenv("OMNI_CONTEXT_SEARCH_TERMS_MODEL", "terms-model")
	t.Setenv("OMNI_CONTEXT_RELEVANCE_MODEL", "relevance-model")
	t.Setenv("OMNI_CONTEXT_MINIFICATION_MODEL", "minification-model")

	cfg := FromEnv()
	routing := Apply(Routing{}, cfg)
	wants := map[station.ID]string{
		station.ContextSearchTerms:  "terms-model",
		station.ContextRelevance:    "relevance-model",
		station.ContextMinification: "minification-model",
	}
	for id, want := range wants {
		if got := routing.Stations[id]; got != want {
			t.Fatalf("station %s model=%q want %q", id, got, want)
		}
	}
}

func TestContextSieveModelConfigRejectsUnknownAlias(t *testing.T) {
	if _, err := FromJSON([]byte(`{"context_agent_model":"forbidden"}`)); err == nil {
		t.Fatal("unregistered context-agent model alias was accepted")
	}
	for _, retired := range []string{
		`{"conversation_context_selection_model":"forbidden"}`,
		`{"memory_context_selection_model":"forbidden"}`,
		`{"roleplay_narrative_continuity_model":"forbidden"}`,
	} {
		if _, err := FromJSON([]byte(retired)); err == nil {
			t.Fatalf("retired context selector model field was accepted: %s", retired)
		}
	}
}
