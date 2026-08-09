package taskstate

import (
	"errors"
	"reflect"
	"testing"
)

func TestNodeCompletionProofSurvivesMaterializationAndRestore(t *testing.T) {
	ledger := completedTaskLedger(t)
	state := ledger.MaterializedState()
	if !reflect.DeepEqual(state.Nodes[0].VerificationRefs, testVerificationRefs()) {
		t.Fatalf("materialized completion proof=%+v", state.Nodes[0].VerificationRefs)
	}
	restored, err := RestoreLedger(state)
	if err != nil {
		t.Fatal(err)
	}
	node, ok := restored.Node("task")
	if !ok || !reflect.DeepEqual(node.VerificationRefs, testVerificationRefs()) {
		t.Fatalf("restored completion proof=%+v", node.VerificationRefs)
	}

	events := ledger.Events()
	events[len(events)-1].VerificationRefs[0].URI = "evidence://mutated"
	node, _ = ledger.Node("task")
	if node.VerificationRefs[0].URI != "evidence://verification/test" {
		t.Fatal("returned event mutated normalized node proof")
	}
}

func TestRestoreRejectsMissingOrMisplacedNodeProof(t *testing.T) {
	t.Run("completed proof removed", func(t *testing.T) {
		state := completedTaskLedger(t).MaterializedState()
		state.Nodes[0].VerificationRefs = []Ref{}
		_, err := RestoreLedger(state)
		if !errors.Is(err, ErrInvalidState) || !errors.Is(err, ErrEvidenceRequired) {
			t.Fatalf("missing completion proof error=%v", err)
		}
	})
	t.Run("pending proof is nil", func(t *testing.T) {
		state := ledgerWithOneRestorableNode(t).MaterializedState()
		state.Nodes[0].VerificationRefs = nil
		assertRestoreInvalid(t, state)
	})
	t.Run("pending acceptance criteria is nil", func(t *testing.T) {
		state := ledgerWithOneRestorableNode(t).MaterializedState()
		state.Nodes[0].AcceptanceCriteria = nil
		assertRestoreInvalid(t, state)
	})
	t.Run("pending node carries proof", func(t *testing.T) {
		state := ledgerWithOneRestorableNode(t).MaterializedState()
		state.Nodes[0].VerificationRefs = testVerificationRefs()
		assertRestoreInvalid(t, state)
	})
}

func TestRestoreRejectsUnassignedExecutableFailureStates(t *testing.T) {
	for _, status := range []NodeStatus{NodeBlocked, NodeFailed} {
		status := status
		t.Run(string(status), func(t *testing.T) {
			ledger := activeTaskLedger(t)
			applyTestCommand(t, ledger, TransitionNodeCommand{
				ExpectedVersion: ledger.Version(), Actor: AuthorityCode,
				NodeID: "task", To: status, Reason: "Authoritative execution failed.",
			})
			state := ledger.MaterializedState()
			state.Nodes[0].AssignedStepID = nil
			assertRestoreInvalid(t, state)
		})
	}
}

func completedTaskLedger(t *testing.T) *Ledger {
	t.Helper()
	ledger := activeTaskLedger(t)
	stepID := int64(41)
	applyTestCommand(t, ledger, TransitionNodeCommand{
		ExpectedVersion: ledger.Version(), Actor: AuthorityCode,
		NodeID: "task", To: NodeDone, CompletedStepID: &stepID,
		VerificationRefs: testVerificationRefs(),
	})
	return ledger
}

func activeTaskLedger(t *testing.T) *Ledger {
	t.Helper()
	ledger := ledgerWithOneRestorableNode(t)
	applyTestCommand(t, ledger, PromoteReadyNodesCommand{
		ExpectedVersion: ledger.Version(), Actor: AuthorityCode,
	})
	applyTestCommand(t, ledger, AssignNodeStepCommand{
		ExpectedVersion: ledger.Version(), Actor: AuthorityCode, NodeID: "task", StepID: 41,
	})
	applyTestCommand(t, ledger, TransitionNodeCommand{
		ExpectedVersion: ledger.Version(), Actor: AuthorityCode, NodeID: "task", To: NodeActive,
	})
	return ledger
}
