package taskstate

import (
	"errors"
	"testing"
)

func TestNodeGenerationSupersessionRetiresHistoryWithoutBlockingClose(t *testing.T) {
	ledger := newTestLedger(t)
	addSupersessionTestNode(t, ledger, "root", "", NodeGoal)
	addSupersessionTestNode(t, ledger, "old-objective", "root", NodeObjective)
	applyTestCommand(t, ledger, PromoteReadyNodesCommand{
		ExpectedVersion: ledger.Version(), Actor: AuthorityCode,
	})
	applyTestCommand(t, ledger, TransitionNodeCommand{
		ExpectedVersion: ledger.Version(), Actor: AuthorityCode,
		NodeID: "root", To: NodeActive,
	})
	applyTestCommand(t, ledger, TransitionNodeCommand{
		ExpectedVersion: ledger.Version(), Actor: AuthorityCode,
		NodeID: "old-objective", To: NodeActive,
	})

	commandID, err := NewCommandID("supersede", "generation-1", "generation-2")
	if err != nil {
		t.Fatal(err)
	}
	command := SupersedeNodeGenerationCommand{
		CommandID: commandID, ExpectedVersion: ledger.Version(), Actor: AuthorityCode,
		RetiringGeneration: 1, SupersededAtGeneration: 2,
		NodeIDs: []NodeID{"old-objective"}, Reason: "Generation 2 replaced generation 1.",
	}
	event, err := ledger.Apply(command)
	if err != nil {
		t.Fatalf("apply supersession: %v", err)
	}
	if event.Kind != EventNodeGenerationSuperseded || event.RetiringGeneration != 1 ||
		event.SupersededAtGeneration != 2 {
		t.Fatalf("supersession event = %+v", event)
	}
	old, _ := ledger.Node("old-objective")
	if old.Status != NodeCanceled || old.StatusReason != command.Reason {
		t.Fatalf("retired node = %+v", old)
	}
	supersession, ok := ledger.NodeSupersession(old.ID)
	if !ok || supersession.CreatedVersion != event.Version || supersession.Reason != command.Reason {
		t.Fatalf("supersession record = %+v, exists=%v", supersession, ok)
	}

	version := ledger.Version()
	replayed, err := ledger.Apply(command)
	if err != nil || replayed.CommandSHA256 != event.CommandSHA256 || ledger.Version() != version {
		t.Fatalf("exact supersession replay = %+v, err=%v version=%d", replayed, err, ledger.Version())
	}
	changed := command
	changed.Reason = "Changed retry content."
	if _, err := ledger.Apply(changed); !errors.Is(err, ErrCommandIDConflict) {
		t.Fatalf("changed supersession replay error = %v", err)
	}

	addSupersessionTestNode(t, ledger, "new-objective", "root", NodeObjective)
	applyTestCommand(t, ledger, PromoteReadyNodesCommand{
		ExpectedVersion: ledger.Version(), Actor: AuthorityCode,
	})
	applyTestCommand(t, ledger, TransitionNodeCommand{
		ExpectedVersion: ledger.Version(), Actor: AuthorityCode,
		NodeID: "new-objective", To: NodeActive,
	})
	completeSupersessionTestNode(t, ledger, "new-objective", 17)
	completeSupersessionTestNode(t, ledger, "root", 17)
	applyTestCommand(t, ledger, CloseLedgerCommand{
		ExpectedVersion: ledger.Version(), Actor: AuthorityCode,
		Status: LedgerClosed, StepID: int64Pointer(17), Reason: "Current obligations are verified.",
	})
	reconstructed, err := Reconstruct(ledger.ID(), ledger.Owner(), ledger.Events())
	if err != nil {
		t.Fatalf("reconstruct superseded ledger: %v", err)
	}
	if reconstructed.Status() != LedgerClosed {
		t.Fatalf("reconstructed status = %q", reconstructed.Status())
	}

	restored, err := RestoreLedger(ledger.MaterializedState())
	if err != nil {
		t.Fatalf("restore superseded ledger: %v", err)
	}
	if restored.Status() != LedgerClosed {
		t.Fatalf("restored status = %q", restored.Status())
	}
}

func TestSupersededNodeRejectsNewAuthorityAndRestoreTampering(t *testing.T) {
	ledger := newTestLedger(t)
	addSupersessionTestNode(t, ledger, "root", "", NodeGoal)
	addSupersessionTestNode(t, ledger, "old", "root", NodeObjective)
	applyTestCommand(t, ledger, SupersedeNodeGenerationCommand{
		ExpectedVersion: ledger.Version(), Actor: AuthorityCode,
		RetiringGeneration: 8, SupersededAtGeneration: 9,
		NodeIDs: []NodeID{"old"}, Reason: "A later generation replaced this obligation.",
	})

	before := ledger.Version()
	_, err := ledger.Apply(withTestCommandID(t, AddEntryCommand{
		ExpectedVersion: before, Actor: AuthorityCode, ID: "stale-entry",
		ScopeNodeID: "old", Kind: EntryNote, Content: "Forbidden stale state.",
		Metadata: EmptyJSONObject(),
	}))
	if !errors.Is(err, ErrInvalidState) || ledger.Version() != before {
		t.Fatalf("stale scoped entry error=%v version=%d", err, ledger.Version())
	}
	_, err = ledger.Apply(withTestCommandID(t, TransitionNodeCommand{
		ExpectedVersion: before, Actor: AuthorityCode, NodeID: "old", To: NodeReady,
		Reason: "Forbidden reopening.",
	}))
	if !errors.Is(err, ErrInvalidState) || ledger.Version() != before {
		t.Fatalf("stale node transition error=%v version=%d", err, ledger.Version())
	}

	state := ledger.MaterializedState()
	state.NodeSupersessions[0].SupersededAtGeneration = 10
	if _, err := RestoreLedger(state); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("forged successor restore error=%v", err)
	}
	state = ledger.MaterializedState()
	for index := range state.Nodes {
		if state.Nodes[index].ID == "old" {
			state.Nodes[index].Status = NodeActive
			state.Nodes[index].StatusReason = ""
		}
	}
	if _, err := RestoreLedger(state); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("active superseded node restore error=%v", err)
	}
}

func TestNodeGenerationSupersessionRejectsRootAndNonSuccessor(t *testing.T) {
	ledger := newTestLedger(t)
	addSupersessionTestNode(t, ledger, "root", "", NodeGoal)
	for _, command := range []SupersedeNodeGenerationCommand{
		{ExpectedVersion: ledger.Version(), Actor: AuthorityCode, RetiringGeneration: 1,
			SupersededAtGeneration: 3, NodeIDs: []NodeID{"root"}, Reason: "Skipped generation."},
		{ExpectedVersion: ledger.Version(), Actor: AuthorityCode, RetiringGeneration: 1,
			SupersededAtGeneration: 2, NodeIDs: []NodeID{"root"}, Reason: "Forbidden root."},
	} {
		_, err := ledger.Apply(withTestCommandID(t, command))
		if !errors.Is(err, ErrInvalidCommand) && !errors.Is(err, ErrInvalidState) {
			t.Fatalf("invalid supersession error=%v", err)
		}
	}
}

func addSupersessionTestNode(t *testing.T, ledger *Ledger, id, parent NodeID, kind NodeKind) {
	t.Helper()
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: ledger.Version(), Actor: AuthorityCode,
		ID: id, ParentID: parent, Kind: kind, Title: string(id), Priority: 50,
		Metadata: EmptyJSONObject(),
	})
}

func completeSupersessionTestNode(t *testing.T, ledger *Ledger, id NodeID, stepID int64) {
	t.Helper()
	applyTestCommand(t, ledger, TransitionNodeCommand{
		ExpectedVersion: ledger.Version(), Actor: AuthorityCode, NodeID: id,
		To: NodeDone, CompletedStepID: &stepID, VerificationRefs: testVerificationRefs(),
	})
}
