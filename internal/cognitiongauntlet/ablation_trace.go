package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
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
	projectionTrace := newAblationProjectionTrace(projection)
	payload, err := traceJSONObject(projectionTrace)
	if err != nil {
		return err
	}
	if err := recorder.Append(TraceProjection, projection.ID, nil, payload); err != nil {
		return err
	}
	policyTrace, err := newAblationPolicyTracePayload(projection, call)
	if err != nil {
		return err
	}
	if err := recorder.Append(policyTrace.kind, call.Attempt.ID, nil, policyTrace.payload); err != nil {
		return err
	}
	if policyTrace.modelCall == nil {
		resources.PolicyCallsConsumed++
		return nil
	}
	callTrace := *policyTrace.modelCall
	resources.PolicyCallsConsumed++
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
	if acceptedAblationCall(call.Result) {
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
