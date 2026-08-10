package cognitionstate

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestReconciliationKeepsOnlyLatestUnresolvedFailure(t *testing.T) {
	t.Parallel()
	snapshot, graph, observation := attentionTestRuntime(t)
	state := attentionTestLedger(t, observation)
	ledger, err := taskstate.RestoreLedger(state)
	if err != nil {
		t.Fatal(err)
	}
	schema := mappingTestSchema(t)
	action := mappingTestAction(t, schema)
	failure, err := cognition.NewActionFailure(
		cognition.ActionFailurePreconditionFailed, action, mappingTestRevision(),
		"The newest unresolved public failure.", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	mutation, err := MapActionFailure(ActionFailureInput{
		Ledger: ledger.MaterializedState(), Binding: ActionBinding{Action: action, Schema: schema},
		ExpectedRevision: mappingTestRevision(), Failure: failure,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(mutation.Command()); err != nil {
		t.Fatal(err)
	}
	set := attentionTestWorkingSet(t, ledger.MaterializedState(), workingset.Budget{
		MaxItems: 16, MaxBytes: 128 * 1024, MaxPinnedItems: 10, MaxPinnedBytes: 96 * 1024,
	})
	plan, err := BuildDefaultReconciliation(ReconciliationInput{
		State: attentionProjectionState(t, snapshot), ObligationGraph: graph, Ledger: ledger.MaterializedState(),
		WorkingSet: set.Snapshot(),
		Evidence:   []EvidenceMaterial{{Ref: observation.EvidenceRef(), Content: observation.Content}},
	})
	if err != nil {
		t.Fatal(err)
	}
	projected := applyAttentionPlan(t, set.Snapshot(), plan)
	materialByID := make(map[workingset.ItemID]string)
	for _, material := range plan.Materials() {
		materialByID[material.ItemID] = material.Content
	}
	failures := 0
	for _, item := range projected.ResidentItems() {
		if item.Role != workingset.RoleFailure {
			continue
		}
		failures++
		if materialByID[item.ID] != failure.PublicMessage {
			t.Fatalf("resident failure = %q, want latest %q", materialByID[item.ID], failure.PublicMessage)
		}
	}
	if failures != 1 {
		t.Fatalf("failure material count = %d, want 1", failures)
	}
}
