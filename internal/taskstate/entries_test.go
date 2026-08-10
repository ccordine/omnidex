package taskstate

import (
	"errors"
	"testing"
)

func TestModelProposalAuthorityIsNarrow(t *testing.T) {
	ledger := newTestLedger(t)
	allowed := []EntryKind{
		EntryObservation, EntryHypothesis, EntryQuestion, EntryDecisionCandidate,
	}
	for index, kind := range allowed {
		applyTestCommand(t, ledger, AddEntryCommand{
			ExpectedVersion: uint64(index), Actor: AuthorityModelProposal,
			ID: EntryID("proposal-" + string(rune('a'+index))), Kind: kind,
			Content: "bounded proposal", Metadata: EmptyJSONObject(),
		})
	}

	for _, kind := range []EntryKind{EntryFact, EntryAcceptedDecision, EntryCheckpoint} {
		before := ledger.Version()
		_, err := ledger.Apply(withTestCommandID(t, AddEntryCommand{
			ExpectedVersion: before, Actor: AuthorityModelProposal,
			ID: EntryID("forbidden-" + string(kind)), Kind: kind,
			Content: "not authoritative", Metadata: EmptyJSONObject(),
		}))
		if !errors.Is(err, ErrAuthorityDenied) {
			t.Fatalf("model entry kind %q error=%v", kind, err)
		}
		if ledger.Version() != before {
			t.Fatalf("rejected model entry %q changed version", kind)
		}
	}

	_, err := ledger.Apply(withTestCommandID(t, AddNodeCommand{
		ExpectedVersion: ledger.Version(), Actor: AuthorityModelProposal,
		ID: "forbidden-node", Kind: NodeTask, Title: "Forbidden", Priority: 1,
		Metadata: EmptyJSONObject(),
	}))
	if !errors.Is(err, ErrAuthorityDenied) {
		t.Fatalf("model created task node: %v", err)
	}
}

func TestModelObservationRemainsNonAuthoritativeAcrossRestore(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 0,
		Actor:           AuthorityModelProposal,
		ID:              "proposed-observation",
		Kind:            EntryObservation,
		Content:         "The bounded policy reports a possible transition.",
		Metadata:        EmptyJSONObject(),
	})

	entry, ok := ledger.Entry("proposed-observation")
	if !ok {
		t.Fatal("model-proposed observation is missing")
	}
	if entry.Authority != AuthorityModelProposal || entry.CreatedBy != AuthorityModelProposal {
		t.Fatalf("model-proposed observation gained authority: %+v", entry)
	}

	restored, err := RestoreLedger(ledger.MaterializedState())
	if err != nil {
		t.Fatalf("restore model-proposed observation: %v", err)
	}
	restoredEntry, ok := restored.Entry(entry.ID)
	if !ok || restoredEntry.Authority != AuthorityModelProposal || restoredEntry.Kind != EntryObservation {
		t.Fatalf("restored model-proposed observation changed semantics: %+v", restoredEntry)
	}

	_, err = restored.Apply(withTestCommandID(t, AcceptDecisionCommand{
		ExpectedVersion:  restored.Version(),
		Actor:            AuthorityCode,
		CandidateID:      entry.ID,
		AcceptedEntryID:  "forbidden-promotion",
		AcceptancePolicy: "not-a-decision",
		AcceptanceRefs:   testVerificationRefs(),
		Metadata:         EmptyJSONObject(),
	}))
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("observation was accepted through decision authority: %v", err)
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

func TestRejectSupersedeAndAcceptRetainInactiveHistory(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 0, Actor: AuthorityModelProposal,
		ID: "candidate-old", Kind: EntryDecisionCandidate, Content: "Use approach A.", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, RejectEntryCommand{
		ExpectedVersion: 1, Actor: AuthorityCode,
		EntryID: "candidate-old", Reason: "Fails the bounded check.",
	})
	old, _ := ledger.Entry("candidate-old")
	if old.Status != EntryRejected || old.Active() {
		t.Fatalf("rejected entry=%+v", old)
	}

	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 2, Actor: AuthorityModelProposal,
		ID: "candidate-new", Kind: EntryDecisionCandidate, Content: "Use approach B.", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, AcceptDecisionCommand{
		ExpectedVersion: 3, Actor: AuthorityCode,
		CandidateID: "candidate-new", AcceptedEntryID: "decision-1",
		AcceptancePolicy: "code_verified_policy_v1", AcceptanceRefs: testVerificationRefs(),
		Metadata: EmptyJSONObject(),
	})

	candidate, _ := ledger.Entry("candidate-new")
	accepted, ok := ledger.Entry("decision-1")
	if !ok {
		t.Fatal("accepted decision is missing")
	}
	if candidate.Status != EntrySuperseded || candidate.SupersededBy != accepted.ID || candidate.Active() {
		t.Fatalf("candidate history=%+v", candidate)
	}
	if candidate.DispositionBy != AuthorityCode || accepted.DispositionBy != "" {
		t.Fatalf("decision disposition authority: candidate=%q accepted=%q", candidate.DispositionBy, accepted.DispositionBy)
	}
	if accepted.Status != EntryActive || accepted.Kind != EntryAcceptedDecision ||
		accepted.Authority != AuthorityAcceptedModelDecision {
		t.Fatalf("accepted entry=%+v", accepted)
	}
	if accepted.Provenance.SourceEntryID != candidate.ID ||
		accepted.Provenance.AcceptancePolicy != "code_verified_policy_v1" ||
		accepted.Provenance.AcceptedBy != AuthorityCode {
		t.Fatalf("acceptance provenance=%+v", accepted.Provenance)
	}
	if accepted.SupersedesID != "" || len(accepted.Refs) != 1 || accepted.Refs[0] != testVerificationRefs()[0] {
		t.Fatalf("accepted decision lineage or evidence=%+v", accepted)
	}

	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 4, Actor: AuthorityCode,
		ID: "note-old", Kind: EntryNote, Content: "Old note.", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 5, Actor: AuthorityCode,
		ID: "note-new", Kind: EntryNote, Content: "New note.", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, SupersedeEntryCommand{
		ExpectedVersion: 6, Actor: AuthorityCode,
		EntryID: "note-old", ReplacementID: "note-new", Reason: "New evidence replaced it.",
	})
	superseded, _ := ledger.Entry("note-old")
	if superseded.Status != EntrySuperseded || superseded.SupersededBy != "note-new" {
		t.Fatalf("superseded entry=%+v", superseded)
	}

	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 7, Actor: AuthorityModelProposal,
		ID: "question", Kind: EntryQuestion, Content: "Is verification available?", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, ResolveEntryCommand{
		ExpectedVersion: 8, Actor: AuthorityCode, EntryID: "question",
		Reason: "The authoritative health probe answered the question.", Refs: testVerificationRefs(),
	})
	resolved, _ := ledger.Entry("question")
	if resolved.Status != EntryResolved || resolved.Active() {
		t.Fatalf("resolved entry=%+v", resolved)
	}
	_, err := ledger.Apply(withTestCommandID(t, ResolveEntryCommand{
		ExpectedVersion: 9, Actor: AuthorityCode, EntryID: "candidate-old",
		Reason: "Rejected history cannot reactivate.", Refs: testVerificationRefs(),
	}))
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("resolved rejected entry error=%v", err)
	}
}
