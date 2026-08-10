package cognitionstore

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/queue"
)

func (store *Store) RecordProviderProcessObservation(
	ctx context.Context,
	receipt cognitionpolicy.ProviderProcessObservation,
) error {
	if store == nil || store.repository == nil {
		return fmt.Errorf("cognition provider process observation store is uninitialized")
	}
	return store.repository.RecordCognitionProviderProcessObservation(ctx, receipt)
}

func (store *Store) ReadProviderProcessObservationPage(
	ctx context.Context,
	episode cognition.EpisodeRef,
	request queue.CognitionProviderProcessObservationPageRequest,
) (queue.CognitionProviderProcessObservationPage, error) {
	if store == nil || store.repository == nil {
		return queue.CognitionProviderProcessObservationPage{},
			fmt.Errorf("cognition provider process observation store is uninitialized")
	}
	if err := episode.Validate(); err != nil {
		return queue.CognitionProviderProcessObservationPage{}, err
	}
	return store.repository.ReadCognitionProviderProcessObservationPage(ctx, episode.ID, request)
}
