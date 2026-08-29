package taskstate

import (
	"errors"
	"testing"
)

func TestTerminalFailureRequiresCodeReasonAndVerifyingProof(t *testing.T) {
	for _, status := range []NodeStatus{NodePending, NodeReady, NodeActive, NodeBlocked} {
		t.Run(string(status), func(t *testing.T) {
			ledger := terminalFailureLedger(t, status)
			command := TerminalFailNodeCommand{
				ExpectedVersion: ledger.Version(), Actor: AuthorityCode, NodeID: "node-1",
				Reason: "The exact terminal environment state ended this obligation.",
				Proof:  testVerificationRefs()[0],
			}
			event := applyTestCommand(t, ledger, command)
			if event.FromStatus != status || event.ToStatus != NodeFailed ||
				len(event.VerificationRefs) != 1 || event.Reason != command.Reason {
				t.Fatalf("terminal failure event=%+v", event)
			}
			node, exists := ledger.Node("node-1")
			if !exists || len(node.VerificationRefs) != 1 || node.VerificationRefs[0] != command.Proof {
				t.Fatalf("terminal failure materialization=%+v", node)
			}
			if err := ValidateMaterializedState(ledger.MaterializedState()); err != nil {
				t.Fatalf("validate terminal failure materialization: %v", err)
			}
		})
	}

	base := TerminalFailNodeCommand{
		Actor: AuthorityCode, NodeID: "node-1", Reason: "terminal proof", Proof: testVerificationRefs()[0],
	}
	for name, mutate := range map[string]func(*TerminalFailNodeCommand){
		"tool authority": func(value *TerminalFailNodeCommand) { value.Actor = AuthorityToolEvidence },
		"user authority": func(value *TerminalFailNodeCommand) { value.Actor = AuthorityUser },
		"missing reason": func(value *TerminalFailNodeCommand) { value.Reason = "" },
		"wrong proof":    func(value *TerminalFailNodeCommand) { value.Proof.Relation = RefConcerns },
	} {
		t.Run(name, func(t *testing.T) {
			ledger := terminalFailureLedger(t, NodePending)
			command := base
			command.ExpectedVersion = ledger.Version()
			mutate(&command)
			if _, err := ledger.Apply(withTestCommandID(t, command)); err == nil {
				t.Fatal("invalid terminal failure was accepted")
			}
		})
	}
}

func TestOrdinaryTransitionCannotImitateTerminalFailure(t *testing.T) {
	ledger := terminalFailureLedger(t, NodeBlocked)
	_, err := ledger.Apply(withTestCommandID(t, TransitionNodeCommand{
		ExpectedVersion: ledger.Version(), Actor: AuthorityCode,
		NodeID: "node-1", To: NodeFailed, Reason: "guessed",
	}))
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("ordinary blocked-to-failed error=%v", err)
	}
}

func terminalFailureLedger(t *testing.T, status NodeStatus) *Ledger {
	t.Helper()
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 0, Actor: AuthorityCode, ID: "node-1", Kind: NodeObjective,
		Title: "node", Priority: 50, AcceptanceCriteria: []string{}, Metadata: EmptyJSONObject(),
	})
	node := ledger.nodes["node-1"]
	node.Status = status
	ledger.nodes[node.ID] = node
	return ledger
}
