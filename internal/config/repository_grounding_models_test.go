package config

import (
	"testing"

	"github.com/gryph/omnidex/internal/station"
)

func TestRepositoryEvidenceRelevanceModelLoadsFromExactEnvironmentKey(t *testing.T) {
	t.Setenv("OMNI_REPOSITORY_EVIDENCE_RELEVANCE_MODEL", "relevance-model")
	models := loadStationModels(Config{})
	if got := models[station.RepositoryEvidenceRelevance]; got != "relevance-model" {
		t.Fatalf("station %q=%q want %q", station.RepositoryEvidenceRelevance, got, "relevance-model")
	}
}

func TestRetiredRepositoryGroundingEnvironmentKeysFailLoudly(t *testing.T) {
	for _, key := range []string{
		"OMNI_REPOSITORY_GROUNDED_REVIEW_MODEL",
		"OMNI_REPOSITORY_GROUNDED_CORRECTION_MODEL",
	} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "retired-model")
			if err := validateTypedEnvironment(); err == nil {
				t.Fatalf("retired environment key %s was silently ignored", key)
			}
		})
	}
}
