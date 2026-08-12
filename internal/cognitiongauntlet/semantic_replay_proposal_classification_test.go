package cognitiongauntlet

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/queue"
)

func TestSemanticProposalMaterializationPreservesModelAuthority(t *testing.T) {
	for name, test := range map[string]struct {
		kind      cognition.LedgerProposalKind
		event     cognitionreplay.EventKind
		knowledge cognitionreplay.KnowledgeKind
		status    cognitionreplay.KnowledgeStatus
	}{
		"hypothesis": {
			cognition.ProposalHypothesis, cognitionreplay.EventHypothesisCreated,
			cognitionreplay.KnowledgeBelief, cognitionreplay.KnowledgeActive,
		},
		"observation": {
			cognition.ProposalObservation, cognitionreplay.EventEvidenceAcquired,
			cognitionreplay.KnowledgeEvidence, cognitionreplay.KnowledgePending,
		},
		"question": {
			cognition.ProposalQuestion, cognitionreplay.EventEvidenceAcquired,
			cognitionreplay.KnowledgeEvidence, cognitionreplay.KnowledgePending,
		},
		"obligation": {
			cognition.ProposalObligation, cognitionreplay.EventObligationCreated,
			cognitionreplay.KnowledgeObligation, cognitionreplay.KnowledgePending,
		},
		"plan revision": {
			cognition.ProposalPlanRevision, cognitionreplay.EventEvidenceAcquired,
			cognitionreplay.KnowledgeEvidence, cognitionreplay.KnowledgePending,
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := queue.CognitionProposalMaterialization{
				Proposal: testProposalForClassification(test.kind),
				EntryURI: "task:ledger/ledger-one/entry/entry-one",
			}
			event, change, err := semanticMaterializationKnowledge(value)
			if err != nil || event != test.event || change == nil ||
				change.Kind != test.knowledge || change.Status != test.status ||
				change.Authority != cognitionreplay.AuthorityModelProposal ||
				change.Ref != value.EntryURI {
				t.Fatalf("proposal mapping event=%q change=%+v error=%v", event, change, err)
			}
		})
	}
}

func testProposalForClassification(kind cognition.LedgerProposalKind) cognition.LedgerProposal {
	return cognition.LedgerProposal{Kind: kind}
}
