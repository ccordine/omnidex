package cognitionstate

import (
	"testing"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestReconciliationOmitsReadySiblingStateAndScopedEntries(t *testing.T) {
	t.Parallel()
	snapshot, graph, observation := attentionTestRuntime(t)
	ledger, err := taskstate.RestoreLedger(attentionTestLedger(t, observation))
	if err != nil {
		t.Fatal(err)
	}
	for index, node := range []taskstate.AddNodeCommand{
		{Actor: taskstate.AuthorityCode, ID: "obligation-41", Kind: taskstate.NodeGoal,
			Title: "Current obligation", Priority: 100, Metadata: taskstate.EmptyJSONObject()},
		{Actor: taskstate.AuthorityCode, ID: "obligation-ready", Kind: taskstate.NodeGoal,
			Title: "Ready sibling", Priority: 90, Metadata: taskstate.EmptyJSONObject()},
	} {
		node.ExpectedVersion = ledger.Version()
		node.CommandID, err = taskstate.NewCommandID(t.Name(), "node", string(node.ID))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ledger.Apply(node); err != nil {
			t.Fatalf("add node %d: %v", index, err)
		}
	}
	for index, entry := range []taskstate.AddEntryCommand{
		{Actor: taskstate.AuthorityCode, ID: "constraint-current", ScopeNodeID: "obligation-41",
			Kind: taskstate.EntryConstraint, Content: "Constraint for the active obligation.",
			Metadata: taskstate.EmptyJSONObject(), Refs: []taskstate.Ref{}},
		{Actor: taskstate.AuthorityCode, ID: "constraint-sibling", ScopeNodeID: "obligation-ready",
			Kind: taskstate.EntryConstraint, Content: "Constraint for a future sibling.",
			Metadata: taskstate.EmptyJSONObject(), Refs: []taskstate.Ref{}},
		{Actor: taskstate.AuthorityToolEvidence, ID: "failure-sibling", ScopeNodeID: "obligation-ready",
			Kind: taskstate.EntryFailure, Content: "Failure from a future sibling.",
			Metadata: taskstate.EmptyJSONObject(), Refs: []taskstate.Ref{}},
	} {
		entry.ExpectedVersion = ledger.Version()
		entry.CommandID, err = taskstate.NewCommandID(t.Name(), "entry", string(entry.ID))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ledger.Apply(entry); err != nil {
			t.Fatalf("add entry %d: %v", index, err)
		}
	}
	set := attentionTestWorkingSet(t, ledger.MaterializedState(), workingset.Budget{
		MaxItems: 32, MaxBytes: 256 * 1024, MaxPinnedItems: 16, MaxPinnedBytes: 192 * 1024,
	})
	plan, err := BuildDefaultReconciliation(ReconciliationInput{
		State: attentionProjectionState(t, snapshot), ObligationGraph: graph,
		Ledger: ledger.MaterializedState(), WorkingSet: set.Snapshot(),
		Evidence: []EvidenceMaterial{{Ref: observation.EvidenceRef(), Content: observation.Content}},
	})
	if err != nil {
		t.Fatal(err)
	}
	content := make(map[string]struct{})
	roles := make(map[workingset.Role]int)
	for _, material := range plan.Materials() {
		content[material.Content] = struct{}{}
	}
	for _, item := range applyAttentionPlan(t, set.Snapshot(), plan).ResidentItems() {
		roles[item.Role]++
	}
	if _, exists := content["Constraint for the active obligation."]; !exists {
		t.Fatal("active obligation constraint was omitted")
	}
	for _, forbidden := range []string{
		"Constraint for a future sibling.", "Failure from a future sibling.",
	} {
		if _, exists := content[forbidden]; exists {
			t.Fatalf("clean desk included unrelated sibling material %q", forbidden)
		}
	}
	if roles[workingset.RoleDependency] != 0 {
		t.Fatalf("clean desk included %d ready sibling obligations", roles[workingset.RoleDependency])
	}
}
