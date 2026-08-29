package taskstate

import (
	"errors"
	"testing"
)

func TestExecutionOrderEdgesRejectCyclesAndSemanticDuplicates(t *testing.T) {
	ledger := newTestLedger(t)
	for index, id := range []NodeID{"a", "b", "c"} {
		applyTestCommand(t, ledger, AddNodeCommand{
			ExpectedVersion: uint64(index), Actor: AuthorityCode, ID: id,
			Kind: NodeTask, Title: string(id), Priority: 1, Metadata: EmptyJSONObject(),
		})
	}
	applyTestCommand(t, ledger, AddEdgeCommand{
		ExpectedVersion: 3, Actor: AuthorityCode, ID: "a-after-b",
		Kind: EdgeDependsOn, From: "a", To: "b",
	})
	applyTestCommand(t, ledger, AddEdgeCommand{
		ExpectedVersion: 4, Actor: AuthorityCode, ID: "b-after-c",
		Kind: EdgeDependsOn, From: "b", To: "c",
	})
	_, err := ledger.Apply(withTestCommandID(t, AddEdgeCommand{
		ExpectedVersion: 5, Actor: AuthorityCode, ID: "c-after-a",
		Kind: EdgeDependsOn, From: "c", To: "a",
	}))
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("cycle error=%v", err)
	}
	if ledger.Version() != 5 {
		t.Fatalf("cycle changed ledger version to %d", ledger.Version())
	}
	_, err = ledger.Apply(withTestCommandID(t, AddEdgeCommand{
		ExpectedVersion: 5, Actor: AuthorityCode, ID: "same-order-alias",
		Kind: EdgeBlocks, From: "b", To: "a",
	}))
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("cross-kind semantic duplicate error=%v", err)
	}
	_, err = ledger.Apply(withTestCommandID(t, AddEdgeCommand{
		ExpectedVersion: 5, Actor: AuthorityCode, ID: "duplicate-tuple",
		Kind: EdgeDependsOn, From: "a", To: "b",
	}))
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("semantic duplicate error=%v", err)
	}
}

func TestSuccessfulLedgerCloseRejectsUnfinishedNodes(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 0, Actor: AuthorityCode, ID: "unfinished",
		Kind: NodeTask, Title: "Unfinished", Priority: 1, Metadata: EmptyJSONObject(),
	})
	_, err := ledger.Apply(withTestCommandID(t, CloseLedgerCommand{
		ExpectedVersion: 1, Actor: AuthorityCode, Status: LedgerClosed,
		Reason: "Must not declare success over unfinished work.",
	}))
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("unfinished successful close error=%v", err)
	}
	if ledger.Status() != LedgerActive || ledger.Version() != 1 {
		t.Fatalf("rejected close mutated ledger: status=%q version=%d", ledger.Status(), ledger.Version())
	}
}

func TestFeedbackPurposeAndAuthorityAreTyped(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 0, Actor: AuthorityUser, ID: "feedback",
		Kind: EntryFeedback, FeedbackPurpose: FeedbackReplan,
		Content: "Please account for the changed constraint.", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, ResolveEntryCommand{
		ExpectedVersion: 1, Actor: AuthorityCode, EntryID: "feedback",
		Reason: "The replanned task graph incorporates the feedback.", Refs: testVerificationRefs(),
	})
	for _, invalid := range []AddEntryCommand{
		{ExpectedVersion: 2, Actor: AuthorityToolEvidence, ID: "tool-feedback",
			Kind: EntryFeedback, FeedbackPurpose: FeedbackInterrupt,
			Content: "Not direct user input.", Metadata: EmptyJSONObject()},
		{ExpectedVersion: 2, Actor: AuthorityUser, ID: "purpose-on-note",
			Kind: EntryNote, FeedbackPurpose: FeedbackInputResponse,
			Content: "Wrong kind.", Metadata: EmptyJSONObject()},
	} {
		_, err := ledger.Apply(withTestCommandID(t, invalid))
		if err == nil {
			t.Fatalf("invalid feedback entry accepted: %+v", invalid)
		}
	}
}

func TestSupersessionPreservesSemanticKindAndAuthority(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 0, Actor: AuthorityUser, ID: "user-old", Kind: EntryConstraint,
		Content: "User constraint.", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 1, Actor: AuthorityCode, ID: "code-replacement", Kind: EntryConstraint,
		Content: "Lower authority replacement.", Metadata: EmptyJSONObject(),
	})
	_, err := ledger.Apply(withTestCommandID(t, SupersedeEntryCommand{
		ExpectedVersion: 2, Actor: AuthorityCode, EntryID: "user-old",
		ReplacementID: "code-replacement", Reason: "Forbidden downgrade.",
	}))
	if !errors.Is(err, ErrAuthorityDenied) {
		t.Fatalf("user authority downgrade error=%v", err)
	}
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 2, Actor: AuthorityCode, ID: "hypothesis", Kind: EntryHypothesis,
		Content: "Hypothesis.", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 3, Actor: AuthorityCode, ID: "question", Kind: EntryQuestion,
		Content: "Question?", Metadata: EmptyJSONObject(),
	})
	_, err = ledger.Apply(withTestCommandID(t, SupersedeEntryCommand{
		ExpectedVersion: 4, Actor: AuthorityCode, EntryID: "hypothesis",
		ReplacementID: "question", Reason: "Forbidden semantic change.",
	}))
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("cross-kind supersession error=%v", err)
	}
}

func TestRestoreRejectsCyclicAggregateHierarchy(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 0, Actor: AuthorityCode, ID: "objective-a",
		Kind: NodeObjective, Title: "A", Priority: 1, Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 1, Actor: AuthorityCode, ID: "objective-b", ParentID: "objective-a",
		Kind: NodeObjective, Title: "B", Priority: 1, Metadata: EmptyJSONObject(),
	})
	state := ledger.MaterializedState()
	for index := range state.Nodes {
		if state.Nodes[index].ID == "objective-a" {
			state.Nodes[index].ParentID = "objective-b"
		}
	}
	_, err := RestoreLedger(state)
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("cyclic restored hierarchy error=%v", err)
	}
}

func TestAggregateNodesUseVerifierCompletionButNeverEnterStepQueue(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 0, Actor: AuthorityCode, ID: "goal",
		Kind: NodeGoal, Title: "Goal", Priority: 100, Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 1, Actor: AuthorityCode, ID: "task",
		Kind: NodeTask, Title: "Task", Priority: 1, Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, PromoteReadyNodesCommand{ExpectedVersion: 2, Actor: AuthorityCode})
	next, ok := ledger.NextRunnableNode()
	if !ok || next.ID != "task" {
		t.Fatalf("step queue selected aggregate node: next=%+v present=%t", next, ok)
	}
	applyTestCommand(t, ledger, TransitionNodeCommand{
		ExpectedVersion: 3, Actor: AuthorityCode, NodeID: "goal", To: NodeActive,
	})
	verifierStep := int64(44)
	applyTestCommand(t, ledger, TransitionNodeCommand{
		ExpectedVersion: 4, Actor: AuthorityCode, NodeID: "goal", To: NodeDone,
		CompletedStepID: &verifierStep, VerificationRefs: testVerificationRefs(),
	})
}
