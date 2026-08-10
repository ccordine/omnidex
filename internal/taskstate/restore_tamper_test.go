package taskstate

import (
	"errors"
	"testing"
)

func TestRestoreRejectsNonDoneCompletedStep(t *testing.T) {
	ledger := ledgerWithOneRestorableNode(t)
	state := ledger.MaterializedState()
	completed := int64(12)
	state.Nodes[0].CompletedStepID = &completed
	assertRestoreInvalid(t, state)
}

func TestRestoreRejectsNonExactStatusAndDispositionReasons(t *testing.T) {
	t.Run("node status reason", func(t *testing.T) {
		ledger := ledgerWithOneRestorableNode(t)
		state := ledger.MaterializedState()
		state.Nodes[0].Status = NodeBlocked
		assigned := int64(12)
		state.Nodes[0].AssignedStepID = &assigned
		state.Nodes[0].StatusReason = " padded reason "
		assertRestoreInvalid(t, state)
	})
	t.Run("entry disposition reason", func(t *testing.T) {
		ledger := newTestLedger(t)
		applyTestCommand(t, ledger, AddEntryCommand{
			ExpectedVersion: 0, Actor: AuthorityCode, ID: "note", Kind: EntryNote,
			Content: "Note.", Metadata: EmptyJSONObject(),
		})
		state := ledger.MaterializedState()
		state.Entries[0].Status = EntryRejected
		state.Entries[0].DispositionReason = " padded reason "
		assertRestoreInvalid(t, state)
	})
}

func TestRestoreRejectsMixedParentObjectiveCycle(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 0, Actor: AuthorityCode, ID: "a",
		Kind: NodeObjective, Title: "A", Priority: 1, Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 1, Actor: AuthorityCode, ID: "b",
		Kind: NodeObjective, Title: "B", Priority: 1, Metadata: EmptyJSONObject(),
	})
	state := ledger.MaterializedState()
	for index := range state.Nodes {
		if state.Nodes[index].ID == "a" {
			state.Nodes[index].ParentID = "b"
		} else {
			state.Nodes[index].ObjectiveID = "a"
		}
	}
	assertRestoreInvalid(t, state)
}

func TestRestoreRejectsDecisionProvenanceOnOrdinaryEntry(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 0, Actor: AuthorityCode, ID: "note", Kind: EntryNote,
		Content: "Note.", Metadata: EmptyJSONObject(),
	})
	state := ledger.MaterializedState()
	state.Entries[0].Provenance.AcceptancePolicy = "forged-policy"
	assertRestoreInvalid(t, state)
}

func ledgerWithOneRestorableNode(t *testing.T) *Ledger {
	t.Helper()
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 0, Actor: AuthorityCode, ID: "task",
		Kind: NodeTask, Title: "Task", Priority: 1, Metadata: EmptyJSONObject(),
	})
	return ledger
}

func assertRestoreInvalid(t *testing.T, state MaterializedState) {
	t.Helper()
	_, err := RestoreLedger(state)
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("restore tamper error=%v", err)
	}
}
