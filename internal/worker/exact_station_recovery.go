package worker

import (
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
	for links := 0; evidence.Iteration > 1; links++ {
		if links >= 2*assemblyline.MaxSourceBodyAttempts {
			return nil, fmt.Errorf(
				"persisted portable work %s correction lineage exceeds its bound",
				job.ID,
			)
		}
		child := evidence
		parent, parentErr := s.repo.GetLLMCallEvidence(
			ctx, child.ParentCallEvidenceID,
		)
		if parentErr != nil {
			return nil, fmt.Errorf(
				"read persisted portable work %s parent: %w", job.ID, parentErr,
			)
		}
		if child.OutputContinuation == 1 {
			if parent.Iteration != child.Iteration || parent.OutputContinuation != 0 {
				return nil, fmt.Errorf(
					"persisted portable work %s output continuation lineage is invalid",
					job.ID,
				)
			}
		} else if child.OutputContinuation != 0 ||
			parent.Iteration != child.Iteration-1 {
			return nil, fmt.Errorf(
				"persisted portable work %s semantic correction lineage is invalid",
				job.ID,
			)
		}
		evidence = parent
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
	if !evidence.ProviderReceiptPresent && evidence.Outcome != nil &&
		evidence.Outcome.Status == queue.LLMCallInterrupted {
		return s.resumeInterruptedExactStationCall(
			ctx, authority, job, modelName, evidence, correction,
		)
	}
	if evidence.OutputLimitReached {
		execution := exactStationExecution{
			CallEvidenceID: evidence.ID, WorkID: job.ID, WorkKind: job.Kind,
			Model: modelName, Iteration: evidence.Iteration,
			OutputContinuation:       evidence.OutputContinuation,
			DispatchAttempt:          evidence.DispatchAttempt,
			Candidate:                evidence.Candidate,
			Replayed:                 true,
			PersistedOutcome:         queue.LLMCallProviderFailed,
			PersistedValidationError: evidence.Error,
		}
		return &exactStationRecovery{
			Execution: execution, Evidence: evidence,
			SemanticParentCallEvidenceID: evidence.ParentCallEvidenceID,
		}, fmt.Errorf("exact station provider call: %s", evidence.Error)
	}
	if !evidence.ProviderReceiptPresent || evidence.Status != queue.LLMCallSucceeded ||
		evidence.Candidate == "" {
		return nil, fmt.Errorf(
			"persisted portable work %s has no reusable successful provider result",
			job.ID,
		)
	}
	accepted := false
	if evidence.Outcome != nil {
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
	}
	projection, err := assemblyline.NewExactPortableResultProjection(evidence.Candidate)
	if err != nil {
		return nil, fmt.Errorf("rehydrate exact portable result: %w", err)
	}
	if evidence.CandidateSHA256 != projection.SourceResponseSHA256 ||
		(evidence.Outcome != nil &&
			evidence.Outcome.CandidateSHA256 != projection.SourceResponseSHA256) {
		return nil, fmt.Errorf(
			"persisted portable work %s candidate identity is inconsistent",
			job.ID,
		)
	}
	result := assemblyline.PortableResult{
		JobID: job.ID, Candidate: evidence.Candidate, Projection: &projection,
	}
	if accepted {
		if err := result.ValidateFor(job); err != nil {
			return nil, fmt.Errorf("rehydrate accepted portable result: %w", err)
		}
	}
	execution := exactStationExecution{
		CallEvidenceID: evidence.ID, WorkID: job.ID, WorkKind: job.Kind,
		Model: modelName, Iteration: evidence.Iteration,
		OutputContinuation:      evidence.OutputContinuation,
		DispatchAttempt:         evidence.DispatchAttempt,
		Candidate:               evidence.Candidate,
		CandidateResponseSHA256: projection.SourceResponseSHA256,
	}
	if evidence.Outcome != nil {
		execution.Replayed = true
		execution.PersistedOutcome = evidence.Outcome.Status
		execution.PersistedValidationError = evidence.Outcome.ValidationError
	}
	if job.Kind == assemblyline.WorkFragmentGeneration {
		if evidence.Iteration == 1 {
			sourceState, err := assemblyline.ExtractFragmentGenerationSourceBody(
				job, evidence.Candidate,
			)
			if err != nil && accepted {
				return nil, fmt.Errorf(
					"rehydrate accepted portable work %s initial source state: %w", job.ID, err,
				)
			}
			if err == nil {
				execution.SourceState = sourceState
			}
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
