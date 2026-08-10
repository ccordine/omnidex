package taskstate

import (
	"errors"
	"testing"
)

func TestDispositionAuthorityMaterializesAndRestores(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 0, Actor: AuthorityUser, ID: "user-note",
		Kind: EntryNote, Content: "User note.", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, RejectEntryCommand{
		ExpectedVersion: 1, Actor: AuthorityUser, EntryID: "user-note",
		Reason: "The user withdrew it.",
	})
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 2, Actor: AuthorityModelProposal, ID: "question",
		Kind: EntryQuestion, Content: "Is proof available?", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, ResolveEntryCommand{
		ExpectedVersion: 3, Actor: AuthorityCode, EntryID: "question",
		Reason: "The proof answered it.", Refs: testVerificationRefs(),
	})
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 4, Actor: AuthorityModelProposal, ID: "candidate",
		Kind: EntryDecisionCandidate, Content: "Use the bounded option.", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, AcceptDecisionCommand{
		ExpectedVersion: 5, Actor: AuthorityCode, CandidateID: "candidate",
		AcceptedEntryID: "accepted", AcceptancePolicy: "verified-policy",
		AcceptanceRefs: testVerificationRefs(), Metadata: EmptyJSONObject(),
	})

	restored, err := RestoreLedger(ledger.MaterializedState())
	if err != nil {
		t.Fatal(err)
	}
	assertDispositionBy(t, restored, "user-note", AuthorityUser)
	assertDispositionBy(t, restored, "question", AuthorityCode)
	assertDispositionBy(t, restored, "candidate", AuthorityCode)
	assertDispositionBy(t, restored, "accepted", "")
}

func TestRestoreRejectsForgedDispositionAuthority(t *testing.T) {
	t.Run("active entry has disposition actor", func(t *testing.T) {
		ledger := newTestLedger(t)
		applyTestCommand(t, ledger, AddEntryCommand{
			ExpectedVersion: 0, Actor: AuthorityCode, ID: "active", Kind: EntryNote,
			Content: "Active.", Metadata: EmptyJSONObject(),
		})
		state := ledger.MaterializedState()
		state.Entries[0].DispositionBy = AuthorityCode
		assertRestoreInvalid(t, state)
	})
	t.Run("inactive entry lacks disposition actor", func(t *testing.T) {
		state := rejectedUserState(t)
		state.Entries[0].DispositionBy = ""
		assertRestoreInvalid(t, state)
	})
	t.Run("code rejected user authority", func(t *testing.T) {
		state := rejectedUserState(t)
		state.Entries[0].DispositionBy = AuthorityCode
		assertRestoreAuthorityInvalid(t, state)
	})
	t.Run("unregistered rejection actor", func(t *testing.T) {
		state := rejectedUserState(t)
		state.Entries[0].DispositionBy = AuthorityToolEvidence
		assertRestoreAuthorityInvalid(t, state)
	})
	t.Run("user resolved entry", func(t *testing.T) {
		ledger := newTestLedger(t)
		applyTestCommand(t, ledger, AddEntryCommand{
			ExpectedVersion: 0, Actor: AuthorityCode, ID: "note", Kind: EntryNote,
			Content: "Resolve.", Metadata: EmptyJSONObject(),
		})
		applyTestCommand(t, ledger, ResolveEntryCommand{
			ExpectedVersion: 1, Actor: AuthorityCode, EntryID: "note",
			Reason: "Evidence resolved it.", Refs: testVerificationRefs(),
		})
		state := ledger.MaterializedState()
		state.Entries[0].DispositionBy = AuthorityUser
		assertRestoreAuthorityInvalid(t, state)
	})
	t.Run("code superseded user authority", func(t *testing.T) {
		state := supersededUserState(t)
		stateEntry(t, &state, "old").DispositionBy = AuthorityCode
		assertRestoreAuthorityInvalid(t, state)
	})
	t.Run("candidate actor differs from acceptance actor", func(t *testing.T) {
		state := decisionReplacementState(t)
		stateEntry(t, &state, "candidate-1").DispositionBy = AuthorityUser
		assertRestoreInvalid(t, state)
	})
}

func TestProjectedActiveEntryRejectsDispositionActor(t *testing.T) {
	ledger := newTestLedger(t)
	event := applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 0, Actor: AuthorityCode, ID: "note",
		Kind: EntryNote, Content: "Note.", Metadata: EmptyJSONObject(),
	})
	event.Entry.DispositionBy = AuthorityCode
	if err := ValidateEventProjection(event); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("active projected disposition actor error=%v", err)
	}
}

func rejectedUserState(t *testing.T) MaterializedState {
	t.Helper()
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 0, Actor: AuthorityUser, ID: "user-note",
		Kind: EntryNote, Content: "User note.", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, RejectEntryCommand{
		ExpectedVersion: 1, Actor: AuthorityUser, EntryID: "user-note",
		Reason: "The user withdrew it.",
	})
	return ledger.MaterializedState()
}

func supersededUserState(t *testing.T) MaterializedState {
	t.Helper()
	ledger := newTestLedger(t)
	for index, id := range []EntryID{"old", "new"} {
		applyTestCommand(t, ledger, AddEntryCommand{
			ExpectedVersion: uint64(index), Actor: AuthorityUser, ID: id,
			Kind: EntryNote, Content: string(id) + " user note.", Metadata: EmptyJSONObject(),
		})
	}
	applyTestCommand(t, ledger, SupersedeEntryCommand{
		ExpectedVersion: 2, Actor: AuthorityUser, EntryID: "old", ReplacementID: "new",
		Reason: "The user replaced it.",
	})
	return ledger.MaterializedState()
}

func assertDispositionBy(t *testing.T, ledger *Ledger, id EntryID, want Authority) {
	t.Helper()
	entry, ok := ledger.Entry(id)
	if !ok || entry.DispositionBy != want {
		t.Fatalf("entry %q disposition actor=%q, want %q", id, entry.DispositionBy, want)
	}
}

func assertRestoreAuthorityInvalid(t *testing.T, state MaterializedState) {
	t.Helper()
	_, err := RestoreLedger(state)
	if !errors.Is(err, ErrInvalidState) || !errors.Is(err, ErrAuthorityDenied) {
		t.Fatalf("restore authority error=%v", err)
	}
}
