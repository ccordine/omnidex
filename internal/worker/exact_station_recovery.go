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

type exactStationRecovery struct {
	Result                       assemblyline.PortableResult
	Execution                    exactStationExecution
	Evidence                     queue.LLMCallEvidence
	SemanticParentCallEvidenceID int64
	Accepted                     bool
}

func (s *Service) recoverExactPortableStation(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	job assemblyline.PortableJob,
	modelName string,
) (*exactStationRecovery, error) {
	if ctx == nil || s == nil || s.repo == nil {
		return nil, fmt.Errorf(
			"exact station recovery requires context, worker, and PostgreSQL authority",
		)
	}
	if err := job.Validate(); err != nil {
		return nil, err
	}
	evidence, found, err := s.repo.LatestReusableLLMCallEvidence(
		ctx, authority, job.ID,
	)
	if err != nil || !found {
		return nil, err
	}
	for depth := 1; evidence.Iteration > 1; depth++ {
		if depth >= assemblyline.MaxSourceBodyAttempts {
			return nil, fmt.Errorf(
				"persisted portable work %s correction lineage exceeds its bound",
				job.ID,
			)
		}
		evidence, err = s.repo.GetLLMCallEvidence(
			ctx, evidence.ParentCallEvidenceID,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"read persisted portable work %s parent: %w", job.ID, err,
			)
		}
	}
	if evidence.Iteration != 1 {
		return nil, fmt.Errorf(
			"persisted portable work %s has no initial lineage root", job.ID,
		)
	}
	return s.recoverExactPortableStationEvidence(
		ctx, authority, job, modelName, evidence, nil,
	)
}

func (s *Service) recoverExactPortableStationChild(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	job assemblyline.PortableJob,
	modelName string,
	parentCallID int64,
	correction *assemblyline.SourceBodyCorrection,
) (*exactStationRecovery, error) {
	if ctx == nil || s == nil || s.repo == nil {
		return nil, fmt.Errorf(
			"exact station child recovery requires context, worker, and PostgreSQL authority",
		)
	}
	evidence, found, err := s.repo.ReusableLLMCallChildEvidence(
		ctx, authority, parentCallID,
	)
	if err != nil || !found {
		return nil, err
	}
	if evidence.ParentCallEvidenceID != parentCallID {
		return nil, fmt.Errorf("persisted correction child differs from its parent")
	}
	if evidence.OutputContinuation == 0 {
		continued, continuedFound, continuedErr := s.repo.ReusableLLMCallChildEvidence(
			ctx, authority, evidence.ID,
		)
		if continuedErr != nil {
			return nil, continuedErr
		}
		if continuedFound && continued.Iteration == evidence.Iteration &&
			continued.OutputContinuation == 1 {
			evidence = continued
		}
	}
	recovery, err := s.recoverExactPortableStationEvidence(
		ctx, authority, job, modelName, evidence, correction,
	)
	if recovery != nil {
		recovery.SemanticParentCallEvidenceID = parentCallID
	}
	return recovery, err
}

func (s *Service) recoverExactPortableStationEvidence(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	job assemblyline.PortableJob,
	modelName string,
	evidence queue.LLMCallEvidence,
	correction *assemblyline.SourceBodyCorrection,
) (*exactStationRecovery, error) {
	wantedScope, err := portableModelScope(job.Kind)
	if err != nil {
		return nil, err
	}
	if evidence.JobID != authority.JobID || evidence.Generation != authority.Generation ||
		evidence.StepID != authority.StepID || evidence.WorkID != job.ID ||
		evidence.WorkKind != string(job.Kind) || evidence.Scope != wantedScope ||
		evidence.RequestedModel != modelName || evidence.Model != modelName ||
		evidence.Protocol != string(llm.ExactPreparedProtocolPlainCompletionV4) {
		return nil, fmt.Errorf(
			"persisted portable work %s differs from its current job, kind, scope, immutable model route, or raw provider protocol",
			job.ID,
		)
	}
	if evidence.OutputLimitReached {
		return s.resumePersistedExactStationOutputLimit(
			ctx, authority, job, modelName, evidence, correction,
		)
	}
	if !evidence.ProviderReceiptPresent || evidence.Status != queue.LLMCallSucceeded ||
		evidence.Candidate == "" || evidence.Outcome == nil {
		return nil, fmt.Errorf(
			"persisted portable work %s has no reusable terminal provider result",
			job.ID,
		)
	}
	accepted := false
	switch evidence.Outcome.Status {
	case queue.LLMCallAccepted:
		accepted = true
	case queue.LLMCallRejected:
	default:
		return nil, fmt.Errorf(
			"persisted portable work %s ended as %s and cannot be replayed",
			job.ID, evidence.Outcome.Status,
		)
	}
	projection, err := assemblyline.NewExactPortableResultProjection(evidence.Candidate)
	if err != nil {
		return nil, fmt.Errorf("rehydrate exact portable result: %w", err)
	}
	if evidence.CandidateSHA256 != projection.SourceResponseSHA256 ||
		evidence.Outcome.CandidateSHA256 != projection.SourceResponseSHA256 {
		return nil, fmt.Errorf(
			"persisted portable work %s candidate identity is inconsistent",
			job.ID,
		)
	}
	result := assemblyline.PortableResult{
		JobID: job.ID, Candidate: evidence.Candidate, Projection: &projection,
	}
	if err := result.ValidateFor(job); err != nil {
		return nil, fmt.Errorf("rehydrate portable result: %w", err)
	}
	execution := exactStationExecution{
		CallEvidenceID: evidence.ID, WorkID: job.ID, WorkKind: job.Kind,
		Model: modelName, Iteration: evidence.Iteration,
		OutputContinuation:      evidence.OutputContinuation,
		Candidate:               evidence.Candidate,
		CandidateResponseSHA256: projection.SourceResponseSHA256,
		Replayed:                true, PersistedOutcome: evidence.Outcome.Status,
		PersistedValidationError: evidence.Outcome.ValidationError,
	}
	if job.Kind == assemblyline.WorkFragmentGeneration {
		if evidence.Iteration == 1 {
			sourceState, err := assemblyline.ExtractFragmentGenerationSourceBody(
				job, evidence.Candidate,
			)
			if err != nil {
				return nil, fmt.Errorf(
					"rehydrate portable work %s initial source state: %w", job.ID, err,
				)
			}
			execution.SourceState = sourceState
		} else if evidence.ParentCallEvidenceID < 1 {
			return nil, fmt.Errorf(
				"rehydrate portable work %s correction lineage is incomplete", job.ID,
			)
		}
	}
	return &exactStationRecovery{
		Result: result, Execution: execution, Evidence: evidence,
		SemanticParentCallEvidenceID: evidence.ParentCallEvidenceID,
		Accepted:                     accepted,
	}, nil
}

func (s *Service) resumePersistedExactStationOutputLimit(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	job assemblyline.PortableJob,
	modelName string,
	evidence queue.LLMCallEvidence,
	correction *assemblyline.SourceBodyCorrection,
) (*exactStationRecovery, error) {
	if evidence.OutputContinuation != 0 {
		return nil, fmt.Errorf(
			"persisted portable work %s exhausted its hard native-context output authority",
			job.ID,
		)
	}
	if !evidence.ProviderReceiptPresent || evidence.Status != queue.LLMCallFailed ||
		evidence.Outcome == nil || evidence.Outcome.Status != queue.LLMCallProviderFailed ||
		evidence.Candidate == "" || evidence.PromptTokens < 1 || evidence.OutputTokens < 1 {
		return nil, fmt.Errorf(
			"persisted portable work %s output-limit evidence is incomplete",
			job.ID,
		)
	}
	prompt := ""
	singleLine := false
	var sourceCorrection *assemblyline.SourceBodyCorrectionEvidence
	if correction == nil {
		if evidence.Iteration != 1 {
			return nil, fmt.Errorf(
				"persisted portable work %s requires its recreated source correction",
				job.ID,
			)
		}
		var err error
		prompt, err = assemblyline.RenderPortableJob(job)
		if err != nil {
			return nil, err
		}
		framing, err := assemblyline.PortableResponseFramingForJob(job)
		if err != nil {
			return nil, err
		}
		singleLine = framing == assemblyline.PortableResponseFramingSingleLine
	} else {
		if evidence.Iteration <= 1 {
			return nil, fmt.Errorf(
				"persisted portable work %s initial call cannot carry source correction state",
				job.ID,
			)
		}
		if err := correction.Validate(); err != nil {
			return nil, err
		}
		persisted, err := correction.Evidence()
		if err != nil {
			return nil, err
		}
		if persisted != sourceCorrectionEvidenceFromCall(evidence) {
			return nil, fmt.Errorf(
				"persisted portable work %s output continuation differs from its recreated source correction",
				job.ID,
			)
		}
		prompt, err = correction.ModelInput()
		if err != nil {
			return nil, err
		}
		_, singleLine, err = correction.OpaqueResponseMaximumBytes()
		if err != nil {
			return nil, err
		}
		sourceCorrection = &persisted
	}
	call := exactStationCall{
		WorkID: job.ID, WorkKind: job.Kind, Iteration: evidence.Iteration,
		ParentCallID: evidence.ParentCallEvidenceID, Prompt: prompt,
		ContextTokens: evidence.ContextTokens, MaxOutputTokens: evidence.MaxOutputTokens,
		SingleLine: singleLine, SourceCorrection: sourceCorrection,
	}
	prepared, err := prepareExactStationCall(call, modelName, nil)
	if err != nil {
		return nil, err
	}
	providerRequest, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		return nil, err
	}
	if evidence.SystemEnvelope != prompt || evidence.ModelInput != prompt ||
		evidence.OutputLimitMode != string(prepared.OutputLimitMode) ||
		!bytes.Equal(evidence.ProviderRequest, providerRequest) {
		return nil, fmt.Errorf(
			"persisted portable work %s incomplete call differs from its deterministic request",
			job.ID,
		)
	}
	failure := &llm.ExactPreparedOutputLimitReachedError{
		DoneReason: "length", PromptTokens: evidence.PromptTokens,
		OutputTokens: evidence.OutputTokens, ContextTokens: evidence.ContextTokens,
		MaxOutputTokens: evidence.MaxOutputTokens, ContentBytes: len(evidence.Candidate),
	}
	result, execution, dispatchErr := s.continueExactStationAfterOutputLimit(
		ctx, authority, call, prepared, evidence, failure,
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

func sourceCorrectionEvidenceFromCall(
	evidence queue.LLMCallEvidence,
) assemblyline.SourceBodyCorrectionEvidence {
	return assemblyline.SourceBodyCorrectionEvidence{
		BaseCandidate:  evidence.SourceBaseCandidate,
		BaseSHA256:     evidence.SourceBaseSHA256,
		StartByte:      evidence.SourceStartByte,
		EndByte:        evidence.SourceEndByte,
		Question:       evidence.SourceQuestion,
		QuestionSHA256: evidence.SourceQuestionSHA256,
	}
}
