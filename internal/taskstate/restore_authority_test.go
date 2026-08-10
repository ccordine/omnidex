package taskstate

import (
	"errors"
	"testing"
)

func TestRestoreKeepsCandidateAndGenericDecisionLineageSeparate(t *testing.T) {
	state := decisionReplacementState(t)
	restored, err := RestoreLedger(state)
	if err != nil {
		t.Fatal(err)
	}
	candidateOne, _ := restored.Entry("candidate-1")
	acceptedOne, _ := restored.Entry("accepted-1")
	candidateTwo, _ := restored.Entry("candidate-2")
	acceptedTwo, _ := restored.Entry("accepted-2")
	if candidateOne.SupersededBy != acceptedOne.ID || candidateTwo.SupersededBy != acceptedTwo.ID {
		t.Fatalf("candidate lineage was not derived: first=%+v second=%+v", candidateOne, candidateTwo)
	}
	if acceptedOne.SupersededBy != acceptedTwo.ID || acceptedTwo.SupersedesID != acceptedOne.ID ||
		acceptedTwo.Provenance.SourceEntryID != candidateTwo.ID {
		t.Fatalf("generic and candidate lineage were conflated: first=%+v second=%+v", acceptedOne, acceptedTwo)
	}
}

func TestRestoreRejectsAcceptedDecisionTampering(t *testing.T) {
	tests := []struct {
		name   string
		tamper func(*Entry)
	}{
		{name: "content", tamper: func(entry *Entry) {
			entry.Content = "Changed after acceptance."
			entry.ContentSHA256 = contentDigest(entry.Content)
		}},
		{name: "scope", tamper: func(entry *Entry) { entry.ScopeNodeID = "scope-b" }},
		{name: "confidence", tamper: func(entry *Entry) {
			changed := 0.2
			entry.Confidence = &changed
		}},
		{name: "candidate stored as generic supersession", tamper: func(entry *Entry) {
			entry.SupersedesID = entry.Provenance.SourceEntryID
		}},
		{name: "acceptance evidence removed", tamper: func(entry *Entry) { entry.Refs = []Ref{} }},
		{name: "acceptance policy changed", tamper: func(entry *Entry) {
			entry.Provenance.AcceptancePolicy = "changed-policy"
		}},
		{name: "candidate and acceptance share creation version", tamper: func(entry *Entry) {
			entry.CreatedVersion--
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			state := decisionReplacementState(t)
			entry := stateEntry(t, &state, "accepted-1")
			test.tamper(entry)
			assertRestoreInvalid(t, state)
		})
	}
}

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

func decisionReplacementState(t *testing.T) MaterializedState {
	t.Helper()
	ledger := newTestLedger(t)
	for index, id := range []NodeID{"scope-a", "scope-b"} {
		applyTestCommand(t, ledger, AddNodeCommand{
			ExpectedVersion: uint64(index), Actor: AuthorityCode, ID: id,
			Kind: NodeObjective, Title: string(id), Priority: 1,
			Metadata: EmptyJSONObject(),
		})
	}
	confidence := 0.7
	for index := 1; index <= 2; index++ {
		candidateID := EntryID("candidate-" + string(rune('0'+index)))
		acceptedID := EntryID("accepted-" + string(rune('0'+index)))
		applyTestCommand(t, ledger, AddEntryCommand{
			ExpectedVersion: ledger.Version(), Actor: AuthorityModelProposal,
			ID: candidateID, ScopeNodeID: "scope-a", Kind: EntryDecisionCandidate,
			Content:    "Use bounded option " + string(rune('A'-1+index)) + ".",
			Confidence: &confidence, Metadata: EmptyJSONObject(),
		})
		applyTestCommand(t, ledger, AcceptDecisionCommand{
			ExpectedVersion: ledger.Version(), Actor: AuthorityCode,
			CandidateID: candidateID, AcceptedEntryID: acceptedID,
			AcceptancePolicy: "verified-policy", AcceptanceRefs: testVerificationRefs(),
			Metadata: EmptyJSONObject(),
		})
	}
	applyTestCommand(t, ledger, SupersedeEntryCommand{
		ExpectedVersion: ledger.Version(), Actor: AuthorityCode,
		EntryID: "accepted-1", ReplacementID: "accepted-2",
		Reason: "The later accepted decision supersedes the earlier one.",
	})
	state := ledger.MaterializedState()
	for index := range state.Entries {
		state.Entries[index].SupersededBy = ""
	}
	return state
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
