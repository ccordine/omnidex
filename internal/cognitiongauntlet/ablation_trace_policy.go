package cognitiongauntlet

import (
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/taskstate"
)

func newAblationProjectionTrace(projection contextbuilder.Projection) ProjectionTrace {
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
	return ProjectionTrace{
		Schema: ProjectionTraceSchemaV1, ProjectionID: projection.ID,
		ProjectionSHA256: projection.RenderedSHA256,
		RenderedBytes:    int64(projection.RenderedBytes),
		EstimatedTokens:  int64(projection.EstimatedTokens),
		TokenEstimator:   projection.TokenEstimator,
		Selected:         selected,
	}
}

type ablationPolicyTracePayload struct {
	kind      TraceKind
	payload   taskstate.JSONObject
	modelCall *ModelCallTrace
}

func newAblationPolicyTracePayload(
	projection contextbuilder.Projection,
	call ablationCall,
) (ablationPolicyTracePayload, error) {
	budget := StationBudget{
		MaxInputBytes:   call.Attempt.RuntimeBudget.MaxInputBytes,
		MaxInputTokens:  call.Attempt.RuntimeBudget.MaxInputTokens,
		MaxOutputBytes:  call.Attempt.RuntimeBudget.MaxOutputBytes,
		MaxOutputTokens: call.Attempt.RuntimeBudget.MaxOutputTokens,
	}
	if call.Result.ProviderRequestDisposition != llm.ProviderRequestDispatched {
		disposition := PolicyDispositionTrace{
			Schema: PolicyDispositionSchemaV3, Disposition: PolicyCallResultDisposition,
			ProjectionID: projection.ID, ProjectionSHA256: projection.RenderedSHA256,
			Budget: budget, ResultStatus: call.Result.Status,
			FailureCode:                call.Result.FailureCode,
			ProviderRequestDisposition: call.Result.ProviderRequestDisposition,
		}
		if err := disposition.Validate(); err != nil {
			return ablationPolicyTracePayload{}, err
		}
		payload, err := traceJSONObject(disposition)
		return ablationPolicyTracePayload{kind: TracePolicyDisposition, payload: payload}, err
	}
	trace := ModelCallTrace{
		Schema: ModelCallTraceSchemaV4, ProjectionID: projection.ID,
		ProjectionSHA256: projection.RenderedSHA256, Budget: budget,
		ResultStatus: call.Result.Status, FailureCode: call.Result.FailureCode,
		ProviderResponseDisposition: call.Result.ProviderResponseDisposition,
		ProviderRequestDisposition:  call.Result.ProviderRequestDisposition,
		ProviderDoneReason:          call.Result.ProviderDoneReason,
		ProviderUsagePresent:        call.Result.ProviderUsagePresent,
		ProviderUsage:               call.Result.ProviderUsage,
		InputBytes:                  int64(call.Attempt.ModelVisibleInputBytes),
		OutputBytes:                 int64(call.Result.ResponseBytes),
	}
	if call.Result.ProviderUsagePresent {
		trace.InputTokens = int64(call.Result.ProviderUsage.PromptEvalCount)
		trace.OutputTokens = int64(call.Result.ProviderUsage.EvalCount)
	}
	payload, err := traceJSONObject(trace)
	if err != nil {
		return ablationPolicyTracePayload{}, err
	}
	return ablationPolicyTracePayload{
		kind: TraceModelCall, payload: payload, modelCall: &trace,
	}, nil
}

func ablationCallFromEvidence(value ablationCallEvidence) ablationCall {
	return ablationCall{
		Attempt: value.Attempt, Result: value.Result,
	}
}

func acceptedAblationCall(result cognitionpolicy.CallResult) bool {
	return result.Status == cognitionpolicy.CallResultAccepted
}
