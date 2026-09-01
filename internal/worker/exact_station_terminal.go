package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func (s *Service) dispatchExactStationCall(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	call exactStationCall,
	prepared llm.PreparedModel,
) (assemblyline.PortableResult, exactStationExecution, error) {
	if _, err := assemblyline.SemanticUncertaintyContractForWorkKind(call.WorkKind); err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"admit exact station semantic uncertainty before dispatch: %w", err,
		)
	}
	if _, err := llm.ExactPreparedRequestBytes(prepared); err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"validate exact station request before dispatch: %w", err,
		)
	}
	opening, err := s.reserveExactStationCallEvidence(ctx, authority, call, prepared)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	started := time.Now()
	stopHeartbeat := s.startProgressHeartbeat(ctx, authority, "station-call:"+call.WorkID)
	result, callErr := generatePreparedExactWithinMaximumDuration(
		ctx, s.stationClient, prepared,
	)
	stopHeartbeat()
	owned, ownershipErr := llm.OwnBoundedPreparedGeneration(result)
	if ownershipErr != nil {
		callErr = errors.Join(callErr, ownershipErr)
	} else {
		result = owned
		validationErr := llm.ValidateExactPreparedGenerationForRequest(prepared, result)
		if callErr == nil {
			callErr = validationErr
		} else if validationErr != nil {
			callErr = errors.Join(callErr, validationErr)
		}
	}
	evidence, evidenceErr := s.finalizeExactStationCallEvidence(
		ctx, authority, opening.ID, prepared, result, callErr, time.Since(started),
	)
	execution := exactStationExecution{
		CallEvidenceID: opening.ID, WorkID: call.WorkID, WorkKind: call.WorkKind,
		Model: prepared.BaseModel, Iteration: call.Iteration,
		OutputContinuation: call.OutputContinuation,
		DispatchAttempt:    call.DispatchAttempt, ProviderCalls: 1,
	}
	if evidenceErr != nil {
		return assemblyline.PortableResult{}, execution, evidenceErr
	}
	if callErr != nil {
		return assemblyline.PortableResult{}, execution, fmt.Errorf(
			"exact station provider call: %w", callErr,
		)
	}
	if evidence.Outcome != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"%w: exact station response was rejected before semantic consumption: %s",
			queue.ErrLLMCallTerminalizedByAttempt,
			evidence.Outcome.ValidationError,
		)
	}
	projection, err := assemblyline.NewExactPortableResultProjection(result.Content)
	if err != nil {
		execution.Candidate = result.Content
		if persistErr := s.persistExactStationSemanticOutcome(
			ctx, authority, execution,
			assemblyline.PortableResult{JobID: call.WorkID, Candidate: result.Content}, err,
		); persistErr != nil {
			return assemblyline.PortableResult{}, exactStationExecution{}, persistErr
		}
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"bind exact station response projection: %w", err,
		)
	}
	portable := assemblyline.PortableResult{
		JobID: call.WorkID, Candidate: result.Content, Projection: &projection,
	}
	execution.CallEvidenceID = evidence.ID
	execution.Candidate = result.Content
	execution.CandidateResponseSHA256 = projection.SourceResponseSHA256
	if err := ctx.Err(); err != nil {
		if persistErr := s.persistExactStationSemanticOutcome(
			ctx, authority, execution, portable, err,
		); persistErr != nil {
			return assemblyline.PortableResult{}, exactStationExecution{}, persistErr
		}
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	return portable, execution, nil
}

func generatePreparedExactWithinMaximumDuration(
	ctx context.Context,
	client llm.ExactStationClient,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	callCtx, cancelCall := context.WithTimeout(ctx, llm.MaximumModelRequestDuration)
	defer cancelCall()
	return client.GeneratePreparedExact(callCtx, prepared)
}
