package cognitiongauntlet

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionstore"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func attestPersistedRuntimeBrain(
	ctx context.Context,
	store *cognitionstore.Store,
	client llm.Client,
	brain cognitionpolicy.BrainRef,
	authority model.StepAttemptAuthority,
	episode cognition.EpisodeRef,
) (cognitionpolicy.BrainBootstrap, error) {
	outcome, observationErr := cognitionpolicy.AttestBrain(ctx, client, brain)
	if observationErr == nil {
		bootstrap, err := outcome.RequireSuccess()
		if err != nil {
			return cognitionpolicy.BrainBootstrap{}, fmt.Errorf(
				"require successful cognition Brain bootstrap: %w", err,
			)
		}
		return bootstrap, nil
	}
	if outcome.Failure == nil {
		return cognitionpolicy.BrainBootstrap{}, errors.Join(
			observationErr,
			fmt.Errorf("cognition Brain bootstrap failure lacks recordable raw evidence"),
		)
	}
	if err := outcome.Validate(); err != nil {
		return cognitionpolicy.BrainBootstrap{}, errors.Join(
			observationErr, fmt.Errorf("validate cognition Brain bootstrap failure: %w", err),
		)
	}
	if err := store.RecordBrainBootstrapFailure(
		ctx, authority, episode, *outcome.Failure,
	); err != nil {
		return cognitionpolicy.BrainBootstrap{}, errors.Join(
			observationErr, fmt.Errorf("persist cognition Brain bootstrap failure: %w", err),
		)
	}
	return cognitionpolicy.BrainBootstrap{}, fmt.Errorf(
		"attest cognition Brain: %w", observationErr,
	)
}

func observePersistedRuntimeProviderProcess(
	ctx context.Context,
	store *cognitionstore.Store,
	client llm.Client,
	bootstrap cognitionpolicy.BrainBootstrap,
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
) (cognitionpolicy.ProviderProcessActivation, error) {
	brain := bootstrap.AttestedBrain
	outcome, observationErr := cognitionpolicy.ObserveProviderProcess(
		ctx, client, brain, episode, actor,
		cognitionpolicy.ProviderProcessEpisodeInvocation,
	)
	if observationErr == nil {
		activation, err := outcome.RequireSuccess(brain)
		if err != nil {
			return cognitionpolicy.ProviderProcessActivation{}, fmt.Errorf(
				"require successful cognition provider process activation: %w", err,
			)
		}
		return activation, nil
	}
	if outcome.Failure == nil {
		return cognitionpolicy.ProviderProcessActivation{}, errors.Join(
			observationErr,
			fmt.Errorf("cognition provider process failure lacks recordable raw evidence"),
		)
	}
	if err := outcome.ValidateFor(brain); err != nil {
		return cognitionpolicy.ProviderProcessActivation{}, errors.Join(
			observationErr, fmt.Errorf("validate cognition provider process failure: %w", err),
		)
	}
	if err := store.RecordProviderProcessFailure(ctx, bootstrap, *outcome.Failure); err != nil {
		return cognitionpolicy.ProviderProcessActivation{}, errors.Join(
			observationErr, fmt.Errorf("persist cognition provider process failure: %w", err),
		)
	}
	return cognitionpolicy.ProviderProcessActivation{}, fmt.Errorf(
		"observe cognition provider process: %w", observationErr,
	)
}
