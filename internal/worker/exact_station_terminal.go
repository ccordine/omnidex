package worker

import (
	"context"
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

func (s *Service) recordAuthorityEndedExactStationCall(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	gap queue.StationGapOpening,
	call queue.StationCallOpening,
	requestedModel string,
	prepared llm.PreparedModel,
	observed llm.ObservedProviderIdentity,
	attemptStatus model.StepAttemptStatus,
) (assemblyline.PortableResult, exactStationExecution, error) {
	reason, err := providerRequestFailureForAttempt(attemptStatus)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, err
	}
	cause := fmt.Errorf("exact station authority became %s before provider dispatch", attemptStatus)
	result := llm.PreparedGeneration{
		Schema: llm.PreparedGenerationSchemaV1, Protocol: prepared.Protocol,
		ProviderRequestDisposition:   llm.ProviderRequestNotDispatched,
		ProviderRequestFailureReason: reason,
		ProviderRequestSHA256:        call.WireRequestSHA256,
		ProviderIdentityEvidence:     observed.Evidence,
	}
	return s.persistExactStationCallResult(
		ctx, authority, gap, call, requestedModel, result, cause, 0,
	)
}

func providerRequestFailureForAttempt(
	status model.StepAttemptStatus,
) (llm.ProviderRequestFailureReason, error) {
	switch status {
	case model.StepAttemptCanceled:
		return llm.ProviderRequestFailureAuthorityCanceled, nil
	case model.StepAttemptSuperseded:
		return llm.ProviderRequestFailureAuthoritySuperseded, nil
	case model.StepAttemptExpired:
		return llm.ProviderRequestFailureAuthorityExpired, nil
	default:
		return "", fmt.Errorf("step attempt status %q is not terminal provider authority", status)
	}
}

func (s *Service) dispatchExactStationCall(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	gap queue.StationGapOpening,
	call queue.StationCallOpening,
	requestedModel string,
	prepared llm.PreparedModel,
) (assemblyline.PortableResult, exactStationExecution, error) {
	started := time.Now()
	stopHeartbeat := s.startProgressHeartbeat(ctx, authority, "station-call:"+gap.GapID)
	result, callErr := s.stationClient.GeneratePreparedExact(ctx, prepared)
	stopHeartbeat()
	owned, ownershipErr := llm.OwnBoundedPreparedGeneration(result)
	if ownershipErr != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"own bounded exact station result: %w", ownershipErr,
		)
	}
	return s.persistExactStationCallResult(
		ctx, authority, gap, call, requestedModel, owned, callErr,
		time.Since(started).Milliseconds(),
	)
}

func (s *Service) persistExactStationCallResult(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	gap queue.StationGapOpening,
	call queue.StationCallOpening,
	requestedModel string,
	result llm.PreparedGeneration,
	callErr error,
	latencyMS int64,
) (assemblyline.PortableResult, exactStationExecution, error) {
	persistCtx, cancel := stationPersistenceContext(ctx)
	_, receiptErr := s.repo.RecordStationCallReceiptAndEvidence(
		persistCtx,
		queue.StationCallReceiptEvidenceRecord{
			Receipt: queue.StationCallReceiptRecord{
				Authority: authority, OpeningID: call.ID, GapID: gap.GapID,
				Result: result, Error: stationFailureText(callErr),
			},
			RequestedModel: requestedModel, EvidenceAttempt: 1, LatencyMS: latencyMS,
		},
	)
	cancel()
	if receiptErr != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"persist exact station call receipt and evidence: %w", receiptErr,
		)
	}
	if callErr != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, s.failStationGap(
			ctx, authority, gap, fmt.Errorf("exact station provider call: %w", callErr),
		)
	}
	if err := ctx.Err(); err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, s.failStationGap(ctx, authority, gap, err)
	}
	latency := time.Duration(latencyMS) * time.Millisecond
	if err := s.recordWorkerLLMCall(ctx, authority, gap.Scope, requestedModel, len(gap.Prompt), 1, true, nil, latency); err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, s.failStationGap(
			ctx, authority, gap, fmt.Errorf("record exact station metrics: %w", err),
		)
	}
	identity, err := llm.DeriveExactProviderIdentityExpectation(
		result.ProviderIdentityEvidence,
		llm.ProviderIdentitySelection{Model: requestedModel, NativeContextLimit: s.inferenceContextTokens},
	)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, s.failStationGap(
			ctx, authority, gap, fmt.Errorf("derive completed station provider identity: %w", err),
		)
	}
	portable := assemblyline.PortableResult{JobID: gap.WorkID, Candidate: result.Content}
	return portable, exactStationExecution{
		Gap: gap, Candidate: result.Content, ProviderIdentity: identity,
	}, nil
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
	return errors.Join(cause, closeErr)
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
