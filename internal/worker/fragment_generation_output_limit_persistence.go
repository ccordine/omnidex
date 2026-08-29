package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

// persistedFragmentGenerationOutputLimitFailure is code-owned routing
// evidence. It can exist only after the exact initial fragment gap, failed
// outcome, and provider call receipt have committed. It deliberately contains
// no rejected provider source bytes.
type persistedFragmentGenerationOutputLimitFailure struct {
	Evidence            llm.ExactPreparedOutputLimitReachedError
	OriginGapOpeningID  int64
	OriginCallReceiptID int64
}

func (failure *persistedFragmentGenerationOutputLimitFailure) Error() string {
	if failure == nil {
		return "persisted fragment generation output-limit evidence is absent"
	}
	return fmt.Sprintf(
		"persisted fragment generation gap %d ended at the exact provider output limit: %s",
		failure.OriginGapOpeningID, failure.Evidence.Error(),
	)
}

func (failure *persistedFragmentGenerationOutputLimitFailure) Validate() error {
	if failure == nil {
		return fmt.Errorf("persisted fragment generation output-limit evidence is absent")
	}
	if err := failure.Evidence.Validate(); err != nil {
		return err
	}
	if failure.OriginGapOpeningID < 1 || failure.OriginCallReceiptID < 1 {
		return fmt.Errorf(
			"persisted fragment generation output-limit evidence requires exact gap and call receipt identities",
		)
	}
	return nil
}

func newPersistedFragmentGenerationOutputLimitFailure(
	gap queue.StationGapOpening,
	receipt queue.StationCallReceipt,
	evidence *llm.ExactPreparedOutputLimitReachedError,
) (*persistedFragmentGenerationOutputLimitFailure, error) {
	if gap.ID < 1 || gap.WorkKind != string(assemblyline.WorkFragmentGeneration) ||
		gap.GapID == "" || receipt.Status != "failed" ||
		receipt.JobID != gap.JobID || receipt.Generation != gap.Generation ||
		receipt.StepID != gap.StepID || receipt.StepAttempt != gap.StepAttempt ||
		receipt.WorkerID != gap.WorkerID || receipt.GapID != gap.GapID {
		return nil, fmt.Errorf(
			"fragment generation output-limit receipt differs from its exact initial gap",
		)
	}
	if evidence == nil {
		return nil, fmt.Errorf("fragment generation output-limit evidence is absent")
	}
	failure := &persistedFragmentGenerationOutputLimitFailure{
		Evidence:            *evidence,
		OriginGapOpeningID:  gap.ID,
		OriginCallReceiptID: receipt.ID,
	}
	if err := failure.Validate(); err != nil {
		return nil, err
	}
	return failure, nil
}

func (s *Service) persistFragmentGenerationOutputLimitFailure(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	gap queue.StationGapOpening,
	receipt queue.StationCallReceipt,
	evidence *llm.ExactPreparedOutputLimitReachedError,
	cause error,
) error {
	failure, err := newPersistedFragmentGenerationOutputLimitFailure(
		gap, receipt, evidence,
	)
	if err != nil {
		return s.failStationGap(
			ctx, authority, gap,
			fmt.Errorf("%v; bind durable output-limit lineage: %w", cause, err),
		)
	}
	persistCtx, cancel := stationPersistenceContext(ctx)
	_, persistenceErr := s.repo.CloseStationGap(
		persistCtx,
		queue.StationGapTerminalRecord{
			Authority: authority, OpeningID: gap.ID, GapID: gap.GapID,
			Status: queue.StationGapFailed, Error: stationFailureText(cause),
		},
	)
	cancel()
	if persistenceErr != nil {
		return persistedStationGapFailure(cause, persistenceErr)
	}
	return failure
}
