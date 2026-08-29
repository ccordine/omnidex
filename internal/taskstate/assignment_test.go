package taskstate

import (
	"errors"
	"testing"
)

func TestCodeAssignsOnePositiveStepToExecutableNode(t *testing.T) {
	ledger := newTestLedger(t)
	createdStep := int64(11)
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 0, Actor: AuthorityCode,
		ID: "task", Kind: NodeTask, Title: "Task", Priority: 1,
		CreatedStepID: &createdStep, Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, AssignNodeStepCommand{
		ExpectedVersion: 1, Actor: AuthorityCode, NodeID: "task", StepID: 17,
	})
	node, _ := ledger.Node("task")
	if node.CreatedStepID == nil || *node.CreatedStepID != 11 ||
		node.AssignedStepID == nil || *node.AssignedStepID != 17 {
		t.Fatalf("step identities=%+v", node)
	}

	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 2, Actor: AuthorityCode,
		ID: "other", Kind: NodeCheckpoint, Title: "Other", Priority: 1, Metadata: EmptyJSONObject(),
	})
	_, err := ledger.Apply(withTestCommandID(t, AssignNodeStepCommand{
		ExpectedVersion: 3, Actor: AuthorityCode, NodeID: "other", StepID: 17,
	}))
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("duplicate ledger step assignment error=%v", err)
	}
}

func TestNonCodeAuthorityCannotAssignStepAndGoalCannotBeAssigned(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 0, Actor: AuthorityCode,
		ID: "task", Kind: NodeTask, Title: "Task", Priority: 1, Metadata: EmptyJSONObject(),
	})
	_, err := ledger.Apply(withTestCommandID(t, AssignNodeStepCommand{
		ExpectedVersion: 1, Actor: AuthorityToolEvidence, NodeID: "task", StepID: 4,
	}))
	if !errors.Is(err, ErrAuthorityDenied) {
		t.Fatalf("non-code assignment error=%v", err)
	}

	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 1, Actor: AuthorityCode,
		ID: "goal", Kind: NodeGoal, Title: "Goal", Priority: 1, Metadata: EmptyJSONObject(),
	})
	_, err = ledger.Apply(withTestCommandID(t, AssignNodeStepCommand{
		ExpectedVersion: 2, Actor: AuthorityCode, NodeID: "goal", StepID: 5,
	}))
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("goal assignment error=%v", err)
	}
}
