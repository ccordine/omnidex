package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

// continueExactStationAfterOutputLimit repeats one incomplete provider call
// with only its code-owned num_predict authority enlarged. The semantic job,
// prompt, immutable model route, source-correction evidence, and framing remain
// byte-identical. A second length completion is a hard context failure.
func (s *Service) continueExactStationAfterOutputLimit(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	call exactStationCall,
	prepared llm.PreparedModel,
	parent queue.LLMCallEvidence,
	failure *llm.ExactPreparedOutputLimitReachedError,
) (assemblyline.PortableResult, exactStationExecution, error) {
	if failure == nil || failure.Validate() != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"exact station output continuation lacks validated limit evidence",
		)
	}
	if call.OutputContinuation != 0 || parent.ID < 1 || parent.Outcome == nil ||
		parent.ID != parent.Outcome.CallEvidenceID || !parent.OutputLimitReached ||
		parent.Status != queue.LLMCallFailed ||
		parent.Outcome.Status != queue.LLMCallProviderFailed {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"exact station output continuation parent is not one incomplete provider result",
		)
	}
	if parent.WorkID != call.WorkID || parent.WorkKind != string(call.WorkKind) ||
		parent.Iteration != call.Iteration || parent.OutputContinuation != 0 ||
		parent.Model != prepared.BaseModel || parent.RequestedModel != prepared.BaseModel ||
		parent.SystemEnvelope != call.Prompt || parent.ContextTokens != prepared.ContextTokens ||
		parent.MaxOutputTokens != prepared.MaxOutputTokens {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"exact station output continuation differs from its persisted job, model, prompt, or token authority",
		)
	}
	nextMaximum := prepared.ContextTokens - failure.PromptTokens
	if nextMaximum <= prepared.MaxOutputTokens || nextMaximum >= prepared.ContextTokens {
		return assemblyline.PortableResult{}, exactStationExecution{ProviderCalls: 1}, fmt.Errorf(
			"exact station output reached its hard native-context authority: prompt_tokens=%d output_tokens=%d native_context=%d",
			failure.PromptTokens, failure.OutputTokens, failure.ContextTokens,
		)
	}
	nextCall := call
	nextCall.ParentCallID = parent.ID
	nextCall.OutputContinuation = 1
	nextCall.MaxOutputTokens = nextMaximum
	nextPrepared, err := prepareExactStationCall(
		nextCall, prepared.BaseModel, prepared.Temperature,
	)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{ProviderCalls: 1}, err
	}
	if nextPrepared.Prompt != prepared.Prompt || nextPrepared.BaseModel != prepared.BaseModel ||
		nextPrepared.ContextModel != prepared.ContextModel ||
		nextPrepared.ContextTokens != prepared.ContextTokens ||
		nextPrepared.RawTextStopSequence != prepared.RawTextStopSequence ||
		nextPrepared.OutputLimitMode != prepared.OutputLimitMode ||
		nextPrepared.MaxOutputTokens != nextMaximum {
		return assemblyline.PortableResult{}, exactStationExecution{ProviderCalls: 1}, fmt.Errorf(
			"exact station output continuation changed authority other than num_predict",
		)
	}
	result, execution, err := s.dispatchExactStationCall(
		ctx, authority, nextCall, nextPrepared,
	)
	execution.ProviderCalls++
	return result, execution, err
}
