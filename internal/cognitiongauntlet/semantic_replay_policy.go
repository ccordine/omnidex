package cognitiongauntlet

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/queue"
)

func (state *semanticReplayState) mapPolicyAttempt(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value cognitionpolicy.CallAttempt
	if err := decodeProductionPayload(record.Payload, &value, "semantic policy attempt"); err != nil ||
		value.Validate() != nil || record.ID != value.ID+":attempt" ||
		record.CallOrdinal < 1 || record.Phase != 30 || record.Sequence != 0 ||
		value.Brain != state.frozenBrain.Ref ||
		value.ProviderAttestation != state.frozenBrain.Attestation ||
		value.HostHardwareAttestation != state.frozenBrain.Host {
		return nil, fmt.Errorf("invalid semantic policy attempt: %v", err)
	}
	snapshot, exists := state.snapshots[value.SnapshotSHA256]
	if !exists || snapshot.callOrdinal != record.CallOrdinal {
		return nil, fmt.Errorf("semantic policy attempt lacks its exact runtime snapshot")
	}
	projection, exists := state.projections[value.ContextProjection.ID]
	if !exists || projection.callOrdinal != record.CallOrdinal ||
		semanticContextProjectionRef(projection.projection) != value.ContextProjection {
		return nil, fmt.Errorf("semantic policy attempt lacks its exact Context Projection")
	}
	if err := cognitionpolicy.VerifyCallAttempt(
		snapshot.snapshot, projection.projection, value,
	); err != nil {
		return nil, fmt.Errorf("verify semantic policy attempt: %w", err)
	}
	process, err := state.providerProcessAuthority(value.ProviderProcessActivation.ObservationID)
	if err != nil || process != value.ProviderProcessActivation {
		return nil, fmt.Errorf("semantic policy attempt lacks its exact provider process: %v", err)
	}
	if _, duplicate := state.attempts[value.ID]; duplicate {
		return nil, fmt.Errorf("semantic policy attempt is duplicated")
	}
	if _, duplicate := state.attemptOrdinals[record.CallOrdinal]; duplicate {
		return nil, fmt.Errorf("semantic policy attempt call ordinal is duplicated")
	}
	if err := state.consumeSnapshot(
		value.SnapshotSHA256, "policy-call://"+value.ID,
	); err != nil {
		return nil, err
	}
	state.attempts[value.ID] = value
	state.attemptOrdinals[record.CallOrdinal] = value.ID
	state.attemptSourceSHA[value.ID] = record.SHA256
	return []semanticEventDraft{sourceDraft(cognitionreplay.EventModelCalled, source)}, nil
}

func (state *semanticReplayState) mapPolicyResult(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value cognitionpolicy.CallResult
	if err := decodeProductionPayload(record.Payload, &value, "semantic policy result"); err != nil {
		return nil, err
	}
	attempt, exists := state.attempts[value.CallID]
	if !exists || record.ID != value.CallID+":result" ||
		record.CallOrdinal < 1 || record.Phase != 31 || record.Sequence != 0 ||
		state.attemptOrdinals[record.CallOrdinal] != value.CallID {
		return nil, fmt.Errorf("semantic policy result lacks its exact valid attempt")
	}
	if _, duplicate := state.results[value.CallID]; duplicate {
		return nil, fmt.Errorf("semantic policy result is duplicated")
	}
	snapshot, exists := state.snapshots[attempt.SnapshotSHA256]
	if !exists || snapshot.callOrdinal != record.CallOrdinal {
		return nil, fmt.Errorf("semantic policy result lacks its exact runtime snapshot")
	}
	evidence, used, err := state.evidence.callEvidence(value)
	if err != nil {
		return nil, err
	}
	decision, err := cognitionpolicy.VerifyCallOutcome(
		snapshot.snapshot, attempt, value, evidence,
	)
	if err != nil {
		return nil, fmt.Errorf("verify semantic policy result: %w", err)
	}
	for _, key := range used {
		if _, duplicate := state.usedPolicyEvidence[key]; duplicate {
			return nil, fmt.Errorf("semantic policy evidence body is reused")
		}
		state.usedPolicyEvidence[key] = struct{}{}
	}
	state.results[value.CallID] = value
	drafts := []semanticEventDraft{
		sourceDraft(cognitionreplay.EventProviderRequestDisposition, source),
		sourceDraft(cognitionreplay.EventModelCallDisposition, source),
	}
	switch value.Status {
	case cognitionpolicy.CallResultAccepted:
		if decision == nil {
			return nil, fmt.Errorf("accepted semantic policy result lacks a decoded decision")
		}
		state.decisions[value.CallID] = decision.Clone()
		drafts = append(drafts, sourceDraft(cognitionreplay.EventDecisionAccepted, source))
	case cognitionpolicy.CallResultRejected:
		if decision != nil {
			return nil, fmt.Errorf("rejected semantic policy result decoded a decision")
		}
		drafts = append(drafts, sourceDraft(cognitionreplay.EventDecisionRejected, source))
	case cognitionpolicy.CallResultFailed:
		if decision != nil {
			return nil, fmt.Errorf("failed semantic policy result decoded a decision")
		}
		draft := sourceKnowledgeDraft(
			cognitionreplay.EventFailureRecorded, source,
			cognitionreplay.KnowledgeFailure, cognitionreplay.KnowledgeFailed,
			cognitionreplay.AuthorityTool,
		)
		draft.Knowledge.Ref = "policy-failure://" + value.CallID
		drafts = append(drafts, draft)
	default:
		return nil, fmt.Errorf("unregistered policy result status %q", value.Status)
	}
	return drafts, nil
}

func (state *semanticReplayState) mapPolicyAbandonment(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value cognitionruntime.PolicyCallAbandonment
	if err := decodeProductionPayload(record.Payload, &value, "semantic policy abandonment"); err != nil ||
		value.Validate() != nil || value.ID != record.ID || record.CallOrdinal < 1 ||
		record.Phase != 33 || record.Sequence != int64(value.RecoveryActor.Attempt) ||
		value.CallOrdinal != uint64(record.CallOrdinal) {
		return nil, fmt.Errorf("invalid semantic policy abandonment: %v", err)
	}
	if attempt, exists := state.attempts[value.CallID]; !exists ||
		attempt.Actor != value.SourceActor || attempt.SnapshotSHA256 != value.SourceSnapshotSHA256 ||
		state.attemptOrdinals[record.CallOrdinal] != value.CallID ||
		state.attemptSourceSHA[value.CallID] != value.SourceAttemptSHA256 {
		return nil, fmt.Errorf("semantic policy abandonment lacks its exact attempt")
	}
	if _, hasResult := state.results[value.CallID]; hasResult {
		return nil, fmt.Errorf("semantic policy abandonment follows a terminal result")
	}
	if _, duplicate := state.abandoned[value.CallID]; duplicate {
		return nil, fmt.Errorf("semantic policy abandonment is duplicated")
	}
	state.abandoned[value.CallID] = struct{}{}
	return []semanticEventDraft{
		sourceDraft(cognitionreplay.EventModelCallDisposition, source),
		sourceDraft(cognitionreplay.EventStaleWriteRejected, source),
	}, nil
}

func (state *semanticReplayState) mapPolicyTiming(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value queue.CognitionTracePolicyTiming
	if err := decodeProductionPayload(record.Payload, &value, "semantic policy timing"); err != nil ||
		value.Validate() != nil || record.ID != value.CallID+":timing" ||
		record.CallOrdinal < 1 || record.Phase != 32 || record.Sequence != 0 ||
		state.attemptOrdinals[record.CallOrdinal] != value.CallID ||
		validateSemanticPolicyTimingBounds(
			value, state.trace.Header.EpisodeStartedAt, state.trace.Header.SealedAt,
		) != nil {
		return nil, fmt.Errorf("invalid semantic policy timing: %v", err)
	}
	if _, duplicate := state.policyTimings[value.CallID]; duplicate {
		return nil, fmt.Errorf("semantic policy timing is duplicated")
	}
	state.policyTimings[value.CallID] = value
	return []semanticEventDraft{sourceKnowledgeDraft(
		cognitionreplay.EventEvidenceAcquired, source,
		cognitionreplay.KnowledgeEvidence, cognitionreplay.KnowledgeActive,
		cognitionreplay.AuthorityCode,
	)}, nil
}

func (state *semanticReplayState) mapReconciliationCommand(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value cognitionruntime.ReconciliationCommand
	if err := decodeProductionPayload(record.Payload, &value, "semantic reconciliation command"); err != nil ||
		value.Validate() != nil || value.Binding.Episode.ID != state.trace.Header.EpisodeID ||
		record.CallOrdinal < 1 || record.Phase != 40 || record.Sequence != 0 {
		return nil, fmt.Errorf("invalid semantic reconciliation command: %v", err)
	}
	snapshot, exists := state.snapshots[value.SnapshotSHA256]
	if !exists || snapshot.callOrdinal != record.CallOrdinal ||
		value.Projection != snapshot.snapshot.ContextProjection() {
		return nil, fmt.Errorf("semantic reconciliation command lacks its exact runtime snapshot")
	}
	callID := state.attemptOrdinals[record.CallOrdinal]
	if value.Recovery != nil {
		callID = value.Recovery.PolicyCallID
		recovery, recovered := state.recoveries[value.Recovery.ID]
		if !recovered || recovery.Recovery != *value.Recovery ||
			recovery.Binding != value.Binding ||
			recovery.SourceActor != snapshot.snapshot.Attempt() ||
			recovery.SnapshotSHA256 != value.SnapshotSHA256 {
			return nil, fmt.Errorf("semantic reconciliation command lacks its exact recovery")
		}
		if consumer, duplicate := state.recoveryConsumers[value.Recovery.ID]; duplicate {
			return nil, fmt.Errorf("semantic accepted-decision recovery is reused by %q", consumer)
		}
		state.recoveryConsumers[value.Recovery.ID] = record.ID
	} else if value.Binding.Attempt != snapshot.snapshot.Attempt() {
		return nil, fmt.Errorf("semantic source reconciliation actor changed")
	}
	decision, accepted := state.decisions[callID]
	if !accepted || !reflect.DeepEqual(decision, value.Decision) {
		return nil, fmt.Errorf("semantic reconciliation command changed the accepted decision")
	}
	if _, duplicate := state.reconciles[record.ID]; duplicate {
		return nil, fmt.Errorf("semantic reconciliation command is duplicated")
	}
	if _, duplicate := state.reconciliationOrdinals[record.CallOrdinal]; duplicate {
		return nil, fmt.Errorf("semantic reconciliation command call ordinal is duplicated")
	}
	if prior, duplicate := state.callReconciliations[callID]; duplicate {
		return nil, fmt.Errorf("semantic accepted call already reconciles through %q", prior)
	}
	state.reconciles[record.ID] = value.Clone()
	state.reconciliationOrdinals[record.CallOrdinal] = record.ID
	state.reconciliationCalls[record.ID] = callID
	state.callReconciliations[callID] = record.ID
	if err := state.planObligationMaterialization(record.ID, snapshot.snapshot, value); err != nil {
		return nil, err
	}
	return []semanticEventDraft{
		sourceDraft(cognitionreplay.EventDecisionAccepted, source),
	}, nil
}

func (state *semanticReplayState) mapReconciliationReceipt(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value cognitionruntime.ReconciliationReceipt
	command, exists := state.reconciles[record.ID]
	if err := decodeProductionPayload(record.Payload, &value, "semantic reconciliation receipt"); err != nil ||
		!exists || value.ValidateFor(command) != nil || value.ID != record.ID ||
		record.CallOrdinal < 1 || record.Phase != 41 || record.Sequence != 0 {
		return nil, fmt.Errorf("invalid semantic reconciliation receipt: %v", err)
	}
	snapshot, snapshotExists := state.snapshots[value.SnapshotSHA256]
	if !snapshotExists || snapshot.callOrdinal != record.CallOrdinal {
		return nil, fmt.Errorf("semantic reconciliation receipt lacks its exact runtime snapshot")
	}
	if _, duplicate := state.reconciliationReceipts[record.ID]; duplicate {
		return nil, fmt.Errorf("semantic reconciliation receipt is duplicated")
	}
	state.reconciliationReceipts[record.ID] = value.Clone()
	return []semanticEventDraft{sourceKnowledgeDraft(
		cognitionreplay.EventEvidenceAcquired, source,
		cognitionreplay.KnowledgeEvidence, cognitionreplay.KnowledgeActive,
		cognitionreplay.AuthorityCode,
	)}, nil
}
