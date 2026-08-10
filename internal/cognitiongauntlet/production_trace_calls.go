package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/taskstate"
)

func appendProductionProjection(
	recorder *EpisodeRecorder,
	record queue.CognitionSealedTraceRecord,
) error {
	var projection contextbuilder.Projection
	if err := decodeProductionPayload(record.Payload, &projection, "Context Projection"); err != nil {
		return err
	}
	if err := projection.Validate(); err != nil {
		return fmt.Errorf("validate sealed production Context Projection: %w", err)
	}
	selected := make([]ProjectedReference, len(projection.Selected))
	for index, item := range projection.Selected {
		sources := make([]ProjectionReferenceIdentity, len(item.SourceRefs))
		for sourceIndex, source := range item.SourceRefs {
			sources[sourceIndex] = projectionReference(source)
		}
		selected[index] = ProjectedReference{
			Ref: projectionReference(item.Ref), SourceRefs: sources,
			RenderedBytes: int64(item.RenderedBytes),
		}
	}
	trace := ProjectionTrace{
		Schema: ProjectionTraceSchemaV1, ProjectionID: projection.ID,
		ProjectionSHA256: projection.RenderedSHA256,
		RenderedBytes:    int64(projection.RenderedBytes),
		EstimatedTokens:  int64(projection.EstimatedTokens),
		TokenEstimator:   projection.TokenEstimator, Selected: selected,
	}
	if err := trace.Validate(); err != nil {
		return err
	}
	payload, err := traceJSONObject(trace)
	if err != nil {
		return err
	}
	return recorder.Append(TraceProjection, projection.ID, nil, payload)
}

func (state *productionTraceState) acceptPolicyResult(
	recorder *EpisodeRecorder,
	record queue.CognitionSealedTraceRecord,
) error {
	var result cognitionpolicy.CallResult
	if err := decodeProductionPayload(record.Payload, &result, "policy result"); err != nil {
		return err
	}
	attempt, exists := state.attempts[result.CallID]
	if !exists {
		return fmt.Errorf("sealed production policy result has no exact preceding attempt")
	}
	if err := result.Validate(attempt); err != nil {
		return err
	}
	if result.ProviderIdentityChecked && result.ProviderAttestation != state.frozenBrain.Attestation {
		return fmt.Errorf("sealed production policy result changed the frozen provider identity")
	}
	if _, duplicate := state.results[result.CallID]; duplicate {
		return fmt.Errorf("sealed production policy result is duplicated")
	}
	state.results[result.CallID] = result.Status
	budget := StationBudget{
		MaxInputBytes:   attempt.RuntimeBudget.MaxInputBytes,
		MaxInputTokens:  attempt.RuntimeBudget.MaxInputTokens,
		MaxOutputBytes:  attempt.RuntimeBudget.MaxOutputBytes,
		MaxOutputTokens: attempt.RuntimeBudget.MaxOutputTokens,
	}
	if !result.ProviderRequestDispatched {
		return appendProductionPolicyDisposition(recorder, attempt, result, budget)
	}
	call := ModelCallTrace{
		Schema:           ModelCallTraceSchemaV2,
		ProjectionID:     string(attempt.ContextProjection.ID),
		ProjectionSHA256: attempt.ContextProjection.SHA256,
		Budget:           budget, ResultStatus: result.Status, FailureCode: result.FailureCode,
		ProviderResponseDisposition: result.ProviderResponseDisposition,
		ProviderRequestDispatched:   result.ProviderRequestDispatched,
		ProviderDoneReason:          result.ProviderDoneReason,
		ProviderUsagePresent:        result.ProviderUsagePresent, ProviderUsage: result.ProviderUsage,
		InputBytes: int64(attempt.ModelVisibleInputBytes), OutputBytes: int64(result.ResponseBytes),
	}
	if result.ProviderUsagePresent {
		call.InputTokens = int64(result.ProviderUsage.PromptEvalCount)
		call.OutputTokens = int64(result.ProviderUsage.EvalCount)
	}
	if err := call.Validate(); err != nil {
		return err
	}
	payload, err := traceJSONObject(call)
	if err != nil {
		return err
	}
	if err := recorder.Append(TraceModelCall, attempt.ID, nil, payload); err != nil {
		return err
	}
	resources := &state.metrics.Resources
	resources.ModelCalls++
	resources.ContextBytes += call.InputBytes
	resources.InputTokens += call.InputTokens
	resources.OutputBytes += int64(result.ResponseBytes)
	resources.OutputTokens += call.OutputTokens
	resources.ProviderTotalNanoseconds += result.ProviderUsage.TotalDurationNanos
	resources.ProviderLoadNanoseconds += result.ProviderUsage.LoadDurationNanos
	resources.ProviderPromptEvalNanoseconds += result.ProviderUsage.PromptEvalDurationNanos
	resources.ProviderEvalNanoseconds += result.ProviderUsage.EvalDurationNanos
	if call.InputBytes > resources.PeakContextBytes {
		resources.PeakContextBytes = call.InputBytes
	}
	if result.Status == cognitionpolicy.CallResultAccepted {
		resources.ModelDecisions++
	}
	return nil
}

func appendProductionPolicyDisposition(
	recorder *EpisodeRecorder,
	attempt cognitionpolicy.CallAttempt,
	result cognitionpolicy.CallResult,
	budget StationBudget,
) error {
	disposition := PolicyDispositionTrace{
		Schema:           PolicyDispositionSchemaV1,
		ProjectionID:     string(attempt.ContextProjection.ID),
		ProjectionSHA256: attempt.ContextProjection.SHA256,
		Budget:           budget, ResultStatus: result.Status,
		FailureCode: result.FailureCode, ProviderRequestDispatched: false,
	}
	if err := disposition.Validate(); err != nil {
		return err
	}
	payload, err := traceJSONObject(disposition)
	if err != nil {
		return err
	}
	return recorder.Append(TracePolicyDisposition, attempt.ID, nil, payload)
}

func projectionReference(ref taskstate.Ref) ProjectionReferenceIdentity {
	return ProjectionReferenceIdentity{URI: ref.URI, Version: ref.Version, ContentSHA256: ref.Hash}
}
