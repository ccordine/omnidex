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
	evidence, found, err := s.repo.ReusableLLMCallRootEvidence(ctx, authority, job)
	if err != nil || !found {
		return nil, err
	}
	return s.recoverExactPortableStationEvidence(job, modelName, authority, evidence, evidence)
}

func (s *Service) recoverExactPortableStationChild(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	job assemblyline.PortableJob,
	modelName string,
	parentCallID int64,
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
	root, err := s.exactStationLineageRoot(ctx, evidence)
	if err != nil {
		return nil, err
	}
	recovery, err := s.recoverExactPortableStationEvidence(job, modelName, authority, evidence, root)
	if recovery != nil {
		recovery.SemanticParentCallEvidenceID = parentCallID
	}
	return recovery, err
}

func (s *Service) recoverExactPortableStationEvidence(
	job assemblyline.PortableJob,
	modelName string,
	authority model.StepAttemptAuthority,
	evidence queue.LLMCallEvidence,
	root queue.LLMCallEvidence,
) (*exactStationRecovery, error) {
	if root.ID < 1 || root.Iteration != 1 || !bytes.Equal(root.WorkInput, job.Payload) ||
		root.WorkKind != string(job.Kind) {
		return nil, fmt.Errorf("persisted station input differs from the current semantic work")
	}
	wantedScope, err := portableModelScope(job.Kind)
	if err != nil {
		return nil, err
	}
	if evidence.OutputContinuation != 0 || evidence.DispatchAttempt != 1 ||
		evidence.ReplacesCallEvidenceID != 0 {
		return nil, fmt.Errorf(
			"persisted portable work %s contains unsupported continuation or replacement dispatch state",
			job.Kind,
		)
	}
	if evidence.JobID != authority.JobID || evidence.Generation != authority.Generation ||
		evidence.StepID != authority.StepID ||
		evidence.WorkKind != string(job.Kind) || evidence.Scope != wantedScope ||
		evidence.RequestedModel != modelName || evidence.Model != modelName ||
		evidence.Protocol != string(llm.ExactPreparedProtocolPlainCompletionV4) {
		return nil, fmt.Errorf(
			"persisted portable work %s differs from its current job, kind, scope, immutable model route, or raw provider protocol",
			job.Kind,
		)
	}
	if !evidence.ProviderReceiptPresent && evidence.Outcome != nil &&
		evidence.Outcome.Status == queue.LLMCallInterrupted {
		return nil, fmt.Errorf(
			"persisted portable work %s was interrupted before one complete provider response and is terminal",
			job.Kind,
		)
	}
	if evidence.OutputLimitReached {
		execution := exactStationExecution{
			CallEvidenceID: evidence.ID, RootCallEvidenceID: root.ID,
			WorkInput: string(job.Payload), WorkKind: job.Kind,
			Model: modelName, Iteration: evidence.Iteration,
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
			job.Kind,
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
				job.Kind, evidence.Outcome.Status,
			)
		}
	}
	result := assemblyline.PortableResult{
		Candidate: evidence.Candidate,
	}
	if accepted {
		if err := result.ValidateFor(job); err != nil {
			return nil, fmt.Errorf("rehydrate accepted portable result: %w", err)
		}
	}
	execution := exactStationExecution{
		CallEvidenceID: evidence.ID, RootCallEvidenceID: root.ID,
		WorkInput: string(job.Payload), WorkKind: job.Kind,
		Model: modelName, Iteration: evidence.Iteration,
		Candidate: evidence.Candidate,
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
					"rehydrate accepted portable work %s initial source state: %w", job.Kind, err,
				)
			}
			if err == nil {
				execution.SourceState = sourceState
			}
		} else if evidence.ParentCallEvidenceID < 1 {
			return nil, fmt.Errorf(
				"rehydrate portable work %s correction lineage is incomplete", job.Kind,
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
		BaseCandidate: evidence.SourceBaseCandidate,
		StartByte:     evidence.SourceStartByte,
		EndByte:       evidence.SourceEndByte,
		Question:      evidence.SourceQuestion,
	}
}
