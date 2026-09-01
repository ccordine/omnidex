package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type exactStationCall struct {
	WorkID           string
	WorkKind         assemblyline.WorkKind
	Iteration        int
	ParentCallID     int64
	Prompt           string
	ContextTokens    int
	MaxOutputTokens  int
	SingleLine       bool
	SourceCorrection *assemblyline.SourceBodyCorrectionEvidence
}

type exactStationExecution struct {
	CallEvidenceID           int64
	WorkID                   string
	WorkKind                 assemblyline.WorkKind
	Model                    string
	Iteration                int
	ProviderCalls            int
	Candidate                string
	CandidateResponseSHA256  string
	SourceState              string
	Replayed                 bool
	PersistedOutcome         queue.LLMCallOutcomeStatus
	PersistedValidationError string
}

func (s *Service) executeExactPortableStation(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	job assemblyline.PortableJob,
	modelName string,
) (assemblyline.PortableResult, exactStationExecution, error) {
	if ctx == nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf("exact station requires context, worker, and PostgreSQL authority")
	}
	if err := ctx.Err(); err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	deterministic, resolved, err := assemblyline.ResolvePortableJobWithoutInference(job)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	if resolved {
		return deterministic, exactStationExecution{
			WorkID: job.ID, WorkKind: job.Kind, Candidate: deterministic.Candidate,
		}, nil
	}
	if s == nil || s.repo == nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf("exact station requires context, worker, and PostgreSQL authority")
	}
	if modelName == "" {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf("exact station model is required")
	}
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	contextTokens, err := s.exactStationContextTokens(ctx)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"resolve exact station context: %w", err,
		)
	}
	maxOutputTokens, err := queue.ExpectedPortableStationMaxOutputTokens(job, contextTokens)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"derive exact station output ceiling: %w", err,
		)
	}
	call := exactStationCall{
		WorkID: job.ID, WorkKind: job.Kind, Prompt: prompt,
		Iteration:     1,
		ContextTokens: contextTokens, MaxOutputTokens: maxOutputTokens,
	}
	if nilWorkerTransport(s.stationClient) {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"exact station generation provider is not configured",
		)
	}
	prepared, err := prepareExactStationCall(call, modelName, nil)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	return s.dispatchExactStationCall(ctx, authority, call, prepared)
}

func (s *Service) executeExactPortableStationCorrection(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	job assemblyline.PortableJob,
	modelName string,
	previous exactStationExecution,
	correction assemblyline.SourceBodyCorrection,
) (assemblyline.PortableResult, exactStationExecution, error) {
	if ctx == nil || s == nil || s.repo == nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"exact station correction requires context, worker, and PostgreSQL authority",
		)
	}
	if err := ctx.Err(); err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	if nilWorkerTransport(s.stationClient) {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"exact station generation provider is not configured",
		)
	}
	if previous.CallEvidenceID < 1 || previous.WorkID != job.ID ||
		previous.WorkKind != job.Kind || previous.Model != modelName || previous.Iteration < 1 ||
		previous.Iteration >= assemblyline.MaxSourceBodyAttempts {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"exact station correction differs from its persisted job or model context",
		)
	}
	if err := correction.Validate(); err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	correctionEvidence, err := correction.Evidence()
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	if previous.SourceState == "" || correctionEvidence.BaseCandidate != previous.SourceState {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"exact station correction differs from its code-owned current source state",
		)
	}
	persisted, found, err := s.repo.LatestReusableLLMCallEvidence(
		ctx, authority, job.ID,
	)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	if !found || persisted.ID != previous.CallEvidenceID {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"exact station correction parent is not the latest reusable persisted response",
		)
	}
	if persisted.JobID != authority.JobID || persisted.Generation != authority.Generation ||
		persisted.StepID != authority.StepID || persisted.WorkID != job.ID ||
		persisted.WorkKind != string(job.Kind) || persisted.Iteration != previous.Iteration ||
		persisted.Model != modelName || persisted.RequestedModel != modelName ||
		persisted.Protocol != string(llm.ExactPreparedProtocolPlainCompletionV4) ||
		persisted.Candidate != previous.Candidate ||
		persisted.Status != queue.LLMCallSucceeded || persisted.Outcome == nil ||
		persisted.Outcome.Status != queue.LLMCallRejected {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"exact station correction is not bound to one rejected persisted job/model response",
		)
	}
	prompt, err := correction.ModelInput()
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	contextTokens := persisted.ContextTokens
	if err := llm.ValidateExactPreparedContextTokens(contextTokens); err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"persisted exact station model context is invalid: %w", err,
		)
	}
	opaqueResponseBytes, opaqueCorrection, err := correction.OpaqueResponseMaximumBytes()
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	maxOutputTokens, err := queue.ExpectedSourceBodyCorrectionMaxOutputTokens(
		opaqueResponseBytes, opaqueCorrection, contextTokens,
	)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	call := exactStationCall{
		WorkID: job.ID, WorkKind: job.Kind, Iteration: previous.Iteration + 1,
		ParentCallID: previous.CallEvidenceID, Prompt: prompt,
		ContextTokens: contextTokens, MaxOutputTokens: maxOutputTokens,
		SingleLine:       opaqueCorrection,
		SourceCorrection: &correctionEvidence,
	}
	prepared, err := prepareExactStationCall(call, modelName, nil)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	return s.dispatchExactStationCall(ctx, authority, call, prepared)
}
