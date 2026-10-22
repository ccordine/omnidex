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
	var preparationErr error
	prepared, preparationErr = llm.PrepareNativeRequest(s.stationClient, prepared)
	if preparationErr != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, preparationErr
	}
	if call.Iteration == 1 && call.RootCallEvidenceID != 0 ||
		call.Iteration > 1 && call.RootCallEvidenceID < 1 {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf("station call has invalid initial-call lineage")
	}
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
	stopHeartbeat := s.startProgressHeartbeat(ctx, authority, fmt.Sprintf("station-call:%d", opening.ID))
	result, callErr := generatePreparedExactWithActivity(
		ctx, s.stationClient, prepared, s.exactStationPublicActivity(ctx, authority),
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
		CallEvidenceID: opening.ID, RootCallEvidenceID: call.RootCallEvidenceID,
		WorkInput: call.WorkInput, WorkKind: call.WorkKind,
		Model: prepared.BaseModel, Iteration: call.Iteration,
		ProviderCalls: 1,
	}
	if call.Iteration == 1 {
		execution.RootCallEvidenceID = opening.ID
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
	portable := assemblyline.PortableResult{
		Candidate: result.Content,
	}
	execution.CallEvidenceID = evidence.ID
	execution.Candidate = result.Content
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

func generatePreparedExact(
	ctx context.Context,
	client llm.ExactStationClient,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	return generatePreparedExactWithActivity(ctx, client, prepared, nil)
}

func generatePreparedExactWithActivity(
	ctx context.Context,
	client llm.ExactStationClient,
	prepared llm.PreparedModel,
	observe func(model.AgentActivity) error,
) (llm.PreparedGeneration, error) {
	callCtx, cancelCall := context.WithCancel(ctx)
	defer cancelCall()
	if observable, ok := client.(llm.ExactStationActivityClient); ok && observe != nil {
		return observable.GeneratePreparedExactWithActivity(callCtx, prepared, observe)
	}
	return client.GeneratePreparedExact(callCtx, prepared)
}
