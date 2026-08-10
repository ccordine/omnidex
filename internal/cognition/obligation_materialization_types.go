package cognition

type ObligationMaterializationInput struct {
	EpisodeID            EpisodeID
	Graph                ObligationGraphSnapshot
	ActiveObligationID   ObligationID
	Proposal             ObligationProposal
	AvailableEvidence    []EvidenceRef
	CompletionAuthority  CompletionAuthority
	SourceSnapshotSHA256 string
	SourceDecisionSHA256 string
	ProposalIndex        int
}

// ObligationMaterialization is one fixed atomic graph operation: add the
// code-derived child, make it a prerequisite of the active obligation, and
// activate the child. The schema does not permit a model-authored operation
// list or lifecycle fields.
type ObligationMaterialization struct {
	Schema               string              `json:"schema"`
	ID                   string              `json:"id"`
	SHA256               string              `json:"sha256"`
	SourceSnapshotSHA256 string              `json:"source_snapshot_sha256"`
	SourceDecisionSHA256 string              `json:"source_decision_sha256"`
	SourceProposalSHA256 string              `json:"source_proposal_sha256"`
	ProposalIndex        int                 `json:"proposal_index"`
	EpisodeID            EpisodeID           `json:"episode_id"`
	Generation           uint64              `json:"generation"`
	ExpectedGraphSHA256  string              `json:"expected_graph_sha256"`
	ActiveObligationID   ObligationID        `json:"active_obligation_id"`
	CompletionAuthority  CompletionAuthority `json:"completion_authority"`
	Spec                 ObligationSpec      `json:"spec"`
	ResultGraphSHA256    string              `json:"result_graph_sha256"`
}

func (materialization ObligationMaterialization) Clone() ObligationMaterialization {
	materialization.CompletionAuthority = materialization.CompletionAuthority.Clone()
	materialization.Spec.Desired = materialization.Spec.Desired.Clone()
	materialization.Spec.DependsOn = cloneSlice(materialization.Spec.DependsOn)
	materialization.Spec.SupportingRefs = cloneSlice(materialization.Spec.SupportingRefs)
	return materialization
}
