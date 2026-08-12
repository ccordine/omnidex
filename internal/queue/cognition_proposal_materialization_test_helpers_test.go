package queue

import "github.com/gryph/omnidex/internal/cognition"

func cognitionProposalMaterializationDecision(
	fixture cognitionDatabaseFixture,
) cognition.CognitionDecision {
	return cognition.CognitionDecision{
		ObligationID: fixture.Start.Root.ID,
		Action: cognition.ActionRequest{
			Kind: fixture.Catalog.Schemas[0].Kind,
			Arguments: []cognition.ActionArgument{{
				Name: "target", Value: "artifact-1",
			}},
		},
		EvidenceRefs:   []cognition.EvidenceRef{fixture.Evidence},
		ExpectedEffect: "Inspect the exact current mechanism.",
		Proposals: []cognition.LedgerProposal{{
			Kind: cognition.ProposalHypothesis, Content: "The first mechanism remains available.",
			EvidenceRefs: []cognition.EvidenceRef{fixture.Evidence},
		}},
		Attention: []cognition.AttentionRequest{},
	}
}
