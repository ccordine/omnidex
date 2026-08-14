package config

import (
	"testing"

	"github.com/gryph/omnidex/internal/station"
)

func TestRepositoryGroundingStationModelsLoadFromExactEnvironmentKeys(t *testing.T) {
	t.Setenv("OMNI_OBJECTIVE_ADVISORY_MODEL", "advisory-model")
	t.Setenv("OMNI_REPOSITORY_EVIDENCE_RELEVANCE_MODEL", "relevance-model")
	t.Setenv("OMNI_REPOSITORY_GROUNDED_REVIEW_MODEL", "review-model")
	t.Setenv("OMNI_REPOSITORY_GROUNDED_CORRECTION_MODEL", "correction-model")
	models := loadStationModels(Config{})
	for id, want := range map[station.ID]string{
		station.ObjectiveAdvisory:            "advisory-model",
		station.RepositoryEvidenceRelevance:  "relevance-model",
		station.RepositoryGroundedReview:     "review-model",
		station.RepositoryGroundedCorrection: "correction-model",
	} {
		if models[id] != want {
			t.Fatalf("station %q=%q want %q", id, models[id], want)
		}
	}
}
