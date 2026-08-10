package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/taskstate"
)

type ablationFailureRecord struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ablationTerminalRecord struct {
	Revision      cognition.WorldRevision `json:"revision"`
	PublicOutcome string                  `json:"public_outcome"`
	GoalSatisfied bool                    `json:"goal_satisfied"`
}

func appendAblationPolicyTrace(
	recorder *EpisodeRecorder,
	projection contextbuilder.Projection,
	call ablationCall,
	resources *Resources,
) error {
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
	projectionTrace := ProjectionTrace{
		Schema: ProjectionTraceSchemaV1, ProjectionID: projection.ID,
		ProjectionSHA256: projection.RenderedSHA256,
		RenderedBytes:    int64(projection.RenderedBytes), EstimatedTokens: int64(projection.EstimatedTokens),
		TokenEstimator: projection.TokenEstimator, Selected: selected,
	}
	payload, err := traceJSONObject(projectionTrace)
	if err != nil {
		return err
	}
	if err := recorder.Append(TraceProjection, projection.ID, nil, payload); err != nil {
		return err
	}
	budget := StationBudget{
		MaxInputBytes:   call.Attempt.RuntimeBudget.MaxInputBytes,
		MaxInputTokens:  call.Attempt.RuntimeBudget.MaxInputTokens,
		MaxOutputBytes:  call.Attempt.RuntimeBudget.MaxOutputBytes,
		MaxOutputTokens: call.Attempt.RuntimeBudget.MaxOutputTokens,
	}
	if !call.Result.ProviderRequestDispatched {
		disposition := PolicyDispositionTrace{
			Schema: PolicyDispositionSchemaV1, ProjectionID: projection.ID,
			ProjectionSHA256: projection.RenderedSHA256, Budget: budget,
			ResultStatus: call.Result.Status, FailureCode: call.Result.FailureCode,
			ProviderRequestDispatched: false,
		}
		if err := disposition.Validate(); err != nil {
			return err
		}
		payload, err = traceJSONObject(disposition)
		if err != nil {
			return err
		}
		return recorder.Append(TracePolicyDisposition, call.Attempt.ID, nil, payload)
	}
	callTrace := ModelCallTrace{
		Schema: ModelCallTraceSchemaV2, ProjectionID: projection.ID,
		ProjectionSHA256: projection.RenderedSHA256,
		Budget:           budget, ResultStatus: call.Result.Status, FailureCode: call.Result.FailureCode,
		ProviderResponseDisposition: call.Result.ProviderResponseDisposition,
		ProviderRequestDispatched:   call.Result.ProviderRequestDispatched,
		ProviderDoneReason:          call.Result.ProviderDoneReason,
		ProviderUsagePresent:        call.Result.ProviderUsagePresent, ProviderUsage: call.Result.ProviderUsage,
		InputBytes:  int64(call.Attempt.ModelVisibleInputBytes),
		OutputBytes: int64(call.Result.ResponseBytes),
	}
	if call.Result.ProviderUsagePresent {
		callTrace.InputTokens = int64(call.Result.ProviderUsage.PromptEvalCount)
		callTrace.OutputTokens = int64(call.Result.ProviderUsage.EvalCount)
	}
	payload, err = traceJSONObject(callTrace)
	if err != nil {
		return err
	}
	if err := recorder.Append(TraceModelCall, call.Attempt.ID, nil, payload); err != nil {
		return err
	}
	resources.ModelCalls++
	resources.ContextBytes += callTrace.InputBytes
	resources.InputTokens += callTrace.InputTokens
	resources.OutputBytes += callTrace.OutputBytes
	resources.OutputTokens += callTrace.OutputTokens
	resources.ProviderTotalNanoseconds += call.Result.ProviderUsage.TotalDurationNanos
	resources.ProviderLoadNanoseconds += call.Result.ProviderUsage.LoadDurationNanos
	resources.ProviderPromptEvalNanoseconds += call.Result.ProviderUsage.PromptEvalDurationNanos
	resources.ProviderEvalNanoseconds += call.Result.ProviderUsage.EvalDurationNanos
	if callTrace.InputBytes > resources.PeakContextBytes {
		resources.PeakContextBytes = callTrace.InputBytes
	}
	if call.Result.Status == cognitionpolicy.CallResultAccepted {
		resources.ModelDecisions++
	}
	return nil
}

func appendAblationActionFailure(
	recorder *EpisodeRecorder,
	action cognition.RegisteredAction,
	expected cognition.WorldRevision,
	failure cognition.ActionFailure,
) error {
	payload, err := traceJSONObject(ActionTrace{
		Schema: ActionTraceSchemaV1, Action: action.Clone(), ExpectedRevision: expected,
		Failure: &failure,
	})
	if err != nil {
		return err
	}
	return recorder.Append(TraceAction, string(action.ID), &expected, payload)
}

func appendAblationFailure(
	recorder *EpisodeRecorder,
	id string,
	revision cognition.WorldRevision,
	code string,
	message string,
) error {
	payload, err := traceJSONObject(ablationFailureRecord{Code: code, Message: message})
	if err != nil {
		return err
	}
	return recorder.Append(TraceFailure, id, &revision, payload)
}

func appendAblationTerminal(
	recorder *EpisodeRecorder,
	revision cognition.WorldRevision,
	publicOutcome string,
	goalSatisfied bool,
) error {
	payload, err := traceJSONObject(ablationTerminalRecord{
		Revision: revision, PublicOutcome: publicOutcome, GoalSatisfied: goalSatisfied,
	})
	if err != nil {
		return err
	}
	return recorder.Append(TraceTerminal, "terminal-"+revision.SHA256, &revision, payload)
}

func selectedAblationEvidence(
	projection contextbuilder.Projection,
	available []cognition.EvidenceRef,
) []cognition.EvidenceRef {
	selected := make(map[string]struct{})
	for _, item := range projection.Selected {
		selected[taskstate.RefIdentity(item.Ref)] = struct{}{}
		for _, source := range item.SourceRefs {
			selected[taskstate.RefIdentity(source)] = struct{}{}
		}
	}
	result := make([]cognition.EvidenceRef, 0, len(available))
	for _, evidence := range available {
		ref := taskstate.Ref{
			URI: "cognition:episode/" + string(evidence.Revision.EpisodeID) +
				"/observation/" + string(evidence.ObservationID),
			Version: fmt.Sprint(evidence.Revision.Number), Hash: evidence.SHA256,
			Relation: taskstate.RefEvidence,
		}
		if _, exists := selected[taskstate.RefIdentity(ref)]; exists {
			result = append(result, evidence)
		}
	}
	return result
}
