package cognitionstate

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

type PlanRevisionProposalInput struct {
	Graph               cognition.ObligationGraphSnapshot
	Snapshot            cognition.RuntimeSnapshot
	Decision            cognition.CognitionDecision
	ActionSchema        cognition.ActionSchema
	ProposalIndex       int
	CompletionAuthority cognition.CompletionAuthority
}

func MaterializePlanRevisionProposal(
	input PlanRevisionProposalInput,
) (cognition.PlanRevisionMaterialization, error) {
	if err := input.Graph.Validate(); err != nil {
		return cognition.PlanRevisionMaterialization{}, fmt.Errorf("%w: graph: %v", ErrInvalidMapping, err)
	}
	if err := input.Snapshot.Validate(); err != nil {
		return cognition.PlanRevisionMaterialization{}, fmt.Errorf("%w: snapshot: %v", ErrInvalidMapping, err)
	}
	schema, exists := input.Snapshot.ActionCatalog().Schema(input.Decision.Action.Kind)
	if !exists || schema.Ref() != input.ActionSchema.Ref() || input.Decision.Validate(input.ActionSchema) != nil {
		return cognition.PlanRevisionMaterialization{}, fmt.Errorf("%w: decision schema is not bound to the snapshot", ErrInvalidMapping)
	}
	current := input.Snapshot.CurrentObligation()
	if input.Decision.ObligationID != current.ID || current.CreatedGeneration != input.Graph.Generation {
		return cognition.PlanRevisionMaterialization{}, fmt.Errorf("%w: decision is outside the current plan generation", ErrInvalidMapping)
	}
	graphCurrent, found := graphObligation(input.Graph, current.ID)
	root, rootFound := graphObligation(input.Graph, input.Graph.RootID)
	if !found || graphCurrent.Status != cognition.ObligationActive || !rootFound {
		return cognition.PlanRevisionMaterialization{}, fmt.Errorf("%w: active/root graph authority is unavailable", ErrInvalidMapping)
	}
	wantCurrent, err := mappingDigest(current)
	if err != nil {
		return cognition.PlanRevisionMaterialization{}, err
	}
	gotCurrent, currentErr := mappingDigest(graphCurrent)
	rootGoal, rootErr := mappingDigest(root.Desired)
	snapshotGoal, snapshotErr := mappingDigest(input.Snapshot.Goal())
	if currentErr != nil || rootErr != nil || snapshotErr != nil || gotCurrent != wantCurrent || rootGoal != snapshotGoal {
		return cognition.PlanRevisionMaterialization{}, fmt.Errorf("%w: graph, snapshot, and root goal differ", ErrInvalidMapping)
	}
	available := make(map[cognition.EvidenceRef]struct{}, len(input.Snapshot.EvidenceRefs()))
	for _, ref := range input.Snapshot.EvidenceRefs() {
		available[ref] = struct{}{}
	}
	for _, ref := range input.Decision.EvidenceRefs {
		if _, exists := available[ref]; !exists {
			return cognition.PlanRevisionMaterialization{}, fmt.Errorf("%w: decision cites evidence outside the snapshot", ErrInvalidMapping)
		}
	}
	proposal, err := exactPlanRevisionProposal(input.Decision.Proposals, input.ProposalIndex)
	if err != nil {
		return cognition.PlanRevisionMaterialization{}, err
	}
	decisionSHA, err := mappingDigest(input.Decision.Clone())
	if err != nil {
		return cognition.PlanRevisionMaterialization{}, err
	}
	value, err := cognition.BuildPlanRevisionMaterialization(cognition.PlanRevisionMaterializationInput{
		EpisodeID: input.Snapshot.CurrentRevision().EpisodeID, Graph: input.Graph,
		ActiveObligationID: current.ID, Proposal: proposal,
		AvailableEvidence: input.Snapshot.EvidenceRefs(), CompletionAuthority: input.CompletionAuthority,
		SourceSnapshotSHA256: input.Snapshot.SHA256(), SourceDecisionSHA256: decisionSHA,
		ProposalIndex: input.ProposalIndex,
	})
	if err != nil {
		return cognition.PlanRevisionMaterialization{}, fmt.Errorf("%w: build plan revision: %w", ErrInvalidMapping, err)
	}
	return value.Clone(), nil
}

func exactPlanRevisionProposal(
	proposals []cognition.LedgerProposal,
	index int,
) (cognition.PlanRevisionProposal, error) {
	if index < 0 || index >= len(proposals) || len(proposals) != 1 {
		return cognition.PlanRevisionProposal{}, fmt.Errorf("%w: plan revision must be the sole proposal", ErrInvalidMapping)
	}
	selected := proposals[index]
	if selected.Kind != cognition.ProposalPlanRevision || selected.PlanRevision == nil {
		return cognition.PlanRevisionProposal{}, fmt.Errorf("%w: typed plan revision is unavailable", ErrInvalidMapping)
	}
	return selected.PlanRevision.Clone(), nil
}
