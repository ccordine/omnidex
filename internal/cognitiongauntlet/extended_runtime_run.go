package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstore"
	"github.com/gryph/omnidex/internal/labyrinth"
)

func RunExtendedRuntime(
	ctx context.Context,
	generated labyrinth.ExtendedCase,
	request ExtendedRuntimeRunRequest,
) (ExtendedRuntimeReceipt, error) {
	return runExtendedRuntime(ctx, generated, request, "", false)
}

func RunRogueRuntime(
	ctx context.Context,
	generated labyrinth.ExtendedCase,
	request ExtendedRuntimeRunRequest,
	prerequisites RoguePrerequisiteBundle,
) (ExtendedRuntimeReceipt, error) {
	if err := prerequisites.Validate(); err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	return runExtendedRuntime(ctx, generated, request, prerequisites.BundleSHA256, true)
}

func runExtendedRuntime(
	ctx context.Context,
	generated labyrinth.ExtendedCase,
	request ExtendedRuntimeRunRequest,
	prerequisiteSHA256 string,
	allowRogue bool,
) (ExtendedRuntimeReceipt, error) {
	if ctx == nil {
		return ExtendedRuntimeReceipt{}, fmt.Errorf("extended runtime context is nil")
	}
	authority, err := extendedRuntimeAuthority(generated, request, allowRogue)
	if err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	episode, err := VariantEpisodeRef(authority, VariantFullCognition)
	if err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	binding, err := cognitionstore.BindAttempt(episode.ID, request.Attempt)
	if err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	brain, err := productionBrain(request.RatGeneration, authority.Budget.Station.MaxOutputTokens)
	if err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	scenario := generated.ExecutionScenario()
	components, err := newFullRuntimeComponents(
		ctx, request.Pool, request.Client, brain, request.RatGeneration.Fixed.Brain,
		request.HostStore, scenario, episode, request.Surface,
	)
	if err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	if err := components.store.AuthorizeAttempt(ctx, binding.Attempt); err != nil {
		return ExtendedRuntimeReceipt{}, fmt.Errorf("authorize extended runtime attempt: %w", err)
	}
	if err := validateExtendedWorkingSet(ctx, components, request, authority); err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	start, err := components.environment.Start(ctx, scenario.Ref())
	if err != nil {
		return ExtendedRuntimeReceipt{}, fmt.Errorf("start durable extended Labyrinth episode: %w", err)
	}
	stored, err := startExtendedRuntimeEpisode(
		ctx, components.store, request, authority, components.frozenBrain, episode, scenario, start,
	)
	if err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	components, err = activateRuntimeComponents(ctx, components, stored, binding.Attempt)
	if err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	run, err := components.runtime.Run(ctx, binding, cognitionruntime.RunLimits{
		MaxCycles: uint32(authority.Budget.RuntimeCycles),
	})
	if err != nil {
		source := fmt.Errorf("execute extended production cognition: %w", err)
		if _, registered := classifyRuntimeCancellation(source); !registered {
			return ExtendedRuntimeReceipt{}, source
		}
		if cancelErr := cancelFullCognitionRuntimeFailure(
			ctx, components, binding, source,
		); cancelErr != nil {
			return ExtendedRuntimeReceipt{}, cancelErr
		}
		return finishExtendedCanceledRuntime(
			ctx, generated, request, authority, episode, run, components, prerequisiteSHA256,
		)
	}
	if run.Terminal.State == cognitionruntime.StepEpisodeCanceled {
		return finishExtendedCanceledRuntime(
			ctx, generated, request, authority, episode, run, components, prerequisiteSHA256,
		)
	}
	if run.PolicyCalls > uint32(authority.Budget.ModelCalls) ||
		run.EnvironmentActions > uint32(authority.Budget.EnvironmentActions) ||
		run.Terminal.State != cognitionruntime.StepEpisodeCompleted || run.Terminal.Seal == nil {
		return ExtendedRuntimeReceipt{}, fmt.Errorf("extended production cognition did not complete within its frozen budget")
	}
	trace, err := readProductionTrace(ctx, components.repository, episode.ID)
	if err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	return admitExtendedRuntime(
		ctx, generated, request, authority, episode, run, trace, prerequisiteSHA256,
	)
}

func validateExtendedWorkingSet(
	ctx context.Context,
	components fullRuntimeComponents,
	request ExtendedRuntimeRunRequest,
	authority PairedRunAuthority,
) error {
	set, err := components.repository.CurrentWorkingSet(ctx, request.Attempt.JobID)
	if err != nil {
		return fmt.Errorf("load extended runtime Working Set: %w", err)
	}
	if set.Budget.MaxBytes != authority.Budget.WorkingSetBytes || string(set.Status) != "active" {
		return fmt.Errorf("extended runtime Working Set changed its frozen budget or is not active")
	}
	return nil
}

func extendedEvaluatorActor(attempt cognition.AttemptRef) cognition.AttemptRef {
	attempt.WorkerID += "-evaluator"
	return attempt
}
