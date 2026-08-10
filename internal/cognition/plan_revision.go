package cognition

import (
	"fmt"
	"sort"
)

// PlanRevisionProposal asks code to replace the open plan with one new
// prerequisite while preserving the exact root goal. It deliberately exposes
// no identity, generation, status, dependency, or completion-check authority.
type PlanRevisionProposal struct {
	Next         GoalExpression `json:"next"`
	EvidenceRefs []EvidenceRef  `json:"evidence_refs"`
}

func (proposal PlanRevisionProposal) Validate() error {
	if err := proposal.Next.Validate(); err != nil {
		return fmt.Errorf("%w: next obligation: %v", ErrInvalidPlanRevision, err)
	}
	if len(proposal.EvidenceRefs) == 0 {
		return fmt.Errorf("%w: supporting evidence is required", ErrInvalidPlanRevision)
	}
	if err := validateEvidenceRefs(proposal.EvidenceRefs); err != nil {
		return fmt.Errorf("%w: supporting evidence: %v", ErrInvalidPlanRevision, err)
	}
	return nil
}

func (proposal PlanRevisionProposal) Clone() PlanRevisionProposal {
	return PlanRevisionProposal{
		Next: proposal.Next.Clone(), EvidenceRefs: cloneSlice(proposal.EvidenceRefs),
	}
}

func canonicalPlanRevisionProposal(proposal PlanRevisionProposal) (PlanRevisionProposal, error) {
	if err := proposal.Validate(); err != nil {
		return PlanRevisionProposal{}, err
	}
	next, err := canonicalGoal(proposal.Next)
	if err != nil {
		return PlanRevisionProposal{}, fmt.Errorf("%w: %v", ErrInvalidPlanRevision, err)
	}
	refs := cloneSlice(proposal.EvidenceRefs)
	sort.Slice(refs, func(left, right int) bool {
		return evidenceIdentity(refs[left]) < evidenceIdentity(refs[right])
	})
	return PlanRevisionProposal{Next: next, EvidenceRefs: refs}, nil
}
