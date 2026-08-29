package taskstate

import (
	"errors"
	"testing"
)

func TestValidateEventProjectionAcceptsGeneratedEvents(t *testing.T) {
	ledger := newTestLedger(t)
	events := []Event{
		applyTestCommand(t, ledger, AddNodeCommand{
			ExpectedVersion: 0, Actor: AuthorityCode, ID: "task",
			Kind: NodeTask, Title: "Task", Priority: 10, Metadata: EmptyJSONObject(),
		}),
		applyTestCommand(t, ledger, AddNodeCommand{
			ExpectedVersion: 1, Actor: AuthorityCode, ID: "check",
			Kind: NodeCheckpoint, Title: "Check", Priority: 5, Metadata: EmptyJSONObject(),
		}),
		applyTestCommand(t, ledger, AddEdgeCommand{
			ExpectedVersion: 2, Actor: AuthorityCode, ID: "check-after-task",
			Kind: EdgeDependsOn, From: "check", To: "task",
		}),
		applyTestCommand(t, ledger, AddEntryCommand{
			ExpectedVersion: 3, Actor: AuthorityCode, ID: "question",
			Kind: EntryQuestion, Content: "Is the proof available?", Metadata: EmptyJSONObject(),
		}),
		applyTestCommand(t, ledger, ResolveEntryCommand{
			ExpectedVersion: 4, Actor: AuthorityCode, EntryID: "question",
			Reason: "The verifier produced exact evidence.", Refs: testVerificationRefs(),
		}),
		applyTestCommand(t, ledger, PromoteReadyNodesCommand{ExpectedVersion: 5, Actor: AuthorityCode}),
		applyTestCommand(t, ledger, AssignNodeStepCommand{
			ExpectedVersion: 6, Actor: AuthorityCode, NodeID: "task", StepID: 81,
		}),
		applyTestCommand(t, ledger, TransitionNodeCommand{
			ExpectedVersion: 7, Actor: AuthorityCode, NodeID: "task", To: NodeActive,
		}),
		applyTestCommand(t, ledger, TransitionNodeCommand{
			ExpectedVersion: 8, Actor: AuthorityCode, NodeID: "task", To: NodeFailed,
			Reason: "The authoritative verification command failed.",
		}),
	}

	failedLedger := newTestLedger(t)
	events = append(events, applyTestCommand(t, failedLedger, CloseLedgerCommand{
		ExpectedVersion: 0, Actor: AuthorityCode, Status: LedgerFailed,
		Reason: "The owning job failed.",
	}))
	for index, event := range events {
		if err := ValidateEventProjection(event); err != nil {
			t.Fatalf("generated event %d (%s) rejected: %v", index, event.Kind, err)
		}
	}
}

func TestValidateEventProjectionRejectsInvalidCompletionAndClosure(t *testing.T) {
	base := Event{
		LedgerID:      LedgerID("ledger_" + repeatHex("a")),
		Version:       1,
		CommandID:     CommandID("command_" + repeatHex("b")),
		CommandSHA256: repeatHex("c"),
		CommandKind:   CommandTransitionNode,
		Kind:          EventNodeTransitioned,
		Authority:     AuthorityCode,
		NodeID:        "task",
		FromStatus:    NodeActive,
		ToStatus:      NodeDone,
	}
	if err := ValidateEventProjection(base); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("completion without step/evidence error=%v", err)
	}
	stepID := int64(9)
	base.StepID = &stepID
	base.VerificationRefs = testVerificationRefs()
	if err := ValidateEventProjection(base); err != nil {
		t.Fatalf("valid completion rejected: %v", err)
	}

	closed := base
	closed.CommandKind = CommandCloseLedger
	closed.Kind = EventLedgerClosed
	closed.NodeID, closed.FromStatus, closed.ToStatus = "", "", ""
	closed.VerificationRefs = nil
	closed.LedgerStatus = LedgerActive
	closed.Reason = "Closed incorrectly."
	if err := ValidateEventProjection(closed); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("nonterminal close error=%v", err)
	}
	closed.LedgerStatus = LedgerClosed
	if err := ValidateEventProjection(closed); err != nil {
		t.Fatalf("terminal close rejected: %v", err)
	}
}

func TestValidateEventProjectionRejectsKindMismatchAndExtraProjection(t *testing.T) {
	ledger := newTestLedger(t)
	event := applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 0, Actor: AuthorityCode, ID: "task",
		Kind: NodeTask, Title: "Task", Priority: 1, Metadata: EmptyJSONObject(),
	})
	event.CommandKind = CommandAddEntry
	if err := ValidateEventProjection(event); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("command/event mismatch error=%v", err)
	}
	event.CommandKind = CommandAddNode
	event.EntryID = "hidden-extra-state"
	if err := ValidateEventProjection(event); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("extra event projection error=%v", err)
	}
}

func TestReconstructDoesNotNormalizeMissingProjectionArrays(t *testing.T) {
	t.Run("node verification references", func(t *testing.T) {
		ledger := newTestLedger(t)
		event := applyTestCommand(t, ledger, AddNodeCommand{
			ExpectedVersion: 0, Actor: AuthorityCode, ID: "task",
			Kind: NodeTask, Title: "Task", Priority: 1, Metadata: EmptyJSONObject(),
		})
		event.Node.VerificationRefs = nil
		_, err := Reconstruct(ledger.ID(), ledger.Owner(), []Event{event})
		if !errors.Is(err, ErrInvalidState) {
			t.Fatalf("nil node verification array error=%v", err)
		}
	})
	t.Run("node acceptance criteria", func(t *testing.T) {
		ledger := newTestLedger(t)
		event := applyTestCommand(t, ledger, AddNodeCommand{
			ExpectedVersion: 0, Actor: AuthorityCode, ID: "task",
			Kind: NodeTask, Title: "Task", Priority: 1, Metadata: EmptyJSONObject(),
		})
		event.Node.AcceptanceCriteria = nil
		_, err := Reconstruct(ledger.ID(), ledger.Owner(), []Event{event})
		if !errors.Is(err, ErrInvalidState) {
			t.Fatalf("nil node acceptance array error=%v", err)
		}
	})
	t.Run("entry references", func(t *testing.T) {
		ledger := newTestLedger(t)
		event := applyTestCommand(t, ledger, AddEntryCommand{
			ExpectedVersion: 0, Actor: AuthorityCode, ID: "note",
			Kind: EntryNote, Content: "Note.", Metadata: EmptyJSONObject(),
		})
		event.Entry.Refs = nil
		_, err := Reconstruct(ledger.ID(), ledger.Owner(), []Event{event})
		if !errors.Is(err, ErrInvalidState) {
			t.Fatalf("nil entry reference array error=%v", err)
		}
	})
}

func repeatHex(value string) string {
	result := ""
	for len(result) < 64 {
		result += value
	}
	return result[:64]
}
