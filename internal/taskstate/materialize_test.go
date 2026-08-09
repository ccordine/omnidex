package taskstate

import (
	"errors"
	"reflect"
	"testing"
)

func TestReconstructSelectsNextRunnableDeterministically(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 0, Actor: AuthorityCode,
		ID: "low", Kind: NodeTask, Title: "Low", Priority: 1, Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 1, Actor: AuthorityCode,
		ID: "z-created-first", Kind: NodeTask, Title: "High first", Priority: 9, Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 2, Actor: AuthorityCode,
		ID: "a-created-second", Kind: NodeTask, Title: "High second", Priority: 9, Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, PromoteReadyNodesCommand{ExpectedVersion: 3, Actor: AuthorityCode})

	rebuilt, err := Reconstruct(ledger.ID(), ledger.Owner(), ledger.Events())
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.Version() != ledger.Version() {
		t.Fatalf("rebuilt version=%d, want %d", rebuilt.Version(), ledger.Version())
	}
	next, ok := rebuilt.NextRunnableNode()
	if !ok || next.ID != "z-created-first" {
		t.Fatalf("next runnable=%+v present=%t", next, ok)
	}

	applyTestCommand(t, rebuilt, AssignNodeStepCommand{
		ExpectedVersion: rebuilt.Version(), Actor: AuthorityCode,
		NodeID: "z-created-first", StepID: 71,
	})
	applyTestCommand(t, rebuilt, TransitionNodeCommand{
		ExpectedVersion: rebuilt.Version(), Actor: AuthorityCode,
		NodeID: "z-created-first", To: NodeActive,
	})
	next, ok = rebuilt.NextRunnableNode()
	if !ok || next.ID != "a-created-second" {
		t.Fatalf("second runnable=%+v present=%t", next, ok)
	}

	rebuiltAgain, err := Reconstruct(rebuilt.ID(), rebuilt.Owner(), rebuilt.Events())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rebuiltAgain.Events(), rebuilt.Events()) {
		t.Fatal("event reconstruction changed immutable history")
	}
}

func TestExpectedVersionConflictDoesNotMutateLedger(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 0, Actor: AuthorityCode,
		ID: "one", Kind: NodeTask, Title: "One", Priority: 1, Metadata: EmptyJSONObject(),
	})

	_, err := ledger.Apply(withTestCommandID(t, AddNodeCommand{
		ExpectedVersion: 0, Actor: AuthorityCode,
		ID: "stale", Kind: NodeTask, Title: "Stale", Priority: 1, Metadata: EmptyJSONObject(),
	}))
	var conflict VersionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("stale command error=%v", err)
	}
	if conflict.Expected != 0 || conflict.Actual != 1 {
		t.Fatalf("version conflict=%+v", conflict)
	}
	if ledger.Version() != 1 {
		t.Fatalf("stale command changed version to %d", ledger.Version())
	}
	if _, exists := ledger.Node("stale"); exists {
		t.Fatal("stale command mutated materialized nodes")
	}
}

func TestReconstructRejectsNonSequentialEventHistory(t *testing.T) {
	ledger := newTestLedger(t)
	event := applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 0, Actor: AuthorityCode,
		ID: "one", Kind: NodeTask, Title: "One", Priority: 1, Metadata: EmptyJSONObject(),
	})
	event.Version = 2

	_, err := Reconstruct(ledger.ID(), ledger.Owner(), []Event{event})
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("non-sequential event error=%v", err)
	}
}
