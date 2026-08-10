package cognition

import (
	"fmt"
	"sort"
)

type ObligationProposal struct {
	Desired      GoalExpression `json:"desired"`
	EvidenceRefs []EvidenceRef  `json:"evidence_refs"`
}

func (proposal ObligationProposal) Validate() error {
	if err := proposal.Desired.Validate(); err != nil {
		return fmt.Errorf("%w: desired goal: %v", ErrInvalidObligationProposal, err)
	}
	if len(proposal.EvidenceRefs) == 0 {
		return fmt.Errorf("%w: supporting evidence is required", ErrInvalidObligationProposal)
	}
	if err := validateEvidenceRefs(proposal.EvidenceRefs); err != nil {
		return fmt.Errorf("%w: supporting evidence: %v", ErrInvalidObligationProposal, err)
	}
	return nil
}

func (proposal ObligationProposal) Clone() ObligationProposal {
	return ObligationProposal{
		Desired:      proposal.Desired.Clone(),
		EvidenceRefs: cloneSlice(proposal.EvidenceRefs),
	}
}

func canonicalObligationProposal(proposal ObligationProposal) (ObligationProposal, error) {
	if err := proposal.Validate(); err != nil {
		return ObligationProposal{}, err
	}
	desired, err := canonicalGoal(proposal.Desired)
	if err != nil {
		return ObligationProposal{}, fmt.Errorf("%w: %v", ErrInvalidObligationProposal, err)
	}
	refs := cloneSlice(proposal.EvidenceRefs)
	sort.Slice(refs, func(left, right int) bool {
		return evidenceIdentity(refs[left]) < evidenceIdentity(refs[right])
	})
	return ObligationProposal{Desired: desired, EvidenceRefs: refs}, nil
}

func obligationProposalSHA256(proposal ObligationProposal) (string, error) {
	canonical, err := canonicalObligationProposal(proposal)
	if err != nil {
		return "", err
	}
	return cognitionValueSHA256(struct {
		Schema   string
		Proposal ObligationProposal
	}{ObligationMaterializationSchemaV1, canonical})
}
