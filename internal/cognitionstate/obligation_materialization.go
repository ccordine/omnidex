package cognitionstate

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

type ObligationProposalInput struct {
	Graph               cognition.ObligationGraphSnapshot
	Snapshot            cognition.RuntimeSnapshot
	Decision            cognition.CognitionDecision
	ActionSchema        cognition.ActionSchema
	ProposalIndex       int
	CompletionAuthority cognition.CompletionAuthority
}

func MaterializeObligationProposal(
	input ObligationProposalInput,
) (cognition.ObligationMaterialization, error) {
	if err := input.Graph.Validate(); err != nil {
		return cognition.ObligationMaterialization{}, fmt.Errorf("%w: graph: %v", ErrInvalidMapping, err)
	}
	if err := input.Snapshot.Validate(); err != nil {
		return cognition.ObligationMaterialization{}, fmt.Errorf("%w: snapshot: %v", ErrInvalidMapping, err)
	}
	catalogSchema, exists := input.Snapshot.ActionCatalog().Schema(input.Decision.Action.Kind)
	if !exists || catalogSchema.Ref() != input.ActionSchema.Ref() {
		return cognition.ObligationMaterialization{}, fmt.Errorf(
			"%w: decision schema is not the snapshot catalog schema", ErrInvalidMapping,
		)
	}
	if err := input.Decision.Validate(input.ActionSchema); err != nil {
		return cognition.ObligationMaterialization{}, fmt.Errorf("%w: decision: %v", ErrInvalidMapping, err)
	}
	current := input.Snapshot.CurrentObligation()
	if input.Decision.ObligationID != current.ID {
		return cognition.ObligationMaterialization{}, fmt.Errorf("%w: decision targets another obligation", ErrInvalidMapping)
	}
	if current.CreatedGeneration != input.Graph.Generation {
		return cognition.ObligationMaterialization{}, fmt.Errorf("%w: graph generation is not the snapshot generation", ErrInvalidMapping)
	}
	graphCurrent, found := graphObligation(input.Graph, current.ID)
	if !found || graphCurrent.Status != cognition.ObligationActive {
		return cognition.ObligationMaterialization{}, fmt.Errorf("%w: snapshot obligation is not active in the graph", ErrInvalidMapping)
	}
	wantCurrent, err := mappingDigest(current)
	if err != nil {
		return cognition.ObligationMaterialization{}, err
	}
	gotCurrent, err := mappingDigest(graphCurrent)
	if err != nil || gotCurrent != wantCurrent {
		return cognition.ObligationMaterialization{}, fmt.Errorf("%w: graph and snapshot obligations differ", ErrInvalidMapping)
	}
	available := make(map[cognition.EvidenceRef]struct{}, len(input.Snapshot.EvidenceRefs()))
	for _, ref := range input.Snapshot.EvidenceRefs() {
		available[ref] = struct{}{}
	}
	for _, ref := range input.Decision.EvidenceRefs {
		if _, exists := available[ref]; !exists {
			return cognition.ObligationMaterialization{}, fmt.Errorf("%w: decision cites evidence outside the snapshot", ErrInvalidMapping)
		}
	}
	proposal, err := exactObligationProposal(input.Decision.Proposals, input.ProposalIndex)
	if err != nil {
		return cognition.ObligationMaterialization{}, err
	}
	decisionSHA, err := mappingDigest(input.Decision.Clone())
	if err != nil {
		return cognition.ObligationMaterialization{}, err
	}
	materialization, err := cognition.BuildObligationMaterialization(cognition.ObligationMaterializationInput{
		EpisodeID: input.Snapshot.CurrentRevision().EpisodeID,
		Graph:     input.Graph, ActiveObligationID: current.ID, Proposal: proposal,
		AvailableEvidence:    input.Snapshot.EvidenceRefs(),
		CompletionAuthority:  input.CompletionAuthority,
		SourceSnapshotSHA256: input.Snapshot.SHA256(), SourceDecisionSHA256: decisionSHA,
		ProposalIndex: input.ProposalIndex,
	})
	if err != nil {
		return cognition.ObligationMaterialization{}, fmt.Errorf(
			"%w: build obligation materialization: %w", ErrInvalidMapping, err,
		)
	}
	return materialization.Clone(), nil
}

func exactObligationProposal(
	proposals []cognition.LedgerProposal,
	index int,
) (cognition.ObligationProposal, error) {
	if index < 0 || index >= len(proposals) {
		return cognition.ObligationProposal{}, fmt.Errorf("%w: obligation proposal index is unavailable", ErrInvalidMapping)
	}
	count := 0
	for _, proposal := range proposals {
		if proposal.Kind == cognition.ProposalObligation {
			count++
		}
	}
	selected := proposals[index]
	if count != 1 || selected.Kind != cognition.ProposalObligation || selected.Obligation == nil {
		return cognition.ObligationProposal{}, fmt.Errorf("%w: decision must identify exactly one obligation proposal", ErrInvalidMapping)
	}
	return selected.Obligation.Clone(), nil
}

func graphObligation(
	graph cognition.ObligationGraphSnapshot,
	id cognition.ObligationID,
) (cognition.Obligation, bool) {
	for _, obligation := range graph.Obligations {
		if obligation.ID == id {
			return obligation.Clone(), true
		}
	}
	return cognition.Obligation{}, false
}
