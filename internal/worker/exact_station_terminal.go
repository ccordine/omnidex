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
	validationErr := llm.ValidateExactPreparedGenerationForRequest(prepared, owned)
	var validatedLimit *llm.ExactPreparedOutputLimitReachedError
	if errors.As(validationErr, &validatedLimit) {
		callErr = errors.Join(validationErr, callErr)
	} else if callErr == nil {
		callErr = validationErr
	} else {
		// A provider client cannot create output-limit routing authority merely
		// by returning an error of the registered type. Only independently
		// validated owned response evidence may preserve that classification.
		var unvalidatedLimit *llm.ExactPreparedOutputLimitReachedError
		if errors.As(callErr, &unvalidatedLimit) {
			callErr = fmt.Errorf(
				"provider claimed output-limit completion without validated response evidence: %v",
				callErr,
			)
		}
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
	receiptEvidence, receiptErr := s.repo.RecordStationCallReceiptAndEvidence(
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
		var outputLimit *llm.ExactPreparedOutputLimitReachedError
		if gap.WorkKind == string(assemblyline.WorkFragmentGeneration) &&
			errors.As(callErr, &outputLimit) {
			return assemblyline.PortableResult{}, exactStationExecution{},
				s.persistFragmentGenerationOutputLimitFailure(
					ctx, authority, gap, receiptEvidence.Receipt,
					outputLimit, fmt.Errorf("exact station provider call: %w", callErr),
				)
		}
		return assemblyline.PortableResult{}, exactStationExecution{}, s.failStationGap(
			ctx, authority, gap, fmt.Errorf("exact station provider call: %w", callErr),
		)
	}
	if err := queue.ValidateStationCallNativeUsage(call, result); err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, s.failStationGap(
			ctx, authority, gap, fmt.Errorf("validate exact provider native context usage: %w", err),
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
	selection, err := llm.ProviderIdentitySelectionForProfile(
		requestedModel, call.ContextTokens, call.TokenizerProfile,
	)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, s.failStationGap(
			ctx, authority, gap, fmt.Errorf("reconstruct completed station provider policy: %w", err),
		)
	}
	identity, err := llm.DeriveExactProviderIdentityExpectation(
		result.ProviderIdentityEvidence, selection,
	)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, s.failStationGap(
			ctx, authority, gap, fmt.Errorf("derive completed station provider identity: %w", err),
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
		CallReceiptSHA256:       receiptEvidence.Receipt.GenerationSHA256,
		CandidateResponseSHA256: receiptEvidence.Evidence.ResponseSHA256,
		ProviderIdentity:        identity,
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
	return persistedStationGapFailure(cause, closeErr)
}

// persistedStationGapFailure preserves typed failure routing authority only
// after the terminal gap outcome was durably recorded. A persistence failure
// still reports the original cause as text, but it must not unwrap to that
// cause: downstream code cannot open replacement work from unmatched state.
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
