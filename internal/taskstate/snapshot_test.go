package taskstate

import (
	"errors"
	"testing"
)

func TestReadAPIsReturnDeepCopiesAndRestoreLoadsCurrentState(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddNodeCommand{
		ExpectedVersion: 0, Actor: AuthorityCode, ID: "node", Kind: NodeTask,
		Title: "Original", Priority: 1, AcceptanceCriteria: []string{"criterion"},
		Metadata: mustJSONObject(t, `{"nested":{"value":1}}`),
	})
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 1, Actor: AuthorityCode, ID: "fact", ScopeNodeID: "node",
		Kind: EntryFact, Content: "Grounded.", Metadata: EmptyJSONObject(), Refs: testVerificationRefs(),
	})

	node, _ := ledger.Node("node")
	node.AcceptanceCriteria[0] = "mutated"
	nodeMetadata := node.Metadata.Bytes()
	nodeMetadata[0] = '['
	entry, _ := ledger.Entry("fact")
	entry.Refs[0].Hash = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	events := ledger.Events()
	events[0].Node.Title = "mutated"
	state := ledger.MaterializedState()
	state.Nodes[0].Title = "mutated"
	state.Entries[0].Refs[0].URI = "evidence://mutated"

	storedNode, _ := ledger.Node("node")
	storedEntry, _ := ledger.Entry("fact")
	if storedNode.Title != "Original" || storedNode.AcceptanceCriteria[0] != "criterion" ||
		storedEntry.Refs[0].URI != "evidence://verification/test" || ledger.Events()[0].Node.Title != "Original" {
		t.Fatal("read snapshot mutated authoritative ledger state")
	}

	clean := ledger.MaterializedState()
	restored, err := RestoreLedger(clean)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Version() != ledger.Version() || restored.Status() != ledger.Status() {
		t.Fatalf("restored ledger mismatch: version=%d status=%q", restored.Version(), restored.Status())
	}
	if len(restored.Events()) != 0 {
		t.Fatal("normalized restore replayed immutable history")
	}
}

func TestReferenceIdentityAndResolutionSetFailLoudly(t *testing.T) {
	ledger := newTestLedger(t)
	badRef := testVerificationRefs()[0]
	badRef.URI = "evidence:"
	_, err := ledger.Apply(withTestCommandID(t, AddEntryCommand{
		ExpectedVersion: 0, Actor: AuthorityCode, ID: "bad", Kind: EntryFact,
		Content: "Bad reference.", Metadata: EmptyJSONObject(), Refs: []Ref{badRef},
	}))
	if !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("empty reference suffix error=%v", err)
	}

	ref := testVerificationRefs()[0]
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 0, Actor: AuthorityCode, ID: "note", Kind: EntryNote,
		Content: "Resolve later.", Metadata: EmptyJSONObject(), Refs: []Ref{ref},
	})
	_, err = ledger.Apply(withTestCommandID(t, ResolveEntryCommand{
		ExpectedVersion: 1, Actor: AuthorityCode, EntryID: "note",
		Reason: "Evidence resolves it.", Refs: []Ref{ref},
	}))
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("duplicate combined reference identity error=%v", err)
	}
	entry, _ := ledger.Entry("note")
	if entry.Status != EntryActive || ledger.Version() != 1 {
		t.Fatal("rejected combined reference set mutated entry")
	}
}

func TestRestoreDerivesSupersededByFromCanonicalReplacementLink(t *testing.T) {
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
		Reason: "New evidence replaced the old note.",
	})
	state := ledger.MaterializedState()
	for index := range state.Entries {
		if state.Entries[index].ID == "old" {
			state.Entries[index].SupersededBy = ""
		}
	}
	restored, err := RestoreLedger(state)
	if err != nil {
		t.Fatal(err)
	}
	old, _ := restored.Entry("old")
	if old.SupersededBy != "new" {
		t.Fatalf("derived superseded-by=%q", old.SupersededBy)
	}
}

func mustJSONObject(t *testing.T, raw string) JSONObject {
	t.Helper()
	object, err := NewJSONObject([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return object
}
