package taskstate

import (
	"errors"
	"testing"
)

func TestCodeRejectsHypothesisOnlyWithPersistedContradictionEvidence(t *testing.T) {
	t.Parallel()
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 0, Actor: AuthorityCode,
		ID: "hypothesis-1", Kind: EntryHypothesis, Content: "The first mechanism is available.",
		Metadata: EmptyJSONObject(), Refs: []Ref{},
	})
	withoutEvidence := withTestCommandID(t, RejectEntryCommand{
		ExpectedVersion: 1, Actor: AuthorityCode, EntryID: "hypothesis-1",
		Reason: "Current evidence contradicts the hypothesis.", Refs: []Ref{},
	})
	if _, err := ledger.Apply(withoutEvidence); !errors.Is(err, ErrEvidenceRequired) {
		t.Fatalf("missing contradiction error = %v, want ErrEvidenceRequired", err)
	}
	if ledger.Version() != 1 {
		t.Fatalf("failed revision changed ledger version to %d", ledger.Version())
	}

	contradiction := Ref{
		URI: "observation:episode-1/observation-9", Version: "3",
		Hash:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Relation: RefContradicts,
	}
	applyTestCommand(t, ledger, RejectEntryCommand{
		ExpectedVersion: 1, Actor: AuthorityCode, EntryID: "hypothesis-1",
		Reason: "Current evidence contradicts the hypothesis.", Refs: []Ref{contradiction},
	})
	state := ledger.MaterializedState()
	entry := state.Entries[0]
	if entry.Status != EntryRejected || entry.DispositionBy != AuthorityCode ||
		len(entry.Refs) != 1 || entry.Refs[0] != contradiction {
		t.Fatalf("rejected hypothesis did not retain exact contradiction: %+v", entry)
	}
	if _, err := RestoreLedger(state); err != nil {
		t.Fatalf("restore evidence-bound rejection: %v", err)
	}

	state.Entries[0].Refs = []Ref{}
	if _, err := RestoreLedger(state); !errors.Is(err, ErrEvidenceRequired) {
		t.Fatalf("missing restored contradiction error = %v, want ErrEvidenceRequired", err)
	}
}
