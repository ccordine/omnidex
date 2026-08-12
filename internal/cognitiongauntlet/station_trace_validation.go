package cognitiongauntlet

import "fmt"

type stationTraceState struct {
	pending        *ProjectionTrace
	policyCalls    int
	modelCalls     int
	inputBytes     int64
	inputTokens    int64
	outputBytes    int64
	outputTokens   int64
	peakInput      int64
	providerTotal  int64
	providerLoad   int64
	providerPrompt int64
	providerEval   int64
}

func (state *stationTraceState) acceptProjection(entry TraceEntry) error {
	if state.pending != nil {
		return fmt.Errorf("cognition trace replaced an unused Context Projection")
	}
	var projection ProjectionTrace
	if err := decodeTracePayload(entry.Payload, &projection, "Context Projection trace"); err != nil {
		return err
	}
	if err := projection.Validate(); err != nil {
		return err
	}
	if projection.ProjectionID != entry.ID {
		return fmt.Errorf("Context Projection trace entry changed its projection ID")
	}
	state.pending = &projection
	return nil
}

func (state *stationTraceState) acceptModelCall(entry TraceEntry, budget StationBudget) error {
	if state.pending == nil {
		return fmt.Errorf("cognition model call has no preceding Context Projection")
	}
	var call ModelCallTrace
	if err := decodeTracePayload(entry.Payload, &call, "model-call trace"); err != nil {
		return err
	}
	if err := call.Validate(); err != nil {
		return err
	}
	if call.Budget != budget {
		return fmt.Errorf("model-call trace changed its sealed station budget")
	}
	if call.ProjectionID != state.pending.ProjectionID ||
		call.ProjectionSHA256 != state.pending.ProjectionSHA256 ||
		call.InputBytes < state.pending.RenderedBytes {
		return fmt.Errorf("model-call trace does not bind its preceding Context Projection")
	}
	state.modelCalls++
	state.policyCalls++
	state.inputBytes += call.InputBytes
	state.inputTokens += call.InputTokens
	state.outputBytes += call.OutputBytes
	state.outputTokens += call.OutputTokens
	state.providerTotal += call.ProviderUsage.TotalDurationNanos
	state.providerLoad += call.ProviderUsage.LoadDurationNanos
	state.providerPrompt += call.ProviderUsage.PromptEvalDurationNanos
	state.providerEval += call.ProviderUsage.EvalDurationNanos
	if call.InputBytes > state.peakInput {
		state.peakInput = call.InputBytes
	}
	state.pending = nil
	return nil
}

func (state *stationTraceState) acceptPolicyDisposition(entry TraceEntry, budget StationBudget) error {
	if state.pending == nil {
		return fmt.Errorf("non-inference policy disposition has no preceding Context Projection")
	}
	var disposition PolicyDispositionTrace
	if err := decodeTracePayload(entry.Payload, &disposition, "policy disposition trace"); err != nil {
		return err
	}
	if err := disposition.Validate(); err != nil {
		return err
	}
	if disposition.Budget != budget || disposition.ProjectionID != state.pending.ProjectionID ||
		disposition.ProjectionSHA256 != state.pending.ProjectionSHA256 {
		return fmt.Errorf("non-inference policy disposition changed its projection or budget authority")
	}
	state.policyCalls++
	state.pending = nil
	return nil
}

func (state stationTraceState) validateResources(resources Resources) error {
	if state.pending != nil {
		return fmt.Errorf("cognition trace did not consume its final Context Projection")
	}
	if state.policyCalls != resources.PolicyCallsConsumed ||
		state.modelCalls != resources.ModelCalls || state.inputBytes != resources.ContextBytes ||
		state.inputTokens != resources.InputTokens || state.outputBytes != resources.OutputBytes ||
		state.outputTokens != resources.OutputTokens || state.peakInput != resources.PeakContextBytes ||
		state.providerTotal != resources.ProviderTotalNanoseconds ||
		state.providerLoad != resources.ProviderLoadNanoseconds ||
		state.providerPrompt != resources.ProviderPromptEvalNanoseconds ||
		state.providerEval != resources.ProviderEvalNanoseconds {
		return fmt.Errorf("sealed station calls do not match aggregate resource metrics")
	}
	return nil
}
