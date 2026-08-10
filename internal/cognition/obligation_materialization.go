package cognition

import (
	"fmt"
	"reflect"
	"strings"
)

func BuildObligationMaterialization(
	input ObligationMaterializationInput,
) (ObligationMaterialization, error) {
	if err := input.Graph.Validate(); err != nil {
		return ObligationMaterialization{}, fmt.Errorf("%w: graph: %v", ErrInvalidObligationMaterialization, err)
	}
	if !validSHA256(input.SourceSnapshotSHA256) || !validSHA256(input.SourceDecisionSHA256) {
		return ObligationMaterialization{}, fmt.Errorf("%w: source snapshot or decision hash is invalid", ErrInvalidObligationMaterialization)
	}
	if input.ProposalIndex < 0 || input.ProposalIndex >= MaxLedgerProposals {
		return ObligationMaterialization{}, fmt.Errorf("%w: proposal index is outside fixed bounds", ErrInvalidObligationMaterialization)
	}
	proposal, err := canonicalObligationProposal(input.Proposal)
	if err != nil {
		return ObligationMaterialization{}, fmt.Errorf("%w: %v", ErrInvalidObligationMaterialization, err)
	}
	if err := validateEvidenceRefs(input.AvailableEvidence); err != nil {
		return ObligationMaterialization{}, fmt.Errorf("%w: available evidence: %v", ErrInvalidObligationMaterialization, err)
	}
	available := make(map[string]struct{}, len(input.AvailableEvidence))
	for _, ref := range input.AvailableEvidence {
		available[evidenceIdentity(ref)] = struct{}{}
	}
	for index, ref := range proposal.EvidenceRefs {
		if ref.Revision.EpisodeID != input.EpisodeID {
			return ObligationMaterialization{}, fmt.Errorf(
				"%w: evidence %d belongs to another episode", ErrInvalidObligationMaterialization, index,
			)
		}
		if _, exists := available[evidenceIdentity(ref)]; !exists {
			return ObligationMaterialization{}, fmt.Errorf(
				"%w: evidence %d is outside the code-owned packet", ErrInvalidObligationMaterialization, index,
			)
		}
	}
	check, err := input.CompletionAuthority.Resolve(proposal.Desired)
	if err != nil {
		return ObligationMaterialization{}, err
	}
	id, err := DeriveObligationID(
		input.EpisodeID, input.Graph.Generation, input.ActiveObligationID, proposal.Desired, check,
	)
	if err != nil {
		return ObligationMaterialization{}, fmt.Errorf("%w: derive obligation identity: %w", ErrInvalidObligationMaterialization, err)
	}
	proposalSHA, err := obligationProposalSHA256(proposal)
	if err != nil {
		return ObligationMaterialization{}, err
	}
	spec := ObligationSpec{
		ID: id, ParentID: input.ActiveObligationID, Desired: proposal.Desired,
		DependsOn: []ObligationID{}, SupportingRefs: proposal.EvidenceRefs, CompletionCheck: check,
	}
	after, err := simulateObligationMaterialization(input.Graph, input.ActiveObligationID, spec)
	if err != nil {
		return ObligationMaterialization{}, err
	}
	materialization := ObligationMaterialization{
		Schema:               ObligationMaterializationSchemaV1,
		SourceSnapshotSHA256: input.SourceSnapshotSHA256,
		SourceDecisionSHA256: input.SourceDecisionSHA256,
		SourceProposalSHA256: proposalSHA, ProposalIndex: input.ProposalIndex,
		EpisodeID: input.EpisodeID, Generation: input.Graph.Generation,
		ExpectedGraphSHA256: input.Graph.SHA256, ActiveObligationID: input.ActiveObligationID,
		CompletionAuthority: input.CompletionAuthority.Clone(), Spec: spec,
		ResultGraphSHA256: after.SHA256,
	}
	materialization.SHA256, err = obligationMaterializationSHA256(materialization)
	if err != nil {
		return ObligationMaterialization{}, err
	}
	materialization.ID = "cognition_obligation_materialization_" + materialization.SHA256
	if err := materialization.Validate(); err != nil {
		return ObligationMaterialization{}, err
	}
	return materialization.Clone(), nil
}

func (materialization ObligationMaterialization) Apply(
	before ObligationGraphSnapshot,
) (ObligationGraphSnapshot, error) {
	if err := materialization.Validate(); err != nil {
		return ObligationGraphSnapshot{}, err
	}
	if err := before.Validate(); err != nil {
		return ObligationGraphSnapshot{}, fmt.Errorf("%w: graph: %v", ErrInvalidObligationMaterialization, err)
	}
	if before.Generation != materialization.Generation ||
		before.SHA256 != materialization.ExpectedGraphSHA256 {
		return ObligationGraphSnapshot{}, fmt.Errorf("%w: expected graph authority is stale", ErrInvalidObligationMaterialization)
	}
	after, err := simulateObligationMaterialization(
		before, materialization.ActiveObligationID, materialization.Spec,
	)
	if err != nil {
		return ObligationGraphSnapshot{}, err
	}
	if after.SHA256 != materialization.ResultGraphSHA256 {
		return ObligationGraphSnapshot{}, fmt.Errorf("%w: result hash does not bind the graph mutation", ErrInvalidObligationMaterialization)
	}
	return after.Clone(), nil
}

func (materialization ObligationMaterialization) Validate() error {
	if materialization.Schema != ObligationMaterializationSchemaV1 ||
		!strings.HasPrefix(materialization.ID, "cognition_obligation_materialization_") ||
		materialization.ID != "cognition_obligation_materialization_"+materialization.SHA256 ||
		!validSHA256(materialization.SHA256) {
		return fmt.Errorf("%w: descriptor identity is invalid", ErrInvalidObligationMaterialization)
	}
	if !validSHA256(materialization.SourceSnapshotSHA256) ||
		!validSHA256(materialization.SourceDecisionSHA256) ||
		!validSHA256(materialization.SourceProposalSHA256) ||
		!validSHA256(materialization.ExpectedGraphSHA256) ||
		!validSHA256(materialization.ResultGraphSHA256) ||
		materialization.Generation == 0 || materialization.ProposalIndex < 0 ||
		materialization.ProposalIndex >= MaxLedgerProposals {
		return fmt.Errorf("%w: descriptor authority is invalid", ErrInvalidObligationMaterialization)
	}
	if materialization.Spec.ParentID != materialization.ActiveObligationID ||
		materialization.Spec.DependsOn == nil ||
		len(materialization.Spec.DependsOn) != 0 || len(materialization.Spec.SupportingRefs) == 0 {
		return fmt.Errorf("%w: descriptor does not encode the fixed child operation", ErrInvalidObligationMaterialization)
	}
	if err := materialization.CompletionAuthority.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidObligationMaterialization, err)
	}
	canonical, err := canonicalObligationProposal(ObligationProposal{
		Desired: materialization.Spec.Desired, EvidenceRefs: materialization.Spec.SupportingRefs,
	})
	if err != nil || !reflect.DeepEqual(canonical.Desired, materialization.Spec.Desired) ||
		!reflect.DeepEqual(canonical.EvidenceRefs, materialization.Spec.SupportingRefs) {
		return fmt.Errorf("%w: spec is not canonical", ErrInvalidObligationMaterialization)
	}
	check, err := materialization.CompletionAuthority.Resolve(materialization.Spec.Desired)
	if err != nil || check != materialization.Spec.CompletionCheck {
		return fmt.Errorf("%w: completion check is not code-authorized", ErrInvalidObligationMaterialization)
	}
	wantID, err := DeriveObligationID(
		materialization.EpisodeID, materialization.Generation,
		materialization.ActiveObligationID, materialization.Spec.Desired, check,
	)
	if err != nil || materialization.Spec.ID != wantID {
		return fmt.Errorf("%w: obligation ID is not code-derived", ErrInvalidObligationMaterialization)
	}
	wantProposal, err := obligationProposalSHA256(canonical)
	if err != nil || wantProposal != materialization.SourceProposalSHA256 {
		return fmt.Errorf("%w: proposal hash does not bind the exact semantics", ErrInvalidObligationMaterialization)
	}
	want, err := obligationMaterializationSHA256(materialization)
	if err != nil || want != materialization.SHA256 {
		return fmt.Errorf("%w: hash does not bind the exact descriptor", ErrInvalidObligationMaterialization)
	}
	return nil
}
