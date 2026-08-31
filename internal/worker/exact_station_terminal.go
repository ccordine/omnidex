package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func (s *Service) dispatchExactStationCall(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	call exactStationCall,
	prepared llm.PreparedModel,
) (assemblyline.PortableResult, exactStationExecution, error) {
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
	if callErr != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"exact station provider call: %w", callErr,
		)
	}
	if err := ctx.Err(); err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	projection, err := assemblyline.NewExactPortableResultProjection(result.Content)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"bind exact station response projection: %w", err,
		)
	}
	portable := assemblyline.PortableResult{
		JobID: call.WorkID, Candidate: result.Content, Projection: &projection,
	}
	return portable, exactStationExecution{
		WorkID: call.WorkID, Candidate: result.Content,
		CandidateResponseSHA256: projection.SourceResponseSHA256,
	}, nil
}
