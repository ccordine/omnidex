package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
)

func (client *extendedWitnessPolicyClient) rogueRevisionProposals(
	envelope extendedWitnessEnvelope,
	witness labyrinth.WitnessAction,
	latest []cognition.EvidenceRef,
) ([]cognition.LedgerProposal, []cognition.AttentionRequest, []cognition.EvidenceRef, error) {
	switch client.next {
	case 4:
		if len(latest) == 0 {
			return nil, nil, nil, fmt.Errorf("Rogue tentative evidence is absent")
		}
		return []cognition.LedgerProposal{{
			Kind: cognition.ProposalHypothesis, Content: "The terminal route may be unavailable.",
			EvidenceRefs: latest,
		}}, retainExtendedEvidence(latest), latest, nil
	case 6:
		target, err := extendedHypothesisRef(envelope)
		if err != nil || len(latest) == 0 {
			return nil, nil, nil, fmt.Errorf("Rogue hypothesis or contradiction evidence is absent")
		}
		return []cognition.LedgerProposal{{
			Kind: cognition.ProposalRevision,
			Revision: &cognition.BeliefRevisionProposal{
				TargetRef: target, EvidenceRefs: latest,
			},
		}}, retainExtendedEvidence(latest), latest, nil
	case 7:
		to, ok := actionArgument(witness.Request, "to")
		if !ok || len(latest) == 0 {
			return nil, nil, nil, fmt.Errorf("Rogue revised route target or evidence is absent")
		}
		predicate, err := cognition.NewPredicate("surface.marker", []string{to})
		if err != nil {
			return nil, nil, nil, err
		}
		goal, err := cognition.NewGoalExpression([]cognition.Predicate{predicate}, nil, nil)
		if err != nil {
			return nil, nil, nil, err
		}
		resolved := client.consumerEvidence(client.witness[6].ID)
		return []cognition.LedgerProposal{{
			Kind: cognition.ProposalPlanRevision,
			PlanRevision: &cognition.PlanRevisionProposal{
				Next: goal, EvidenceRefs: latest,
			},
		}}, releaseExtendedEvidence(resolved), appendUniqueEvidence(latest, resolved), nil
	default:
		return []cognition.LedgerProposal{}, []cognition.AttentionRequest{}, nil, nil
	}
}
