package cognitionstore

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/model"
)

func (store *Store) RecordBrainBootstrapFailure(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	episode cognition.EpisodeRef,
	failure cognitionpolicy.BrainBootstrapFailure,
) error {
	if store == nil || store.repository == nil {
		return fmt.Errorf("cognition Brain bootstrap failure store is uninitialized")
	}
	if err := episode.Validate(); err != nil {
		return err
	}
	return store.repository.RecordCognitionBrainBootstrapFailure(
		ctx, authority, episode.ID, failure,
	)
}

func (store *Store) RecordProviderProcessFailure(
	ctx context.Context,
	bootstrap cognitionpolicy.BrainBootstrap,
	failure cognitionpolicy.ProviderProcessFailure,
) error {
	if store == nil || store.repository == nil {
		return fmt.Errorf("cognition provider process failure store is uninitialized")
	}
	return store.repository.RecordCognitionProviderProcessFailure(ctx, bootstrap, failure)
}
