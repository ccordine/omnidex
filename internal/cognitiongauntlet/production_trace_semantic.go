package cognitiongauntlet

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/queue"
)

const ProductionTraceRecordSchemaV1 = "omnidex.gauntlet-production-record.v1"

type productionRecordPayload struct {
	Schema      string          `json:"schema"`
	Kind        string          `json:"kind"`
	CallOrdinal int64           `json:"call_ordinal"`
	Phase       int             `json:"phase"`
	Sequence    int64           `json:"sequence"`
	ID          string          `json:"id"`
	SHA256      string          `json:"sha256"`
	Payload     json.RawMessage `json:"payload"`
}

type productionTraceMetrics struct {
	Resources Resources
	Memory    MemoryMetrics
	Planning  PlanningMetrics
	Recovery  RecoveryMetrics
	Outcome   Outcome
}

func appendProductionTrace(
	recorder *EpisodeRecorder,
	trace productionTrace,
	recovery RecoveryMetrics,
	restarts []RestartTrace,
) (productionTraceMetrics, error) {
	frozen, err := recorder.template.RatGeneration.Fixed.Brain.attestedBrain()
	if err != nil {
		return productionTraceMetrics{}, err
	}
	state := newProductionTraceState(trace, recovery, frozen)
	if state.initErr != nil {
		return productionTraceMetrics{}, state.initErr
	}
	for _, record := range trace.Records {
		if err := state.accept(recorder, record); err != nil {
			return productionTraceMetrics{}, err
		}
	}
	return state.finish(recorder, restarts)
}

type productionTraceState struct {
	trace             productionTrace
	attempts          map[string]cognitionpolicy.CallAttempt
	results           map[string]cognitionpolicy.CallResultStatus
	abandonments      map[string]cognitionruntime.PolicyCallAbandonment
	policyProjections map[string]struct{}
	actions           map[cognition.ActionID]queue.CognitionTraceAction
	consumedActions   map[cognition.ActionID]struct{}
	diagnostics       productionTraceDiagnostics
	metrics           productionTraceMetrics
	terminalProgress  *cognitionruntime.EpisodeProgress
	cancellation      *cognitionruntime.CancellationEvidence
	frozenBrain       cognitionpolicy.AttestedBrain
	initErr           error
}

func newProductionTraceState(
	trace productionTrace,
	recovery RecoveryMetrics,
	frozen cognitionpolicy.AttestedBrain,
) *productionTraceState {
	projections := make(map[string]struct{})
	for _, record := range trace.Records {
		if record.Kind == "policy_attempt" {
			var attempt cognitionpolicy.CallAttempt
			if err := decodeProductionPayload(record.Payload, &attempt, "policy attempt authority"); err != nil {
				return &productionTraceState{initErr: err}
			}
			projections[string(attempt.ContextProjection.ID)] = struct{}{}
		}
	}
	return &productionTraceState{
		trace:             trace,
		attempts:          make(map[string]cognitionpolicy.CallAttempt),
		results:           make(map[string]cognitionpolicy.CallResultStatus),
		abandonments:      make(map[string]cognitionruntime.PolicyCallAbandonment),
		policyProjections: projections,
		actions:           make(map[cognition.ActionID]queue.CognitionTraceAction),
		consumedActions:   make(map[cognition.ActionID]struct{}),
		diagnostics:       newProductionTraceDiagnostics(),
		metrics:           productionTraceMetrics{Recovery: recovery}, frozenBrain: frozen,
	}
}

func (state *productionTraceState) accept(
	recorder *EpisodeRecorder,
	record queue.CognitionSealedTraceRecord,
) error {
	switch record.Kind {
	case "transition":
		if err := state.acceptTransition(recorder, record); err != nil {
			return err
		}
	case "context_projection":
		if _, used := state.policyProjections[record.ID]; used {
			if err := appendProductionProjection(recorder, record); err != nil {
				return err
			}
		}
	case "policy_attempt":
		var attempt cognitionpolicy.CallAttempt
		if err := decodeProductionPayload(record.Payload, &attempt, "policy attempt"); err != nil {
			return err
		}
		if err := attempt.Validate(); err != nil {
			return err
		}
		if attempt.Brain != state.frozenBrain.Ref ||
			attempt.ProviderAttestation != state.frozenBrain.Attestation ||
			attempt.HostHardwareAttestation != state.frozenBrain.Host {
			return fmt.Errorf("sealed production policy attempt changed the frozen Rat identity")
		}
		if _, duplicate := state.attempts[attempt.ID]; duplicate {
			return fmt.Errorf("sealed production policy attempt is duplicated")
		}
		state.attempts[attempt.ID] = attempt
	case "policy_result":
		if err := state.acceptPolicyResult(recorder, record); err != nil {
			return err
		}
	case "policy_abandonment":
		var abandonment cognitionruntime.PolicyCallAbandonment
		if err := decodeProductionPayload(record.Payload, &abandonment, "policy abandonment"); err != nil {
			return err
		}
		if err := abandonment.Validate(); err != nil || record.ID != abandonment.ID {
			return fmt.Errorf("sealed production policy abandonment is invalid")
		}
		attempt, exists := state.attempts[abandonment.CallID]
		if !exists {
			return fmt.Errorf("sealed production policy abandonment has no exact attempt")
		}
		if _, hasResult := state.results[abandonment.CallID]; hasResult {
			return fmt.Errorf("sealed production policy call has both a result and abandonment")
		}
		if _, duplicate := state.abandonments[abandonment.CallID]; duplicate {
			return fmt.Errorf("sealed production policy abandonment is duplicated")
		}
		state.abandonments[abandonment.CallID] = abandonment
		if err := appendProductionPolicyAbandonment(recorder, attempt, abandonment.ID); err != nil {
			return err
		}
		state.metrics.Resources.PolicyCallsConsumed++
	case "policy_timing", "working_set_snapshot", "working_set_event",
		"accepted_decision_recovery":
		if err := state.diagnostics.accept(record, state.trace.Header); err != nil {
			return err
		}
	case "action":
		if err := state.acceptAction(recorder, record); err != nil {
			return err
		}
	case "obligation_graph":
		var graph cognition.ObligationGraphSnapshot
		if err := decodeProductionPayload(record.Payload, &graph, "obligation graph"); err != nil {
			return err
		}
		if err := graph.Validate(); err != nil {
			return err
		}
		state.measureGraph(graph)
	case "episode_progress":
		var progress cognitionruntime.EpisodeProgress
		if err := decodeProductionPayload(record.Payload, &progress, "episode progress"); err != nil {
			return err
		}
		if progress.State == cognitionruntime.ProgressCompleted || progress.State == cognitionruntime.ProgressFailed {
			copy := progress
			state.terminalProgress = &copy
		}
	case "cancellation_evidence":
		var evidence cognitionruntime.CancellationEvidence
		if err := decodeProductionPayload(record.Payload, &evidence, "cancellation evidence"); err != nil {
			return err
		}
		if err := evidence.Validate(); err != nil || state.cancellation != nil || record.ID != evidence.ID {
			return fmt.Errorf("sealed production cancellation evidence is invalid or duplicated")
		}
		copy := evidence
		state.cancellation = &copy
	}
	return appendRawProductionRecord(recorder, record)
}

func decodeProductionPayload(raw []byte, target any, label string) error {
	return decodeStrictJSON(raw, target, "sealed production "+label)
}

func appendRawProductionRecord(
	recorder *EpisodeRecorder,
	record queue.CognitionSealedTraceRecord,
) error {
	payload := productionRecordPayload{
		Schema: ProductionTraceRecordSchemaV1, Kind: record.Kind,
		CallOrdinal: record.CallOrdinal, Phase: record.Phase, Sequence: record.Sequence,
		ID: record.ID, SHA256: record.SHA256, Payload: append(json.RawMessage(nil), record.Payload...),
	}
	digest, err := digestJSON(struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
		SHA  string `json:"sha"`
	}{record.Kind, record.ID, record.SHA256})
	if err != nil {
		return err
	}
	object, err := traceJSONObject(payload)
	if err != nil {
		return err
	}
	return recorder.Append(productionTraceKind(record.Kind), "production-"+digest, nil, object)
}

func productionTraceKind(kind string) TraceKind {
	switch kind {
	case "cancellation_evidence":
		return TraceFailure
	case "obligation_graph", "episode_progress", "episode_progress_command":
		return TraceObligation
	case "runtime_snapshot", "context_projection":
		return TraceWorkingSet
	case "action_event":
		return TraceLease
	default:
		return TraceLedger
	}
}
