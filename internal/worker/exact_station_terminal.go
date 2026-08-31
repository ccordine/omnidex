package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const stationPersistenceTimeout = 30 * time.Second

func (s *Service) dispatchExactStationCall(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	gap queue.StationGapOpening,
	requestedModel string,
	prepared llm.PreparedModel,
) (assemblyline.PortableResult, exactStationExecution, error) {
	started := time.Now()
	stopHeartbeat := s.startProgressHeartbeat(ctx, authority, "station-call:"+gap.GapID)
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
	latency := time.Since(started)
	if metricsErr := s.recordWorkerLLMCall(
		ctx, authority, gap.Scope, requestedModel, len(gap.Prompt), 1,
		callErr == nil, callErr, latency,
	); metricsErr != nil && s.logger != nil {
		s.logger.Printf(
			"job=%d step=%d station=%s telemetry error: %v",
			authority.JobID, authority.StepID, gap.Station, metricsErr,
		)
	}
	if callErr != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, s.failStationGap(
			ctx, authority, gap, fmt.Errorf("exact station provider call: %w", callErr),
		)
	}
	if err := ctx.Err(); err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, s.failStationGap(
			ctx, authority, gap, err,
		)
	}
	projection, err := assemblyline.NewExactPortableResultProjection(result.Content)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, s.failStationGap(
			ctx, authority, gap, fmt.Errorf("bind exact station response projection: %w", err),
		)
	}
	portable := assemblyline.PortableResult{
		JobID: gap.WorkID, Candidate: result.Content, Projection: &projection,
	}
	return portable, exactStationExecution{
		Gap: gap, Candidate: result.Content,
		CallReceiptSHA256:       directStationReceiptSHA256(result),
		CandidateResponseSHA256: projection.SourceResponseSHA256,
	}, nil
}

func directStationReceiptSHA256(result llm.PreparedGeneration) string {
	value := strings.Join([]string{
		result.ProviderRequestSHA256,
		result.ProviderResponseSHA256,
		result.Content,
	}, "\n")
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func (s *Service) failStationGap(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	gap queue.StationGapOpening,
	cause error,
) error {
	persistCtx, cancel := stationPersistenceContext(ctx)
	defer cancel()
	_, closeErr := s.repo.CloseStationGap(persistCtx, queue.StationGapTerminalRecord{
		Authority: authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: queue.StationGapFailed, Error: stationFailureText(cause),
	})
	return persistedStationGapFailure(cause, closeErr)
}

func persistedStationGapFailure(cause, persistenceErr error) error {
	if cause == nil {
		return fmt.Errorf("station gap failure requires one exact cause")
	}
	if persistenceErr == nil {
		return cause
	}
	return fmt.Errorf(
		"station gap failed with %v; persist terminal station gap outcome: %w",
		cause, persistenceErr,
	)
}

func stationPersistenceContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	return context.WithTimeout(base, stationPersistenceTimeout)
}

func stationFailureText(err error) string {
	if err == nil {
		return ""
	}
	value := queue.SanitizeUTF8Text(strings.TrimSpace(err.Error()))
	if value == "" {
		value = "station boundary failed without a diagnostic"
	}
	return queue.TruncateUTF8Text(value, 7900, "...[truncated]")
}
