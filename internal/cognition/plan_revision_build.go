package cognition

import (
	"fmt"
	"math"
)

func BuildPlanRevisionMaterialization(
	input PlanRevisionMaterializationInput,
) (PlanRevisionMaterialization, error) {
	if err := input.Graph.Validate(); err != nil {
		return PlanRevisionMaterialization{}, fmt.Errorf("%w: graph: %v", ErrInvalidPlanRevisionMaterialization, err)
	}
	if !validSHA256(input.SourceSnapshotSHA256) || !validSHA256(input.SourceDecisionSHA256) {
		return PlanRevisionMaterialization{}, fmt.Errorf("%w: source hashes are invalid", ErrInvalidPlanRevisionMaterialization)
	}
	if input.Graph.Generation == math.MaxUint64 || input.ProposalIndex < 0 || input.ProposalIndex >= MaxLedgerProposals {
		return PlanRevisionMaterialization{}, fmt.Errorf("%w: generation or proposal index is outside bounds", ErrInvalidPlanRevisionMaterialization)
	}
	proposal, err := canonicalPlanRevisionProposal(input.Proposal)
	if err != nil {
		return PlanRevisionMaterialization{}, fmt.Errorf("%w: %v", ErrInvalidPlanRevisionMaterialization, err)
	}
	if err := requirePlanRevisionEvidence(input, proposal); err != nil {
		return PlanRevisionMaterialization{}, err
	}
	active, found := obligationInSnapshot(input.Graph, input.ActiveObligationID)
	root, rootFound := obligationInSnapshot(input.Graph, input.Graph.RootID)
	if !found || active.Status != ObligationActive || active.CreatedGeneration != input.Graph.Generation ||
		!rootFound || terminalOrSuperseded(root.Status) || root.CreatedGeneration != input.Graph.Generation {
		return PlanRevisionMaterialization{}, fmt.Errorf("%w: current active/root authority is unavailable", ErrInvalidPlanRevisionMaterialization)
	}
	if err := requireDistinctPlanRevisionGoal(root, active, proposal.Next); err != nil {
		return PlanRevisionMaterialization{}, err
	}
	rootCheck, err := input.CompletionAuthority.Resolve(root.Desired)
	if err != nil || rootCheck != root.CompletionCheck {
		return PlanRevisionMaterialization{}, fmt.Errorf("%w: root completion authority changed", ErrInvalidPlanRevisionMaterialization)
	}
	nextCheck, err := input.CompletionAuthority.Resolve(proposal.Next)
	if err != nil {
		return PlanRevisionMaterialization{}, err
	}
	nextGeneration := input.Graph.Generation + 1
	rootID, err := DeriveObligationID(input.EpisodeID, nextGeneration, "", root.Desired, rootCheck)
	if err != nil {
		return PlanRevisionMaterialization{}, err
	}
	nextID, err := DeriveObligationID(input.EpisodeID, nextGeneration, rootID, proposal.Next, nextCheck)
	if err != nil {
		return PlanRevisionMaterialization{}, err
	}
	rootSpec := ObligationSpec{
		ID: rootID, Desired: root.Desired, DependsOn: []ObligationID{nextID},
		SupportingRefs: root.SupportingRefs, CompletionCheck: rootCheck,
	}
	nextSpec := ObligationSpec{
		ID: nextID, ParentID: rootID, Desired: proposal.Next,
		DependsOn: []ObligationID{}, SupportingRefs: proposal.EvidenceRefs,
		CompletionCheck: nextCheck,
	}
	after, err := simulatePlanRevision(input.Graph, input.ActiveObligationID, rootSpec, nextSpec)
	if err != nil {
		return PlanRevisionMaterialization{}, err
	}
	proposalSHA, err := planRevisionProposalSHA256(proposal)
	if err != nil {
		return PlanRevisionMaterialization{}, err
	}
	value := PlanRevisionMaterialization{
		Schema: PlanRevisionMaterializationSchemaV1, SourceSnapshotSHA256: input.SourceSnapshotSHA256,
		SourceDecisionSHA256: input.SourceDecisionSHA256, SourceProposalSHA256: proposalSHA,
		ProposalIndex: input.ProposalIndex, EpisodeID: input.EpisodeID,
		PreviousGeneration: input.Graph.Generation, NextGeneration: nextGeneration,
		ExpectedGraphSHA256: input.Graph.SHA256, ActiveObligationID: input.ActiveObligationID,
		CompletionAuthority: input.CompletionAuthority.Clone(), Root: rootSpec, Next: nextSpec,
		ResultGraphSHA256: after.SHA256,
	}
	value.SHA256, err = planRevisionMaterializationSHA256(value)
	if err != nil {
		return PlanRevisionMaterialization{}, err
	}
	value.ID = "cognition_plan_revision_" + value.SHA256
	if err := value.Validate(); err != nil {
		return PlanRevisionMaterialization{}, err
	}
	return value.Clone(), nil
}

func requirePlanRevisionEvidence(input PlanRevisionMaterializationInput, proposal PlanRevisionProposal) error {
	if err := validateEvidenceRefs(input.AvailableEvidence); err != nil {
		return fmt.Errorf("%w: available evidence: %v", ErrInvalidPlanRevisionMaterialization, err)
	}
	available := make(map[string]struct{}, len(input.AvailableEvidence))
	for _, ref := range input.AvailableEvidence {
		available[evidenceIdentity(ref)] = struct{}{}
	}
	for index, ref := range proposal.EvidenceRefs {
		if ref.Revision.EpisodeID != input.EpisodeID {
			return fmt.Errorf("%w: evidence %d belongs to another episode", ErrInvalidPlanRevisionMaterialization, index)
		}
		if _, exists := available[evidenceIdentity(ref)]; !exists {
			return fmt.Errorf("%w: evidence %d is outside the code-owned packet", ErrInvalidPlanRevisionMaterialization, index)
		}
	}
	return nil
}

func requireDistinctPlanRevisionGoal(root, active Obligation, next GoalExpression) error {
	nextID, err := goalIdentity(next)
	if err != nil {
		return fmt.Errorf("%w: next goal: %v", ErrInvalidPlanRevisionMaterialization, err)
	}
	rootID, rootErr := goalIdentity(root.Desired)
	activeID, activeErr := goalIdentity(active.Desired)
	if rootErr != nil || activeErr != nil || nextID == rootID || nextID == activeID {
		return fmt.Errorf("%w: next goal does not revise the active plan", ErrInvalidPlanRevisionMaterialization)
	}
	return nil
}
