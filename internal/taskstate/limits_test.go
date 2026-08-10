package taskstate

import (
	"errors"
	"fmt"
	"testing"
)

func TestRestoreRejectsAggregateCapPlusOne(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		state MaterializedState
	}{
		{name: "nodes", state: MaterializedState{Nodes: make([]Node, MaxLedgerNodes+1)}},
		{name: "edges", state: MaterializedState{Edges: make([]Edge, MaxLedgerEdges+1)}},
		{name: "entries", state: MaterializedState{Entries: make([]Entry, MaxLedgerEntries+1)}},
		{name: "node refs", state: MaterializedState{Nodes: []Node{{
			VerificationRefs: make([]Ref, MaxLedgerNodeVerificationRefs+1),
		}}}},
		{name: "entry refs", state: MaterializedState{Entries: []Entry{{
			Refs: make([]Ref, MaxLedgerEntryRefs+1),
		}}}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateMaterializedState(test.state); !errors.Is(err, ErrInvalidState) {
				t.Fatalf("aggregate limit error=%v", err)
			}
		})
	}
}

func TestApplyRejectsEntryCapPlusOneWithoutMutation(t *testing.T) {
	ledger := ledgerAtEntryLimit(t)
	before := ledger.MaterializedState()
	_, err := ledger.Apply(withTestCommandID(t, AddEntryCommand{
		ExpectedVersion: ledger.Version(), Actor: AuthorityCode,
		ID: "over-limit", Kind: EntryNote, Content: "Must fail.",
		Metadata: EmptyJSONObject(),
	}))
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("entry cap error=%v", err)
	}
	after := ledger.MaterializedState()
	if after.Version != before.Version || len(after.Entries) != len(before.Entries) {
		t.Fatalf("rejected cap+1 command mutated state: before=%d/%d after=%d/%d",
			before.Version, len(before.Entries), after.Version, len(after.Entries))
	}
}

func ledgerAtEntryLimit(t *testing.T) *Ledger {
	t.Helper()
	owner := LedgerOwner{Kind: OwnerJob, JobID: 932, RunID: "123e4567-e89b-12d3-a456-426614174999"}
	id, err := NewLedgerID(owner)
	if err != nil {
		t.Fatal(err)
	}
	entries := make([]Entry, MaxLedgerEntries)
	for index := range entries {
		content := fmt.Sprintf("Bounded entry %d.", index)
		entries[index] = Entry{
			ID: EntryID(fmt.Sprintf("entry-%d", index)), Kind: EntryNote,
			Status: EntryActive, Authority: AuthorityCode, CreatedBy: AuthorityCode,
			Content: content, ContentSHA256: contentDigest(content), Metadata: EmptyJSONObject(),
			Refs: make([]Ref, 0), CreatedVersion: 1, UpdatedVersion: 1,
		}
	}
	ledger, err := RestoreLedger(MaterializedState{
		ID: id, Owner: owner, Version: 1, Status: LedgerActive,
		Nodes: make([]Node, 0), Edges: make([]Edge, 0), Entries: entries,
		NodeSupersessions: make([]NodeGenerationSupersession, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	return ledger
}
