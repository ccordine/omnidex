package cognitionstate

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestProjectionStateKeepsPlanGenerationIndependentAndEvidenceExplicit(t *testing.T) {
	t.Parallel()
	observation := mappingTestObservation(t, "")
	base := mappingTestSnapshot(t, observation.EvidenceRef())
	current := base.CurrentObligation()
	current.CreatedGeneration = 7
	current.SupportingRefs = []cognition.EvidenceRef{}
	snapshot, err := cognition.NewRuntimeSnapshot(
		base.Goal(), base.CurrentRevision(), current, base.ActionCatalog(),
		base.Attempt(), base.ContextProjection(), base.Budget(), []cognition.EvidenceRef{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Attempt().Generation == int64(current.CreatedGeneration) {
		t.Fatal("fixture did not separate worker and plan generations")
	}
	state, err := ProjectionStateFromSnapshot(snapshot)
	if err != nil {
		t.Fatalf("build independent projection state: %v", err)
	}
	if state.EvidenceRefs() == nil || len(state.EvidenceRefs()) != 0 {
		t.Fatalf("evidence packet = %#v, want explicit empty", state.EvidenceRefs())
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("validate independent projection state: %v", err)
	}
}
