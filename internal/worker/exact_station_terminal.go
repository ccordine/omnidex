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
	result, callErr := s.stationClient.GeneratePreparedExact(ctx, prepared)
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
	if evidenceErr != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, evidenceErr
	}
	if callErr != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
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
		execution := exactStationExecution{
			CallEvidenceID: evidence.ID, WorkID: call.WorkID, Candidate: result.Content,
		}
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
	execution := exactStationExecution{
		CallEvidenceID: evidence.ID, WorkID: call.WorkID, Candidate: result.Content,
		CandidateResponseSHA256: projection.SourceResponseSHA256,
	}
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
