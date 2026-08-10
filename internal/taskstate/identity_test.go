package taskstate

import (
	"errors"
	"reflect"
	"testing"
)

func TestApplyIsIdempotentByCanonicalCommandIdentity(t *testing.T) {
	ledger := newTestLedger(t)
	id, err := NewCommandID("idempotency", "add node", "one")
	if err != nil {
		t.Fatal(err)
	}
	command := AddNodeCommand{
		CommandID: id, ExpectedVersion: 0, Actor: AuthorityCode,
		ID: "one", Kind: NodeTask, Title: "One", Priority: 1, Metadata: EmptyJSONObject(),
	}
	first, err := ledger.Apply(command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ledger.Apply(command)
	if err != nil {
		t.Fatal(err)
	}
	if ledger.Version() != 1 || len(ledger.Events()) != 1 || !reflect.DeepEqual(first, second) {
		t.Fatalf("idempotent replay mutated ledger: version=%d events=%d", ledger.Version(), len(ledger.Events()))
	}

	command.Title = "Different command under reused identity"
	_, err = ledger.Apply(command)
	if !errors.Is(err, ErrCommandIDConflict) {
		t.Fatalf("reused command identity error=%v", err)
	}
}

func TestPriorityAndLedgerIdentityFailLoudly(t *testing.T) {
	ledger := newTestLedger(t)
	_, err := ledger.Apply(withTestCommandID(t, AddNodeCommand{
		ExpectedVersion: 0, Actor: AuthorityCode,
		ID: "invalid", Kind: NodeTask, Title: "Invalid", Priority: 0, Metadata: EmptyJSONObject(),
	}))
	if !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("zero priority error=%v", err)
	}

	_, err = NewLedger("ledger_deadbeef", ledger.Owner())
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("invalid ledger identity error=%v", err)
	}
}

func TestLedgerClosesOnceByCode(t *testing.T) {
	ledger := newTestLedger(t)
	stepID := int64(90)
	applyTestCommand(t, ledger, CloseLedgerCommand{
		ExpectedVersion: 0, Actor: AuthorityCode, Status: LedgerClosed,
		StepID: &stepID, Reason: "The owning job completed authoritatively.",
	})
	if ledger.Status() != LedgerClosed {
		t.Fatalf("ledger status=%q", ledger.Status())
	}
	_, err := ledger.Apply(withTestCommandID(t, CloseLedgerCommand{
		ExpectedVersion: 1, Actor: AuthorityCode, Status: LedgerFailed, Reason: "Cannot reopen.",
	}))
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("closed ledger transitioned again: %v", err)
	}
}
