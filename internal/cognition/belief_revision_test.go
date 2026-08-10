package cognition

import (
	"errors"
	"testing"
)

func TestBeliefRevisionIsOneEvidenceBoundProposalWithoutMutationAuthority(t *testing.T) {
	t.Parallel()
	snapshot, schema, evidence := testRuntimeSnapshot(t)
	proposal := BeliefRevisionProposal{
		TargetRef: EpistemicRef{
			URI: "task:ledger/ledger-7/entry/hypothesis-1", Version: "4", SHA256: testDigest,
		},
		EvidenceRefs: []EvidenceRef{evidence},
	}
	decision := testPolicyDecision(snapshot, evidence)
	decision.Proposals = []LedgerProposal{{Kind: ProposalRevision, Revision: &proposal}}
	if err := decision.Validate(schema); err != nil {
		t.Fatalf("validate revision decision: %v", err)
	}
	cloned := decision.Clone()
	cloned.Proposals[0].Revision.EvidenceRefs[0].SHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if decision.Proposals[0].Revision.EvidenceRefs[0] != evidence {
		t.Fatal("revision clone retained caller-owned evidence storage")
	}

	for name, mutate := range map[string]func(*CognitionDecision){
		"mixed proposal": func(value *CognitionDecision) {
			value.Proposals = append(value.Proposals, LedgerProposal{Kind: ProposalQuestion, Content: "Still unresolved?"})
		},
		"missing payload": func(value *CognitionDecision) { value.Proposals[0].Revision = nil },
		"free text":       func(value *CognitionDecision) { value.Proposals[0].Content = "Reject it." },
		"unknown evidence": func(value *CognitionDecision) {
			value.Proposals[0].Revision.EvidenceRefs[0].ObservationID = "observation-other"
		},
		"invalid target": func(value *CognitionDecision) {
			value.Proposals[0].Revision.TargetRef.URI = "bad target"
		},
	} {
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			candidate := decision.Clone()
			mutate(&candidate)
			if err := candidate.Validate(schema); !errors.Is(err, ErrInvalidDecision) {
				t.Fatalf("error = %v, want ErrInvalidDecision", err)
			}
		})
	}
}
