package taskstate

import (
	"errors"
	"testing"
)

func TestRestoreRejectsInvalidGenericSupersessionAuthorityAndKind(t *testing.T) {
	t.Run("authority downgrade", func(t *testing.T) {
		ledger := newTestLedger(t)
		applyTestCommand(t, ledger, AddEntryCommand{
			ExpectedVersion: 0, Actor: AuthorityUser, ID: "old", Kind: EntryNote,
			Content: "User note.", Metadata: EmptyJSONObject(),
		})
		applyTestCommand(t, ledger, AddEntryCommand{
			ExpectedVersion: 1, Actor: AuthorityUser, ID: "new", Kind: EntryNote,
			Content: "User replacement.", Metadata: EmptyJSONObject(),
		})
		applyTestCommand(t, ledger, SupersedeEntryCommand{
			ExpectedVersion: 2, Actor: AuthorityUser, EntryID: "old", ReplacementID: "new",
			Reason: "The user replaced their note.",
		})
		state := ledger.MaterializedState()
		replacement := stateEntry(t, &state, "new")
		replacement.Authority, replacement.CreatedBy = AuthorityCode, AuthorityCode
		_, err := RestoreLedger(state)
		if !errors.Is(err, ErrInvalidState) || !errors.Is(err, ErrAuthorityDenied) {
			t.Fatalf("authority downgrade error=%v", err)
		}
	})
	t.Run("kind change", func(t *testing.T) {
		ledger := supersededNotesLedger(t)
		state := ledger.MaterializedState()
		stateEntry(t, &state, "new").Kind = EntryObservation
		assertRestoreInvalid(t, state)
	})
}

func TestRestoreRejectsUnreachableResolvedEntry(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 0, Actor: AuthorityCode, ID: "note", Kind: EntryNote,
		Content: "Resolve this note.", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, ResolveEntryCommand{
		ExpectedVersion: 1, Actor: AuthorityCode, EntryID: "note",
		Reason: "Authoritative evidence resolved it.", Refs: testVerificationRefs(),
	})

	t.Run("evidence removed", func(t *testing.T) {
		state := ledger.MaterializedState()
		state.Entries[0].Refs = []Ref{}
		_, err := RestoreLedger(state)
		if !errors.Is(err, ErrInvalidState) || !errors.Is(err, ErrEvidenceRequired) {
			t.Fatalf("resolved evidence error=%v", err)
		}
	})
	t.Run("unresolvable kind", func(t *testing.T) {
		state := ledger.MaterializedState()
		state.Entries[0].Kind = EntryConstraint
		assertRestoreInvalid(t, state)
	})
}

func supersededNotesLedger(t *testing.T) *Ledger {
	t.Helper()
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 0, Actor: AuthorityCode, ID: "old", Kind: EntryNote,
		Content: "Old.", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 1, Actor: AuthorityCode, ID: "new", Kind: EntryNote,
		Content: "New.", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, SupersedeEntryCommand{
		ExpectedVersion: 2, Actor: AuthorityCode, EntryID: "old", ReplacementID: "new",
		Reason: "The replacement is authoritative.",
	})
	return ledger
}

func stateEntry(t *testing.T, state *MaterializedState, id EntryID) *Entry {
	t.Helper()
	for index := range state.Entries {
		if state.Entries[index].ID == id {
			return &state.Entries[index]
		}
	}
	t.Fatalf("state entry %q not found", id)
	return nil
}
