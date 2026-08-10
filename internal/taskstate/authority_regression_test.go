package taskstate

import (
	"errors"
	"testing"
)

func TestZeroLedgerCannotBeActivatedWithoutConstructor(t *testing.T) {
	var ledger Ledger
	_, err := ledger.Apply(withTestCommandID(t, AddNodeCommand{
		ExpectedVersion: 0,
		Actor:           AuthorityCode,
		ID:              "must-not-panic",
		Kind:            NodeTask,
		Title:           "Must not panic",
		Priority:        1,
		Metadata:        EmptyJSONObject(),
	}))
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("zero ledger apply error=%v", err)
	}
}

func TestCodeCannotRejectUserAuthority(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 0,
		Actor:           AuthorityUser,
		ID:              "user-constraint",
		Kind:            EntryConstraint,
		Content:         "Preserve the user boundary.",
		Metadata:        EmptyJSONObject(),
	})

	_, err := ledger.Apply(withTestCommandID(t, RejectEntryCommand{
		ExpectedVersion: 1,
		Actor:           AuthorityCode,
		EntryID:         "user-constraint",
		Reason:          "Code cannot erase direct user authority.",
	}))
	if !errors.Is(err, ErrAuthorityDenied) {
		t.Fatalf("code rejection of user authority error=%v", err)
	}
	entry, _ := ledger.Entry("user-constraint")
	if ledger.Version() != 1 || entry.Status != EntryActive {
		t.Fatalf("rejected authority violation mutated ledger: version=%d entry=%+v", ledger.Version(), entry)
	}

	applyTestCommand(t, ledger, RejectEntryCommand{
		ExpectedVersion: 1,
		Actor:           AuthorityUser,
		EntryID:         "user-constraint",
		Reason:          "The user withdrew the constraint.",
	})
}

func TestReconstructRejectsCodeAuthoredUserRejection(t *testing.T) {
	ledger := newTestLedger(t)
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 0, Actor: AuthorityUser, ID: "user-constraint",
		Kind: EntryConstraint, Content: "Preserve direct authority.", Metadata: EmptyJSONObject(),
	})
	applyTestCommand(t, ledger, RejectEntryCommand{
		ExpectedVersion: 1, Actor: AuthorityUser, EntryID: "user-constraint",
		Reason: "The user withdrew it.",
	})
	events := ledger.Events()
	events[1].Authority = AuthorityCode
	_, err := Reconstruct(ledger.ID(), ledger.Owner(), events)
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("code-authored user rejection replay error=%v", err)
	}
}

func TestDecisionAcceptanceRequiresIndependentEvidence(t *testing.T) {
	ledger := newTestLedger(t)
	sourceRef := testVerificationRefs()[0]
	sourceRef.Relation = RefSource
	applyTestCommand(t, ledger, AddEntryCommand{
		ExpectedVersion: 0,
		Actor:           AuthorityModelProposal,
		ID:              "candidate",
		Kind:            EntryDecisionCandidate,
		Content:         "Use the bounded implementation.",
		Metadata:        EmptyJSONObject(),
		Refs:            []Ref{sourceRef},
	})

	for _, refs := range [][]Ref{nil, []Ref{sourceRef}} {
		_, err := ledger.Apply(withTestCommandID(t, AcceptDecisionCommand{
			ExpectedVersion:  ledger.Version(),
			Actor:            AuthorityCode,
			CandidateID:      "candidate",
			AcceptedEntryID:  "accepted",
			AcceptancePolicy: "policy-v1",
			AcceptanceRefs:   refs,
			Metadata:         EmptyJSONObject(),
		}))
		if !errors.Is(err, ErrEvidenceRequired) {
			t.Fatalf("acceptance refs=%+v error=%v", refs, err)
		}
	}

	acceptanceRefs := testVerificationRefs()
	applyTestCommand(t, ledger, AcceptDecisionCommand{
		ExpectedVersion:  ledger.Version(),
		Actor:            AuthorityCode,
		CandidateID:      "candidate",
		AcceptedEntryID:  "accepted",
		AcceptancePolicy: "policy-v1",
		AcceptanceRefs:   acceptanceRefs,
		Metadata:         EmptyJSONObject(),
	})
	accepted, _ := ledger.Entry("accepted")
	candidate, _ := ledger.Entry("candidate")
	if accepted.SupersedesID != "" || accepted.Provenance.SourceEntryID != candidate.ID ||
		len(accepted.Refs) != 1 || accepted.Refs[0] != acceptanceRefs[0] ||
		candidate.SupersededBy != accepted.ID {
		t.Fatalf("accepted decision did not separate lineage and evidence: accepted=%+v candidate=%+v", accepted, candidate)
	}
}
