package cognitionpolicy

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func policySnapshotWithBudget(
	t *testing.T,
	snapshot cognition.RuntimeSnapshot,
	budget cognition.RuntimeBudget,
) cognition.RuntimeSnapshot {
	t.Helper()
	updated, err := cognition.NewRuntimeSnapshot(
		snapshot.Goal(), snapshot.CurrentRevision(), snapshot.CurrentObligation(),
		snapshot.ActionCatalog(), snapshot.Attempt(), snapshot.ContextProjection(),
		budget, snapshot.EvidenceRefs(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return updated
}
