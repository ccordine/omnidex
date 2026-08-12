package cognitiongauntlet

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/queue"
)

func (state *semanticReplayState) mapProviderBrainBootstrap(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value queue.CognitionBrainBootstrapTrace
	if err := decodeProductionPayload(record.Payload, &value, "semantic provider Brain bootstrap"); err != nil {
		return nil, err
	}
	if err := value.Validate(); err != nil || value.SourceID != record.ID ||
		value.EpisodeID != state.trace.Header.EpisodeID ||
		record.CallOrdinal != 0 || !semanticProviderBootstrapTimeExact(
		value, state.trace.Header,
	) {
		return nil, fmt.Errorf("invalid semantic provider Brain bootstrap: %v", err)
	}
	switch value.Source {
	case queue.CognitionBrainBootstrapEpisodeStart:
		if record.Phase != 1 || record.Sequence != 0 ||
			!semanticStableBrainEqual(value.Brain, state.frozenBrain) {
			return nil, fmt.Errorf("semantic initial Brain bootstrap tuple changed")
		}
		if state.initialBrainBootstrap {
			return nil, fmt.Errorf("semantic replay duplicates its initial provider Brain bootstrap")
		}
		state.initialBrainBootstrap = true
		state.initialActor = value.Actor
		copy := value
		state.initialBootstrapTrace = &copy
	case queue.CognitionBrainBootstrapEpisodeReplay:
		if record.Phase != 2 || record.Sequence != int64(value.Actor.Attempt) ||
			!semanticStableBrainEqual(value.Brain, state.frozenBrain) {
			return nil, fmt.Errorf("semantic replay Brain bootstrap tuple changed")
		}
		if _, duplicate := state.replayBootstraps[value.SourceID]; duplicate {
			return nil, fmt.Errorf("semantic replay Brain bootstrap is duplicated")
		}
		state.replayBootstraps[value.SourceID] = value
	case queue.CognitionBrainBootstrapActivationFailure:
		if record.Phase != 3 || record.Sequence < 1 ||
			!semanticStableBrainEqual(value.Brain, state.frozenBrain) {
			return nil, fmt.Errorf("semantic failed-activation Brain bootstrap tuple changed")
		}
		if _, duplicate := state.activationBootstraps[value.SourceID]; duplicate {
			return nil, fmt.Errorf("semantic failed-activation Brain bootstrap is duplicated")
		}
		state.activationBootstraps[value.SourceID] = semanticActivationBootstrap{
			trace: value, sequence: record.Sequence,
		}
	default:
		return nil, fmt.Errorf("semantic provider Brain bootstrap source is not registered")
	}
	draft := sourceKnowledgeDraft(
		cognitionreplay.EventProviderProcessObserved, source,
		cognitionreplay.KnowledgeEvidence, cognitionreplay.KnowledgeActive,
		cognitionreplay.AuthorityTool,
	)
	draft.Knowledge.Ref = "provider-bootstrap://" + value.SourceID
	return []semanticEventDraft{draft}, nil
}

func (state *semanticReplayState) mapProviderProcessObservation(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value cognitionpolicy.ProviderProcessObservation
	if err := decodeProductionPayload(record.Payload, &value, "semantic provider process observation"); err != nil {
		return nil, err
	}
	if err := value.ValidateFor(state.frozenBrain); err != nil || value.ID != record.ID ||
		value.EpisodeID != state.trace.Header.EpisodeID || record.CallOrdinal != 0 ||
		record.Phase != 5 || record.Sequence < 1 {
		return nil, fmt.Errorf("invalid semantic provider process observation: %v", err)
	}
	if record.Sequence != int64(len(state.providerProcesses)+1) {
		return nil, fmt.Errorf("semantic provider process observation sequence is not contiguous")
	}
	if _, duplicate := state.providerProcesses[record.Sequence]; duplicate {
		return nil, fmt.Errorf("semantic provider process observation is duplicated")
	}
	if record.Sequence == 1 {
		if state.initialProviderObservation || !state.initialBrainBootstrap ||
			value.Actor != state.initialActor {
			return nil, fmt.Errorf("semantic replay initial provider process observation changed")
		}
		state.initialProviderObservation = true
	}
	state.providerProcesses[record.Sequence] = value
	draft := sourceKnowledgeDraft(
		cognitionreplay.EventProviderProcessObserved, source,
		cognitionreplay.KnowledgeEvidence, cognitionreplay.KnowledgeActive,
		cognitionreplay.AuthorityTool,
	)
	draft.Knowledge.Ref = "provider-observation://" + value.ID
	return []semanticEventDraft{draft}, nil
}

func (state *semanticReplayState) mapProviderActivationFailure(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var envelope struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(record.Payload, &envelope); err != nil || envelope.Schema == "" {
		return nil, fmt.Errorf("invalid semantic provider activation failure envelope: %v", err)
	}
	switch envelope.Schema {
	case cognitionpolicy.ProviderProcessFailureSchemaV1:
		var value cognitionpolicy.ProviderProcessFailureReceipt
		if err := decodeProductionPayload(record.Payload, &value, "semantic provider process failure"); err != nil {
			return nil, err
		}
		bootstrapTrace, hasBootstrap := state.activationBootstraps[record.ID]
		failureEvidence, hasFailureEvidence := state.evidence.identity[value.Evidence.ID]
		bootstrapEvidence, hasBootstrapEvidence := state.evidence.identity[bootstrapTrace.trace.Evidence.ID]
		bootstrap, bootstrapErr := cognitionpolicy.NewBrainBootstrap(
			bootstrapTrace.trace.Brain, bootstrapEvidence,
		)
		failure := cognitionpolicy.ProviderProcessFailure{
			Receipt: value, IdentityEvidence: failureEvidence,
		}
		if !hasBootstrap || !hasFailureEvidence || !hasBootstrapEvidence ||
			bootstrapErr != nil || record.CallOrdinal != 0 || record.Phase != 4 ||
			record.Sequence < 1 || record.Sequence != bootstrapTrace.sequence ||
			bootstrapTrace.trace.Actor != value.Actor ||
			!semanticStableBrainEqual(bootstrapTrace.trace.Brain, state.frozenBrain) ||
			queue.VerifyCognitionProviderProcessFailureTraceIdentity(
				record.ID, bootstrap, failure,
			) != nil {
			return nil, fmt.Errorf("invalid semantic provider process failure authority")
		}
		if !semanticProviderFailureCode(value.Code) ||
			value.Purpose != cognitionpolicy.ProviderProcessEpisodeInvocation {
			return nil, fmt.Errorf("semantic provider process failure disposition changed")
		}
		if _, duplicate := state.activationFailures[record.ID]; duplicate {
			return nil, fmt.Errorf("semantic provider process failure is duplicated")
		}
		state.activationFailures[record.ID] = semanticActivationFailure{
			record: record, bootstrap: bootstrap, failure: failure,
		}
	default:
		return nil, fmt.Errorf("sealed provider activation failure schema %q is not registered", envelope.Schema)
	}
	draft := sourceKnowledgeDraft(
		cognitionreplay.EventFailureRecorded, source,
		cognitionreplay.KnowledgeFailure, cognitionreplay.KnowledgeFailed,
		cognitionreplay.AuthorityCode,
	)
	draft.Knowledge.Ref = "provider-failure://" + record.ID
	return []semanticEventDraft{draft}, nil
}

func semanticStableBrainEqual(left, right cognitionpolicy.AttestedBrain) bool {
	leftStable, leftErr := left.StableAuthority()
	rightStable, rightErr := right.StableAuthority()
	return leftErr == nil && rightErr == nil && leftStable == rightStable
}

func semanticProviderFailureCode(code cognitionpolicy.ProviderIdentityFailureCode) bool {
	switch code {
	case cognitionpolicy.ProviderIdentityObservationFailed,
		cognitionpolicy.ProviderIdentityObservationInvalid,
		cognitionpolicy.ProviderAttestationIdentityMismatch,
		cognitionpolicy.ProviderHostAttestationFailed,
		cognitionpolicy.ProviderHostIdentityMismatch:
		return true
	default:
		return false
	}
}
