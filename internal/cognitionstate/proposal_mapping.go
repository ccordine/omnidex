package cognitionstate

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

type ModelProposalInput struct {
	Ledger       taskstate.MaterializedState
	ScopeNodeID  taskstate.NodeID
	Snapshot     cognition.RuntimeSnapshot
	Decision     cognition.CognitionDecision
	ActionSchema cognition.ActionSchema
}

func MapModelProposals(input ModelProposalInput) ([]EntryMutation, error) {
	if err := taskstate.ValidateMaterializedState(input.Ledger); err != nil {
		return nil, fmt.Errorf("%w: ledger: %v", ErrInvalidMapping, err)
	}
	if err := input.Snapshot.Validate(); err != nil {
		return nil, fmt.Errorf("%w: snapshot: %v", ErrInvalidMapping, err)
	}
	catalogSchema, exists := input.Snapshot.ActionCatalog().Schema(input.Decision.Action.Kind)
	if !exists || catalogSchema.Ref() != input.ActionSchema.Ref() {
		return nil, fmt.Errorf("%w: decision schema is not the snapshot catalog schema", ErrInvalidMapping)
	}
	if err := input.Decision.Validate(input.ActionSchema); err != nil {
		return nil, fmt.Errorf("%w: decision: %v", ErrInvalidMapping, err)
	}
	if input.Decision.ObligationID != input.Snapshot.CurrentObligation().ID {
		return nil, fmt.Errorf("%w: decision targets another obligation", ErrInvalidMapping)
	}
	available := make(map[cognition.EvidenceRef]struct{}, len(input.Snapshot.EvidenceRefs()))
	for _, ref := range input.Snapshot.EvidenceRefs() {
		available[ref] = struct{}{}
	}
	for _, ref := range input.Decision.EvidenceRefs {
		if _, ok := available[ref]; !ok {
			return nil, fmt.Errorf("%w: decision cites evidence outside the snapshot", ErrInvalidMapping)
		}
	}
	mutations := make([]EntryMutation, 0, len(input.Decision.Proposals))
	for index, proposal := range input.Decision.Proposals {
		if proposal.Kind == cognition.ProposalRevision {
			continue
		}
		sourceKind, entryKind, err := proposalEntryKinds(proposal.Kind)
		if err != nil {
			return nil, err
		}
		content, evidenceRefs, err := proposalEntryContent(proposal)
		if err != nil {
			return nil, fmt.Errorf("%w: proposal %d: %v", ErrInvalidMapping, index, err)
		}
		refs := make([]taskstate.Ref, len(evidenceRefs))
		for refIndex, ref := range evidenceRefs {
			refs[refIndex] = evidenceLedgerRef(ref)
		}
		metadataFields := map[string]any{
			"attempt": input.Snapshot.Attempt(), "context_projection": input.Snapshot.ContextProjection(),
			"snapshot_sha256": input.Snapshot.SHA256(), "proposal_index": index,
		}
		if proposal.Kind == cognition.ProposalObligation {
			metadataFields["candidate_kind"] = "obligation"
		} else if proposal.Kind == cognition.ProposalPlanRevision {
			metadataFields["candidate_kind"] = "plan_revision"
		}
		mutation, err := newEntryMutation(entryCommandInput{
			Ledger: input.Ledger, ScopeNodeID: input.ScopeNodeID,
			SourceKind: sourceKind,
			Source: struct {
				SnapshotSHA256 string                         `json:"snapshot_sha256"`
				Attempt        cognition.AttemptRef           `json:"attempt"`
				Projection     cognition.ContextProjectionRef `json:"context_projection"`
				Decision       cognition.CognitionDecision    `json:"decision"`
				ProposalIndex  int                            `json:"proposal_index"`
			}{input.Snapshot.SHA256(), input.Snapshot.Attempt(), input.Snapshot.ContextProjection(),
				input.Decision.Clone(), index},
			Actor: taskstate.AuthorityModelProposal, Kind: entryKind,
			Content: content, Refs: refs, Metadata: metadataFields,
			ExpectedVersion: input.Ledger.Version + uint64(index),
		})
		if err != nil {
			return nil, err
		}
		mutations = append(mutations, mutation)
	}
	ledger, err := taskstate.RestoreLedger(input.Ledger)
	if err != nil {
		return nil, fmt.Errorf("%w: ledger: %v", ErrInvalidMapping, err)
	}
	for index, mutation := range mutations {
		if _, err := ledger.Apply(mutation.Command()); err != nil {
			return nil, fmt.Errorf("%w: apply proposal %d: %v", ErrInvalidMapping, index, err)
		}
	}
	return mutations, nil
}

func proposalEntryKinds(kind cognition.LedgerProposalKind) (SourceKind, taskstate.EntryKind, error) {
	switch kind {
	case cognition.ProposalObservation:
		return SourceModelObservation, taskstate.EntryObservation, nil
	case cognition.ProposalHypothesis:
		return SourceModelHypothesis, taskstate.EntryHypothesis, nil
	case cognition.ProposalQuestion:
		return SourceModelQuestion, taskstate.EntryQuestion, nil
	case cognition.ProposalObligation:
		return SourceModelObligation, taskstate.EntryDecisionCandidate, nil
	case cognition.ProposalPlanRevision:
		return SourceModelPlanRevision, taskstate.EntryDecisionCandidate, nil
	default:
		return "", "", fmt.Errorf("%w: proposal kind %q is not mappable", ErrInvalidMapping, kind)
	}
}
