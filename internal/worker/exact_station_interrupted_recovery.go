package worker

import (
	"bytes"
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

// resumeInterruptedExactStationCall replaces one receipt-less physical
// dispatch after code-owned attempt expiration. It does not create a new
// semantic job, correction iteration, output continuation, prompt, model, or
// context authority. A replacement opening is itself terminal if interrupted.
func (s *Service) resumeInterruptedExactStationCall(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	job assemblyline.PortableJob,
	modelName string,
	evidence queue.LLMCallEvidence,
	correction *assemblyline.SourceBodyCorrection,
) (*exactStationRecovery, error) {
	if evidence.DispatchAttempt != 1 || evidence.ProviderReceiptPresent ||
		evidence.Outcome == nil || evidence.Outcome.Status != queue.LLMCallInterrupted ||
		evidence.ReplacesCallEvidenceID != 0 {
		return nil, fmt.Errorf(
			"persisted portable work %s exhausted or lacks one replaceable interrupted dispatch",
			job.ID,
		)
	}
	call, prepared, err := recreatePersistedExactStationCall(
		job, modelName, evidence, correction,
	)
	if err != nil {
		return nil, err
	}
	call.DispatchAttempt = 2
	call.ReplacesCallID = evidence.ID
	result, execution, dispatchErr := s.dispatchExactStationCall(
		ctx, authority, call, prepared,
	)
	if dispatchErr != nil {
		return &exactStationRecovery{Execution: execution, Evidence: evidence}, dispatchErr
	}
	continued, err := s.repo.GetLLMCallEvidence(ctx, execution.CallEvidenceID)
	if err != nil {
		return &exactStationRecovery{Execution: execution, Evidence: evidence}, err
	}
	return &exactStationRecovery{
		Result: result, Execution: execution, Evidence: continued,
	}, nil
}

func recreatePersistedExactStationCall(
	job assemblyline.PortableJob,
	modelName string,
	evidence queue.LLMCallEvidence,
	correction *assemblyline.SourceBodyCorrection,
) (exactStationCall, llm.PreparedModel, error) {
	prompt := ""
	singleLine := false
	var sourceCorrection *assemblyline.SourceBodyCorrectionEvidence
	if evidence.Iteration == 1 {
		if correction != nil {
			return exactStationCall{}, llm.PreparedModel{}, fmt.Errorf(
				"persisted portable work %s initial call cannot carry source correction state",
				job.ID,
			)
		}
		var err error
		prompt, err = assemblyline.RenderPortableJob(job)
		if err != nil {
			return exactStationCall{}, llm.PreparedModel{}, err
		}
		framing, err := assemblyline.PortableResponseFramingForJob(job)
		if err != nil {
			return exactStationCall{}, llm.PreparedModel{}, err
		}
		singleLine = framing == assemblyline.PortableResponseFramingSingleLine
	} else {
		if correction == nil {
			return exactStationCall{}, llm.PreparedModel{}, fmt.Errorf(
				"persisted portable work %s requires its recreated source correction",
				job.ID,
			)
		}
		if err := correction.Validate(); err != nil {
			return exactStationCall{}, llm.PreparedModel{}, err
		}
		persisted, err := correction.Evidence()
		if err != nil {
			return exactStationCall{}, llm.PreparedModel{}, err
		}
		if persisted != sourceCorrectionEvidenceFromCall(evidence) {
			return exactStationCall{}, llm.PreparedModel{}, fmt.Errorf(
				"persisted portable work %s differs from its recreated source correction",
				job.ID,
			)
		}
		prompt, err = correction.ModelInput()
		if err != nil {
			return exactStationCall{}, llm.PreparedModel{}, err
		}
		_, singleLine, err = correction.OpaqueResponseMaximumBytes()
		if err != nil {
			return exactStationCall{}, llm.PreparedModel{}, err
		}
		sourceCorrection = &persisted
	}
	call := exactStationCall{
		WorkID: job.ID, WorkKind: job.Kind, Iteration: evidence.Iteration,
		OutputContinuation: evidence.OutputContinuation,
		DispatchAttempt:    evidence.DispatchAttempt,
		ParentCallID:       evidence.ParentCallEvidenceID,
		ReplacesCallID:     evidence.ReplacesCallEvidenceID,
		Prompt:             prompt, ContextTokens: evidence.ContextTokens,
		MaxOutputTokens: evidence.MaxOutputTokens, SingleLine: singleLine,
		SourceCorrection: sourceCorrection,
	}
	prepared, err := prepareExactStationCall(call, modelName, nil)
	if err != nil {
		return exactStationCall{}, llm.PreparedModel{}, err
	}
	providerRequest, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		return exactStationCall{}, llm.PreparedModel{}, err
	}
	if evidence.SystemEnvelope != prompt || evidence.ModelInput != prompt ||
		evidence.OutputLimitMode != string(prepared.OutputLimitMode) ||
		!bytes.Equal(evidence.ProviderRequest, providerRequest) {
		return exactStationCall{}, llm.PreparedModel{}, fmt.Errorf(
			"persisted portable work %s call differs from its deterministic request",
			job.ID,
		)
	}
	return call, prepared, nil
}
