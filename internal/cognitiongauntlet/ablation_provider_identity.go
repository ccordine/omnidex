package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
)

func attestAblationBrain(
	ctx context.Context,
	client llm.Client,
	brain cognitionpolicy.BrainRef,
) (cognitionpolicy.BrainBootstrap, error) {
	outcome, observationErr := cognitionpolicy.AttestBrain(ctx, client, brain)
	if observationErr != nil {
		return cognitionpolicy.BrainBootstrap{}, newAblationBootstrapFailureError(
			outcome, observationErr,
		)
	}
	bootstrap, err := outcome.RequireSuccess()
	if err != nil {
		return cognitionpolicy.BrainBootstrap{}, fmt.Errorf(
			"require successful ablation Brain bootstrap: %w", err,
		)
	}
	return bootstrap, nil
}

func observeAblationProviderProcess(
	ctx context.Context,
	client llm.Client,
	brain cognitionpolicy.AttestedBrain,
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
) (cognitionpolicy.ProviderProcessActivation, error) {
	outcome, observationErr := cognitionpolicy.ObserveProviderProcess(
		ctx, client, brain, episode, actor,
		cognitionpolicy.ProviderProcessEpisodeInvocation,
	)
	if observationErr != nil {
		return cognitionpolicy.ProviderProcessActivation{}, newAblationProcessFailureError(
			outcome, brain, observationErr,
		)
	}
	activation, err := outcome.RequireSuccess(brain)
	if err != nil {
		return cognitionpolicy.ProviderProcessActivation{}, fmt.Errorf(
			"require successful ablation provider process activation: %w", err,
		)
	}
	return activation, nil
}
