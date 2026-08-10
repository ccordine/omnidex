package cognition

const PlanRevisionMaterializationSchemaV1 = "omnidex.cognition-plan-revision-materialization.v1"

type PlanRevisionMaterializationInput struct {
	EpisodeID            EpisodeID
	Graph                ObligationGraphSnapshot
	ActiveObligationID   ObligationID
	Proposal             PlanRevisionProposal
	AvailableEvidence    []EvidenceRef
	CompletionAuthority  CompletionAuthority
	SourceSnapshotSHA256 string
	SourceDecisionSHA256 string
	ProposalIndex        int
}

// PlanRevisionMaterialization is the sole code-owned generation cutover
// descriptor. Root and next-obligation identities, dependencies, checks, and
// lifecycle states are derived rather than accepted from model output.
type PlanRevisionMaterialization struct {
	Schema               string              `json:"schema"`
	ID                   string              `json:"id"`
	SHA256               string              `json:"sha256"`
	SourceSnapshotSHA256 string              `json:"source_snapshot_sha256"`
	SourceDecisionSHA256 string              `json:"source_decision_sha256"`
	SourceProposalSHA256 string              `json:"source_proposal_sha256"`
	ProposalIndex        int                 `json:"proposal_index"`
	EpisodeID            EpisodeID           `json:"episode_id"`
	PreviousGeneration   uint64              `json:"previous_generation"`
	NextGeneration       uint64              `json:"next_generation"`
	ExpectedGraphSHA256  string              `json:"expected_graph_sha256"`
	ActiveObligationID   ObligationID        `json:"active_obligation_id"`
	CompletionAuthority  CompletionAuthority `json:"completion_authority"`
	Root                 ObligationSpec      `json:"root"`
	Next                 ObligationSpec      `json:"next"`
	ResultGraphSHA256    string              `json:"result_graph_sha256"`
}

func (value PlanRevisionMaterialization) Clone() PlanRevisionMaterialization {
	value.CompletionAuthority = value.CompletionAuthority.Clone()
	value.Root = cloneObligationSpec(value.Root)
	value.Next = cloneObligationSpec(value.Next)
	return value
}

func cloneObligationSpec(value ObligationSpec) ObligationSpec {
	value.Desired = value.Desired.Clone()
	value.DependsOn = cloneSlice(value.DependsOn)
	value.SupportingRefs = cloneSlice(value.SupportingRefs)
	return value
}
