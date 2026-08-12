package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/queue"
)

func (state *semanticReplayState) mapProposalMaterialization(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	value, err := queue.DecodeCognitionProposalMaterialization(
		record.Payload, record.SHA256,
	)
	if err != nil || record.CallOrdinal < 1 {
		return nil, fmt.Errorf("invalid semantic proposal materialization: %v", err)
	}
	command, commandExists := state.reconciles[value.ReconciliationID]
	_, receiptExists := state.reconciliationReceipts[value.ReconciliationID]
	snapshot, snapshotExists := state.snapshots[value.SnapshotSHA256]
	callID := state.reconciliationCalls[value.ReconciliationID]
	if !commandExists || !receiptExists || !snapshotExists ||
		snapshot.callOrdinal != record.CallOrdinal ||
		callID != value.PolicyCallID || state.attemptOrdinals[record.CallOrdinal] != callID {
		return nil, fmt.Errorf(
			"semantic proposal materialization lacks its exact reconciliation source",
		)
	}
	authority := queue.CognitionProposalMaterializationTraceAuthority{
		ReconciliationID: value.ReconciliationID, PolicyCallID: value.PolicyCallID,
		CallOrdinal: uint64(record.CallOrdinal), Phase: record.Phase,
		Sequence: record.Sequence, ID: record.ID, SHA256: record.SHA256,
	}
	if err := queue.VerifyCognitionProposalMaterializationTrace(
		value, authority, snapshot.snapshot, command.Decision, command.ActionSchema,
	); err != nil {
		return nil, fmt.Errorf("verify semantic proposal materialization: %w", err)
	}
	members := state.proposalMaterializations[value.ReconciliationID]
	for _, member := range members {
		if member.Value.ID == value.ID || member.Value.ProposalIndex == value.ProposalIndex {
			return nil, fmt.Errorf("semantic proposal materialization is duplicated")
		}
	}
	state.proposalMaterializations[value.ReconciliationID] = append(
		members, queue.CognitionProposalMaterializationTraceMember{
			Value: value, Authority: authority,
		},
	)
	kind, change, err := semanticMaterializationKnowledge(value)
	if err != nil {
		return nil, err
	}
	draft := sourceDraft(kind, source)
	draft.Knowledge = change
	return []semanticEventDraft{draft}, nil
}

func semanticMaterializationKnowledge(
	value queue.CognitionProposalMaterialization,
) (cognitionreplay.EventKind, *semanticKnowledgeChange, error) {
	var event cognitionreplay.EventKind
	var knowledge cognitionreplay.KnowledgeKind
	status := cognitionreplay.KnowledgePending
	switch value.Proposal.Kind {
	case cognition.ProposalHypothesis:
		event, knowledge = cognitionreplay.EventHypothesisCreated, cognitionreplay.KnowledgeBelief
		status = cognitionreplay.KnowledgeActive
	case cognition.ProposalObligation:
		event, knowledge = cognitionreplay.EventObligationCreated, cognitionreplay.KnowledgeObligation
	case cognition.ProposalObservation, cognition.ProposalQuestion, cognition.ProposalPlanRevision:
		event, knowledge = cognitionreplay.EventEvidenceAcquired, cognitionreplay.KnowledgeEvidence
	default:
		return "", nil, fmt.Errorf(
			"semantic proposal materialization kind %q is not registered",
			value.Proposal.Kind,
		)
	}
	return event, knowledgeChange(
		knowledge, value.EntryURI, status, cognitionreplay.AuthorityModelProposal,
	), nil
}

func (state *semanticReplayState) finishProposalMaterializations() error {
	for reconciliationID, command := range state.reconciles {
		receipt, receiptExists := state.reconciliationReceipts[reconciliationID]
		snapshot, snapshotExists := state.snapshots[command.SnapshotSHA256]
		callID := state.reconciliationCalls[reconciliationID]
		if !receiptExists || !snapshotExists || callID == "" ||
			snapshot.callOrdinal < 1 || state.attemptOrdinals[snapshot.callOrdinal] != callID {
			return fmt.Errorf(
				"semantic proposal materialization set lacks its reconciliation authority",
			)
		}
		err := queue.VerifyCognitionProposalMaterializationTraceSet(
			state.proposalMaterializations[reconciliationID],
			queue.CognitionProposalMaterializationReconciliationAuthority{
				ReconciliationID: reconciliationID, PolicyCallID: callID,
				CallOrdinal: uint64(snapshot.callOrdinal),
			},
			snapshot.snapshot, command, receipt,
		)
		if err != nil {
			return fmt.Errorf("verify semantic proposal materialization set: %w", err)
		}
	}
	for reconciliationID := range state.proposalMaterializations {
		if _, exists := state.reconciles[reconciliationID]; !exists {
			return fmt.Errorf("semantic proposal materialization set is orphaned")
		}
	}
	return nil
}
