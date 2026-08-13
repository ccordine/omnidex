package modelconfig

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/station"
)

func TestRepositoryGroundingModelConfigMapsOnlyExactStations(t *testing.T) {
	t.Parallel()
	cfg, err := FromJSON(json.RawMessage(`{
		"repository_evidence_relevance_model":"relevance-model",
		"repository_grounded_review_model":"review-model",
		"repository_grounded_correction_model":"correction-model"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	routing := Apply(Routing{}, cfg)
	want := map[station.ID]string{
		station.RepositoryEvidenceRelevance:  "relevance-model",
		station.RepositoryGroundedReview:     "review-model",
		station.RepositoryGroundedCorrection: "correction-model",
	}
	if len(routing.Stations) != len(want) {
		t.Fatalf("routing=%#v", routing.Stations)
	}
	for id, model := range want {
		if routing.Stations[id] != model {
			t.Fatalf("station %q=%q want %q", id, routing.Stations[id], model)
		}
	}
}

func TestRepositoryGroundingRemovedGenericAliasesRemainRejected(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"repository_model", "review_model", "correction_model"} {
		raw, err := json.Marshal(map[string]string{key: "model"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := FromJSON(raw); err == nil {
			t.Fatalf("generic model authority %q was accepted", key)
		}
	}
}
