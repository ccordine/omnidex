package cognition

type LedgerProposalKind string

const (
	ProposalObservation  LedgerProposalKind = "observation"
	ProposalHypothesis   LedgerProposalKind = "hypothesis"
	ProposalQuestion     LedgerProposalKind = "question"
	ProposalObligation   LedgerProposalKind = "obligation"
	ProposalRevision     LedgerProposalKind = "revision"
	ProposalPlanRevision LedgerProposalKind = "plan_revision"
)

type LedgerProposal struct {
	Kind         LedgerProposalKind      `json:"kind"`
	Content      string                  `json:"content,omitempty"`
	EvidenceRefs []EvidenceRef           `json:"evidence_refs,omitempty"`
	Obligation   *ObligationProposal     `json:"obligation,omitempty"`
	Revision     *BeliefRevisionProposal `json:"revision,omitempty"`
	PlanRevision *PlanRevisionProposal   `json:"plan_revision,omitempty"`
}

type AttentionOperation string

const (
	AttentionRetain  AttentionOperation = "retain"
	AttentionRelease AttentionOperation = "release"
)

type AttentionScope string

const (
	AttentionScopeDecision   AttentionScope = "decision"
	AttentionScopeObligation AttentionScope = "obligation"
	AttentionScopeEpisode    AttentionScope = "episode"
)

type AttentionRequest struct {
	Operation AttentionOperation `json:"operation"`
	TargetRef EvidenceRef        `json:"target_ref"`
	Scope     AttentionScope     `json:"scope"`
	Reason    string             `json:"reason"`
}

// CognitionDecision is a bounded model proposal. It cannot assign action IDs,
// mutate authoritative state, or declare completion.
type CognitionDecision struct {
	ObligationID   ObligationID       `json:"obligation_id"`
	Action         ActionRequest      `json:"action"`
	EvidenceRefs   []EvidenceRef      `json:"evidence_refs"`
	ExpectedEffect string             `json:"expected_effect"`
	Proposals      []LedgerProposal   `json:"ledger_proposals,omitempty"`
	Attention      []AttentionRequest `json:"attention_requests,omitempty"`
}
