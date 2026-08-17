package taskstate

import (
	"errors"
	"testing"
)

func TestNodeTransitionsRespectDependenciesAndCompletionAuthority(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 0, Actor: AuthorityCode,
		ID: "build", Kind: NodeTask, Title: "Build", Priority: 10, Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 1, Actor: AuthorityCode,
		ID: "verify", Kind: NodeCheckpoint, Title: "Verify", Priority: 20, Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, AddEdgeCommand{
		ExpectedVersion: 2, Actor: AuthorityCode,
		ID: "verify-after-build", Kind: EdgeDependsOn, From: "verify", To: "build",
	})

	applyTestCommand(t, ledger, PromoteReadyNodesCommand{ExpectedVersion: 3, Actor: AuthorityCode})
	assertNodeStatus(t, ledger, "build", NodeReady)
	assertNodeStatus(t, ledger, "verify", NodePending)

	applyTestCommand(t, ledger, AssignNodeStepCommand{
		ExpectedVersion: 4, Actor: AuthorityCode, NodeID: "build", StepID: 101,
	})
	applyTestCommand(t, ledger, TransitionNodeCommand{
		ExpectedVersion: 5, Actor: AuthorityCode, NodeID: "build", To: NodeActive,
	})
	completedBuildStep := int64(101)
	verification := []Ref{{
		URI: "evidence://step/101", Version: "1",
		Hash:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Relation: RefVerifies,
	}}
	applyTestCommand(t, ledger, TransitionNodeCommand{
		ExpectedVersion: 6, Actor: AuthorityCode, NodeID: "build", To: NodeDone,
		CompletedStepID: &completedBuildStep, VerificationRefs: verification,
	})
	applyTestCommand(t, ledger, PromoteReadyNodesCommand{ExpectedVersion: 7, Actor: AuthorityCode})
	assertNodeStatus(t, ledger, "verify", NodeReady)

	applyTestCommand(t, ledger, AssignNodeStepCommand{
		ExpectedVersion: 8, Actor: AuthorityCode, NodeID: "verify", StepID: 102,
	})
	applyTestCommand(t, ledger, TransitionNodeCommand{
		ExpectedVersion: 9, Actor: AuthorityCode, NodeID: "verify", To: NodeActive,
	})
	blockedEvent := applyTestCommand(t, ledger, TransitionNodeCommand{
		ExpectedVersion: 10, Actor: AuthorityCode, NodeID: "verify", To: NodeBlocked,
		Reason: "The external verification service is unavailable.",
	})
	if blockedEvent.StepID == nil || *blockedEvent.StepID != 102 {
		t.Fatalf("blocked transition lost owning step: %+v", blockedEvent)
	}
	_, err := ledger.Apply(withTestCommandID(t, PromoteReadyNodesCommand{ExpectedVersion: 11, Actor: AuthorityCode}))
	if !errors.Is(err, ErrNoStateChange) {
		t.Fatalf("blocked node was silently promoted: %v", err)
	}
	assertNodeStatus(t, ledger, "verify", NodeBlocked)
	applyTestCommand(t, ledger, TransitionNodeCommand{
		ExpectedVersion: 11, Actor: AuthorityCode, NodeID: "verify", To: NodeReady,
		Reason: "The verification service recovered and the health probe passed.",
	})
	assertNodeStatus(t, ledger, "verify", NodeReady)
}

func TestInvalidCompletionAuthorityLeavesStateUnchanged(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 0, Actor: AuthorityCode,
		ID: "task", Kind: NodeTask, Title: "Task", Priority: 1, Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, PromoteReadyNodesCommand{ExpectedVersion: 1, Actor: AuthorityCode})
	applyTestCommand(t, ledger, AssignNodeStepCommand{
		ExpectedVersion: 2, Actor: AuthorityCode, NodeID: "task", StepID: 7,
	})
	applyTestCommand(t, ledger, TransitionNodeCommand{
		ExpectedVersion: 3, Actor: AuthorityCode, NodeID: "task", To: NodeActive,
	})

	beforeVersion := ledger.Version()
	completionStep := int64(7)
	_, err := ledger.Apply(withTestCommandID(t, TransitionNodeCommand{
		ExpectedVersion: beforeVersion, Actor: AuthorityModelProposal,
		NodeID: "task", To: NodeDone, CompletedStepID: &completionStep,
		VerificationRefs: testVerificationRefs(),
	}))
	if !errors.Is(err, ErrAuthorityDenied) {
		t.Fatalf("completion error=%v, want authority denied", err)
	}
	if ledger.Version() != beforeVersion {
		t.Fatalf("rejected transition changed version from %d to %d", beforeVersion, ledger.Version())
	}
	assertNodeStatus(t, ledger, "task", NodeActive)

	_, err = ledger.Apply(withTestCommandID(t, TransitionNodeCommand{
		ExpectedVersion: beforeVersion, Actor: AuthorityAcceptedModelDecision,
		NodeID: "task", To: NodeDone, CompletedStepID: &completionStep,
		VerificationRefs: testVerificationRefs(),
	}))
	if !errors.Is(err, ErrAuthorityDenied) {
		t.Fatalf("accepted model decision completed node: %v", err)
	}
}

func TestInlineTaskUsesItsCreatingStepWithoutASecondQueueAssignment(t *testing.T) {
	ledger := newTestLedger(t)
	stepID := int64(71)
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 0, Actor: AuthorityCode,
		ID: "inline-task", Kind: NodeTask, InlineExecution: true,
		Title: "Generate one bounded source block", Priority: 10,
		CreatedStepID: &stepID, Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, PromoteReadyNodesCommand{ExpectedVersion: 1, Actor: AuthorityCode})
	applyTestCommand(t, ledger, TransitionNodeCommand{
		ExpectedVersion: 2, Actor: AuthorityCode, NodeID: "inline-task", To: NodeActive,
	})
	applyTestCommand(t, ledger, TransitionNodeCommand{
		ExpectedVersion: 3, Actor: AuthorityCode, NodeID: "inline-task", To: NodeDone,
		CompletedStepID: &stepID, VerificationRefs: testVerificationRefs(),
	})
	node, ok := ledger.Node("inline-task")
	if !ok || node.AssignedStepID != nil || node.CompletedStepID == nil || *node.CompletedStepID != stepID {
		t.Fatalf("inline task authority=%+v", node)
	}
	if _, err := ledger.Apply(withTestCommandID(t, AssignNodeStepCommand{
		ExpectedVersion: ledger.Version(), Actor: AuthorityCode, NodeID: "inline-task", StepID: 72,
	})); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("inline task accepted a separate queue assignment: %v", err)
	}
}

func TestInlineExecutionRejectsNonTaskAndMissingOwner(t *testing.T) {
	ledger := newTestLedger(t)
	for _, command := range []AddNodeCommand{
		{ExpectedVersion: 0, Actor: AuthorityCode, ID: "objective", Kind: NodeObjective, InlineExecution: true, Title: "Objective", Priority: 1, Metadata: EmptyJSONObject()},
		{ExpectedVersion: 0, Actor: AuthorityCode, ID: "task", Kind: NodeTask, InlineExecution: true, Title: "Task", Priority: 1, Metadata: EmptyJSONObject()},
	} {
		if _, err := ledger.Apply(withTestCommandID(t, command)); !errors.Is(err, ErrInvalidCommand) {
			t.Fatalf("command %+v error=%v, want invalid command", command, err)
		}
	}
}

func TestTerminalNodeCannotReopen(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 0, Actor: AuthorityCode,
		ID: "task", Kind: NodeTask, Title: "Task", Priority: 1, Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, PromoteReadyNodesCommand{ExpectedVersion: 1, Actor: AuthorityCode})
	applyTestCommand(t, ledger, AssignNodeStepCommand{
		ExpectedVersion: 2, Actor: AuthorityCode, NodeID: "task", StepID: 8,
	})
	applyTestCommand(t, ledger, TransitionNodeCommand{
		ExpectedVersion: 3, Actor: AuthorityCode, NodeID: "task", To: NodeActive,
	})
	completedStep := int64(8)
	applyTestCommand(t, ledger, TransitionNodeCommand{
		ExpectedVersion: 4, Actor: AuthorityCode, NodeID: "task", To: NodeDone,
		CompletedStepID: &completedStep, VerificationRefs: testVerificationRefs(),
	})

	_, err := ledger.Apply(withTestCommandID(t, TransitionNodeCommand{
		ExpectedVersion: 5, Actor: AuthorityCode, NodeID: "task", To: NodeReady,
	}))
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("terminal reopen error=%v", err)
	}
}

func newTestLedger(t *testing.T) *Ledger {
	t.Helper()
	owner := LedgerOwner{
		Kind: OwnerJob, JobID: 41, RunID: "4d36e96e-e325-11ce-bfc1-08002be10318",
	}
	id, err := NewLedgerID(owner)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := NewLedger(id, owner)
	if err != nil {
		t.Fatal(err)
	}
	return ledger
}

func applyTestCommand(t *testing.T, ledger *Ledger, command Command) Event {
	t.Helper()
	event, err := ledger.Apply(withTestCommandID(t, command))
	if err != nil {
		t.Fatalf("apply %T: %v", command, err)
	}
	return event
}

func testVerificationRefs() []Ref {
	return []Ref{{
		URI: "evidence://verification/test", Version: "1",
		Hash:     "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		Relation: RefVerifies,
	}}
}

func assertNodeStatus(t *testing.T, ledger *Ledger, id NodeID, want NodeStatus) {
	t.Helper()
	node, ok := ledger.Node(id)
	if !ok {
		t.Fatalf("node %q is missing", id)
	}
	if node.Status != want {
		t.Fatalf("node %q status=%q, want %q", id, node.Status, want)
	}
}
