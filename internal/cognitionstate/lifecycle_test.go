package cognitionstate

import (
	"testing"

	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestReconciliationReleasesRejectedSupersededAndResolvedEntries(t *testing.T) {
	t.Parallel()
	snapshot, graph, observation := attentionTestRuntime(t)
	state := attentionTestLedger(t, observation)
	ledger, err := taskstate.RestoreLedger(state)
	if err != nil {
		t.Fatal(err)
	}
	for index, entry := range []struct {
		id      taskstate.EntryID
		content string
	}{
		{"constraint-new", "Use the newer accepted constraint."},
		{"constraint-rejected", "This constraint will be rejected."},
	} {
		commandID, err := taskstate.NewCommandID("attention-lifecycle-add", string(entry.id))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ledger.Apply(taskstate.AddEntryCommand{
			CommandID: commandID, ExpectedVersion: ledger.Version(), Actor: taskstate.AuthorityCode,
			ID: entry.id, Kind: taskstate.EntryConstraint, Content: entry.content,
			Metadata: taskstate.EmptyJSONObject(), Refs: []taskstate.Ref{},
		}); err != nil {
			t.Fatalf("add constraint %d: %v", index, err)
		}
	}
	state = ledger.MaterializedState()
	set := attentionTestWorkingSet(t, state, workingset.Budget{
		MaxItems: 32, MaxBytes: 256 * 1024, MaxPinnedItems: 20, MaxPinnedBytes: 192 * 1024,
	})
	first, err := BuildDefaultReconciliation(ReconciliationInput{
		State: attentionProjectionState(t, snapshot), ObligationGraph: graph, Ledger: state, WorkingSet: set.Snapshot(),
		Evidence: []EvidenceMaterial{{Ref: observation.EvidenceRef(), Content: observation.Content}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resident := applyAttentionPlan(t, set.Snapshot(), first)
	itemByContent := make(map[string]workingset.ItemID)
	for _, material := range first.Materials() {
		itemByContent[material.Content] = material.ItemID
	}

	supersedeID, err := taskstate.NewCommandID("attention-lifecycle", "supersede")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(taskstate.SupersedeEntryCommand{
		CommandID: supersedeID, ExpectedVersion: ledger.Version(), Actor: taskstate.AuthorityCode,
		EntryID: "constraint-41", ReplacementID: "constraint-new", Reason: "A newer constraint superseded it.",
	}); err != nil {
		t.Fatal(err)
	}
	rejectID, err := taskstate.NewCommandID("attention-lifecycle", "reject")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(taskstate.RejectEntryCommand{
		CommandID: rejectID, ExpectedVersion: ledger.Version(), Actor: taskstate.AuthorityCode,
		EntryID: "constraint-rejected", Reason: "Code rejected this constraint.",
	}); err != nil {
		t.Fatal(err)
	}
	var failureID taskstate.EntryID
	for _, entry := range ledger.Entries() {
		if entry.Kind == taskstate.EntryFailure && entry.Status == taskstate.EntryActive {
			failureID = entry.ID
		}
	}
	resolveID, err := taskstate.NewCommandID("attention-lifecycle", "resolve")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(taskstate.ResolveEntryCommand{
		CommandID: resolveID, ExpectedVersion: ledger.Version(), Actor: taskstate.AuthorityCode,
		EntryID: failureID, Reason: "Tool evidence resolved the failure.",
		Refs: []taskstate.Ref{evidenceLedgerRef(observation.EvidenceRef())},
	}); err != nil {
		t.Fatal(err)
	}
	second, err := BuildDefaultReconciliation(ReconciliationInput{
		State: attentionProjectionState(t, snapshot), ObligationGraph: graph, Ledger: ledger.MaterializedState(),
		WorkingSet: resident.Snapshot(),
		Evidence:   []EvidenceMaterial{{Ref: observation.EvidenceRef(), Content: observation.Content}},
	})
	if err != nil {
		t.Fatal(err)
	}
	finalSet := applyAttentionPlan(t, resident.Snapshot(), second)
	for _, content := range []string{
		"Preserve the public authority boundary.",
		"This constraint will be rejected.",
		"The current public precondition is unresolved.",
	} {
		item, ok := finalSet.Item(itemByContent[content])
		if !ok || item.State != workingset.ItemReleased {
			t.Fatalf("inactive content %q was not released: %#v", content, item)
		}
	}
	var replacementItemID workingset.ItemID
	for _, material := range second.Materials() {
		if material.Content == "Use the newer accepted constraint." {
			replacementItemID = material.ItemID
		}
	}
	newItem, ok := finalSet.Item(replacementItemID)
	if !ok || newItem.State != workingset.ItemResident {
		t.Fatalf("active replacement is not resident: %#v", newItem)
	}
	if _, err := contextbuilder.Build(contextbuilder.BuildInput{
		WorkID: "attention-lifecycle", Spec: second.ContextSpec(),
		WorkingSet: finalSet, Materials: second.Materials(),
	}); err != nil {
		t.Fatalf("build post-lifecycle context: %v", err)
	}
}
