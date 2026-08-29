package taskstate

import (
	"errors"
	"testing"
)

func TestRetiredModelAuthoritiesAndDecisionKindsFailExplicitly(t *testing.T) {
	t.Parallel()
	ledger := newTestLedger(t)
	for _, actor := range []Authority{"model_proposal", "accepted_model_decision"} {
		_, err := ledger.Apply(withTestCommandID(t, AddEntryCommand{
			ExpectedVersion: ledger.Version(), Actor: actor,
			ID: EntryID("forbidden-" + string(actor)), Kind: EntryObservation,
			Content: "retired authority", Metadata: EmptyJSONObject(),
		}))
		if err == nil {
			t.Fatalf("retired authority %q created ledger state", actor)
		}
	}
	for _, kind := range []EntryKind{"decision_candidate", "accepted_decision"} {
		_, err := ledger.Apply(withTestCommandID(t, AddEntryCommand{
			ExpectedVersion: ledger.Version(), Actor: AuthorityCode,
			ID: EntryID("forbidden-" + string(kind)), Kind: kind,
			Content: "retired decision ceremony", Metadata: EmptyJSONObject(),
		}))
		if err == nil {
			t.Fatalf("retired entry kind %q created ledger state", kind)
		}
	}
}

func TestFactRequiresActiveAuthoritativeReference(t *testing.T) {
	ledger := newTestLedger(t)
	_, err := ledger.Apply(withTestCommandID(t, AddEntryCommand{
		ExpectedVersion: 0, Actor: AuthorityCode,
		ID: "unproved", Kind: EntryFact, Content: "Tests passed.", Metadata: EmptyJSONObject(),
	}))
	if !errors.Is(err, ErrEvidenceRequired) {
		t.Fatalf("fact without evidence error=%v", err)
	}

	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 0, Actor: AuthorityCode,
		ID: "proved", Kind: EntryFact, Content: "Tests passed.", Metadata: EmptyJSONObject(), Refs: []Ref{{
			URI: "evidence://test-run/17", Version: "1",
			Hash:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			Relation: RefEvidence,
		}},
	})
}

func TestRejectSupersedeAndResolveRetainInactiveHistory(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 0, Actor: AuthorityCode,
		ID: "note-rejected", Kind: EntryNote, Content: "Rejected note.", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, RejectEntryCommand{
		ExpectedVersion: 1, Actor: AuthorityCode,
		EntryID: "note-rejected", Reason: "The exact evidence superseded this note.",
	})
	rejected, _ := ledger.Entry("note-rejected")
	if rejected.Status != EntryRejected || rejected.Active() {
		t.Fatalf("rejected entry=%+v", rejected)
	}

	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 2, Actor: AuthorityCode,
		ID: "note-old", Kind: EntryNote, Content: "Old note.", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 3, Actor: AuthorityCode,
		ID: "note-new", Kind: EntryNote, Content: "New note.", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, SupersedeEntryCommand{
		ExpectedVersion: 4, Actor: AuthorityCode,
		EntryID: "note-old", ReplacementID: "note-new", Reason: "New evidence replaced it.",
	})
	superseded, _ := ledger.Entry("note-old")
	if superseded.Status != EntrySuperseded || superseded.SupersededBy != "note-new" {
		t.Fatalf("superseded entry=%+v", superseded)
	}

	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 5, Actor: AuthorityCode,
		ID: "question", Kind: EntryQuestion, Content: "Is verification available?", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, ResolveEntryCommand{
		ExpectedVersion: 6, Actor: AuthorityCode, EntryID: "question",
		Reason: "The authoritative health probe answered the question.", Refs: testVerificationRefs(),
	})
	resolved, _ := ledger.Entry("question")
	if resolved.Status != EntryResolved || resolved.Active() {
		t.Fatalf("resolved entry=%+v", resolved)
	}
}
