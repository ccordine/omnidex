package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const exactStationEvidencePersistenceTimeout = 30 * time.Second

type exactStationEvidencePersistenceError struct {
	persistenceErr error
	callErr        error
}

func (failure *exactStationEvidencePersistenceError) Error() string {
	if failure.callErr == nil {
		return fmt.Sprintf("persist exact station evidence: %v", failure.persistenceErr)
	}
	return fmt.Sprintf(
		"persist exact station evidence after provider failure %q: %v",
		failure.callErr, failure.persistenceErr,
	)
}

func (failure *exactStationEvidencePersistenceError) Unwrap() error {
	return failure.persistenceErr
}

func (s *Service) reserveExactStationCallEvidence(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	call exactStationCall,
	prepared llm.PreparedModel,
) (queue.LLMCallEvidence, error) {
	scope, err := portableModelScope(call.WorkKind)
	if err != nil {
		return queue.LLMCallEvidence{}, err
	}
	persistCtx, cancel := exactStationEvidenceContext(ctx)
	defer cancel()
	evidence, persistErr := s.repo.ReserveLLMCallEvidence(
		persistCtx,
		queue.LLMCallOpeningRecord{
			Authority: authority, Scope: scope, WorkID: call.WorkID,
			WorkKind: call.WorkKind, Iteration: call.Iteration,
			OutputContinuation:   call.OutputContinuation,
			ParentCallEvidenceID: call.ParentCallID,
			SourceCorrection:     call.SourceCorrection,
			RequestedModel:       prepared.BaseModel,
			Prepared:             prepared,
		},
	)
	if persistErr != nil {
		return queue.LLMCallEvidence{}, &exactStationEvidencePersistenceError{
			persistenceErr: persistErr,
		}
	}
	return evidence, nil
}

func (s *Service) finalizeExactStationCallEvidence(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	callEvidenceID int64,
	prepared llm.PreparedModel,
	result llm.PreparedGeneration,
	callErr error,
	elapsed time.Duration,
) (queue.LLMCallEvidence, error) {
	persistCtx, cancel := exactStationEvidenceContext(ctx)
	defer cancel()
	record := queue.LLMCallReceiptRecord{
		Authority: authority, CallEvidenceID: callEvidenceID,
		Prepared: prepared, Generation: result, Elapsed: elapsed,
	}
	if callErr != nil {
		record.CallError = exactStationEvidenceError(callErr)
		var outputLimit *llm.ExactPreparedOutputLimitReachedError
		record.OutputLimitReached = errors.As(callErr, &outputLimit)
	}
	evidence, persistErr := s.repo.FinalizeLLMCallEvidence(persistCtx, record)
	if persistErr != nil {
		return queue.LLMCallEvidence{}, &exactStationEvidencePersistenceError{
			persistenceErr: persistErr, callErr: callErr,
		}
	}
	return evidence, nil
}

func (s *Service) persistExactStationSemanticOutcome(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	execution exactStationExecution,
	result assemblyline.PortableResult,
	validationErr error,
) error {
	projection := result.Projection
	if projection != nil && projection.ValidateFor(execution.Candidate) != nil {
		projection = nil
	}
	record := queue.LLMCallOutcomeRecord{
		Authority: authority, CallEvidenceID: execution.CallEvidenceID,
		Candidate: execution.Candidate, Projection: projection,
	}
	if validationErr != nil {
		record.ValidationError = exactStationEvidenceError(validationErr)
	}
	persistCtx, cancel := exactStationEvidenceContext(ctx)
	defer cancel()
	if _, err := s.repo.RecordLLMCallOutcome(persistCtx, record); err != nil {
		if errors.Is(err, queue.ErrLLMCallTerminalizedByAttempt) {
			return err
		}
		return &exactStationEvidencePersistenceError{
			persistenceErr: err, callErr: validationErr,
		}
	}
	return nil
}

func exactStationEvidenceContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	return context.WithTimeout(base, exactStationEvidencePersistenceTimeout)
}

func exactStationEvidenceError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.ToValidUTF8(strings.TrimSpace(err.Error()), "\uFFFD")
	value = strings.ReplaceAll(value, "\x00", "\uFFFD")
	if value == "" {
		value = "station boundary failed without a diagnostic"
	}
	const limit = 8192
	if len(value) <= limit {
		return value
	}
	value = value[:limit-len("...[truncated]")]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value + "...[truncated]"
}
