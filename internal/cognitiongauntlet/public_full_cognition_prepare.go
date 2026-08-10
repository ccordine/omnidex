package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstore"
)

type publicFullCognitionExecution struct {
	episode    cognition.EpisodeRef
	binding    cognitionruntime.Binding
	components fullRuntimeComponents
}

func preparePublicFullCognition(
	ctx context.Context,
	bundle PublicInferenceBundle,
	request PublicFullCognitionRunRequest,
) (publicFullCognitionExecution, error) {
	if ctx == nil {
		return publicFullCognitionExecution{}, fmt.Errorf("public full cognition context is nil")
	}
	if err := request.validate(bundle); err != nil {
		return publicFullCognitionExecution{}, err
	}
	episode, err := PublicVariantEpisodeRef(bundle.Authority)
	if err != nil {
		return publicFullCognitionExecution{}, err
	}
	binding, err := cognitionstore.BindAttempt(episode.ID, request.Attempt)
	if err != nil {
		return publicFullCognitionExecution{}, err
	}
	brain, err := productionBrain(
		bundle.Authority.RatGeneration, bundle.Authority.Budget.Station.MaxOutputTokens,
	)
	if err != nil {
		return publicFullCognitionExecution{}, err
	}
	components, err := newRuntimeComponents(
		ctx, request.Pool, request.Client, brain, bundle.Authority.RatGeneration.Fixed.Brain,
		request.Environment, request.Completion,
	)
	if err != nil {
		return publicFullCognitionExecution{}, err
	}
	if err := components.store.AuthorizeAttempt(ctx, binding.Attempt); err != nil {
		return publicFullCognitionExecution{}, fmt.Errorf("authorize public cognition attempt: %w", err)
	}
	set, err := components.repository.CurrentWorkingSet(ctx, request.Attempt.JobID)
	if err != nil {
		return publicFullCognitionExecution{}, fmt.Errorf("load public cognition Working Set: %w", err)
	}
	if set.Budget.MaxBytes != bundle.Authority.Budget.WorkingSetBytes || string(set.Status) != "active" {
		return publicFullCognitionExecution{}, fmt.Errorf(
			"public cognition Working Set changed its frozen budget or is not active",
		)
	}
	start, err := request.Environment.Start(ctx, bundle.Authority.Scenario)
	if err != nil {
		return publicFullCognitionExecution{}, fmt.Errorf("start public cognition environment: %w", err)
	}
	if err := startPublicCognitionEpisode(
		ctx, components.store, request.Attempt, bundle, components.brain, episode, start,
	); err != nil {
		return publicFullCognitionExecution{}, err
	}
	return publicFullCognitionExecution{
		episode: episode, binding: binding, components: components,
	}, nil
}

func finishPublicFullCognition(
	ctx context.Context,
	bundle PublicInferenceBundle,
	request PublicFullCognitionRunRequest,
	execution publicFullCognitionExecution,
	run cognitionruntime.RunResult,
) (PublicFullCognitionRunResult, error) {
	if int(run.PolicyCalls) > bundle.Authority.Budget.ModelCalls ||
		int(run.EnvironmentActions) > bundle.Authority.Budget.EnvironmentActions ||
		int(run.EnvironmentActions) > bundle.Authority.Budget.ToolOperations {
		return PublicFullCognitionRunResult{}, fmt.Errorf("public cognition execution exceeded its frozen budget")
	}
	return sealPublicFullCognition(
		ctx, request, bundle, execution.episode, execution.components, run,
	)
}
