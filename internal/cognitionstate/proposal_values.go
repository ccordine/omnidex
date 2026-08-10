package cognitionstate

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func proposalEntryContent(
	proposal cognition.LedgerProposal,
) (string, []cognition.EvidenceRef, error) {
	if proposal.Kind != cognition.ProposalObligation && proposal.Kind != cognition.ProposalPlanRevision {
		return proposal.Content, append([]cognition.EvidenceRef(nil), proposal.EvidenceRefs...), nil
	}
	if proposal.Kind == cognition.ProposalObligation && proposal.Obligation == nil {
		return "", nil, fmt.Errorf("typed obligation payload is required")
	}
	if proposal.Kind == cognition.ProposalPlanRevision && proposal.PlanRevision == nil {
		return "", nil, fmt.Errorf("typed plan revision payload is required")
	}
	if proposal.Obligation != nil {
		raw, err := json.Marshal(struct {
			Desired cognition.GoalExpression `json:"desired"`
		}{Desired: proposal.Obligation.Desired})
		if err != nil {
			return "", nil, fmt.Errorf("encode obligation semantics: %w", err)
		}
		return string(raw), append([]cognition.EvidenceRef(nil), proposal.Obligation.EvidenceRefs...), nil
	}
	raw, err := json.Marshal(struct {
		Next cognition.GoalExpression `json:"next"`
	}{Next: proposal.PlanRevision.Next})
	if err != nil {
		return "", nil, fmt.Errorf("encode plan revision semantics: %w", err)
	}
	return string(raw), append([]cognition.EvidenceRef(nil), proposal.PlanRevision.EvidenceRefs...), nil
}
