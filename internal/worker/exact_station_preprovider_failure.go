package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func (s *Service) rejectStationDiscoveryBeforeProvider(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	gap queue.StationGapOpening,
	discovery queue.StationDiscoveryOpening,
	selection llm.ProviderIdentitySelection,
	cause error,
) (assemblyline.PortableResult, exactStationExecution, error) {
	if cause == nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, fmt.Errorf(
			"pre-provider station rejection requires an exact failure",
		)
	}
	evidence, err := llm.NewUndispatchedProviderIdentityEvidence(selection)
	if err != nil {
		return assemblyline.PortableResult{}, exactStationExecution{}, errors.Join(cause, err)
	}
	persistCtx, cancel := stationPersistenceContext(ctx)
	_, persistErr := s.repo.RecordStationDiscoveryFailure(
		persistCtx,
		queue.StationDiscoveryFailureRecord{
			Authority: authority, Gap: gap, Discovery: discovery,
			Observed:      llm.ObservedProviderIdentity{Evidence: evidence},
			FailureReason: queue.StationDiscoveryFailureEvidenceRejected,
			Error:         stationFailureText(cause),
		},
	)
	cancel()
	return assemblyline.PortableResult{}, exactStationExecution{}, errors.Join(cause, persistErr)
}
