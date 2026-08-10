package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionstore"
	"github.com/gryph/omnidex/internal/labyrinth"
	"github.com/gryph/omnidex/internal/queue"
)

func RunFullCognition(
	ctx context.Context,
	fixture MicrogauntletCase,
	request FullCognitionRunRequest,
) (FullCognitionRunResult, error) {
	if ctx == nil {
		return FullCognitionRunResult{}, fmt.Errorf("full cognition run context is nil")
	}
	if err := request.Validate(); err != nil {
		return FullCognitionRunResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return FullCognitionRunResult{}, err
	}
	authority, err := fixture.PairedAuthority(
		request.Surface, request.RatGeneration, request.Repetition, request.RuntimeFingerprint,
	)
	if err != nil {
		return FullCognitionRunResult{}, err
	}
	if err := validateRestartSchedule(request.RestartAfterCycles, authority.Budget.RuntimeCycles); err != nil {
		return FullCognitionRunResult{}, err
	}
	episode, err := VariantEpisodeRef(authority, VariantFullCognition)
	if err != nil {
		return FullCognitionRunResult{}, err
	}
	binding, err := cognitionstore.BindAttempt(episode.ID, request.Attempt)
	if err != nil {
		return FullCognitionRunResult{}, err
	}
	brain, err := productionBrain(request.RatGeneration, authority.Budget.Station.MaxOutputTokens)
	if err != nil {
		return FullCognitionRunResult{}, err
	}
	scenario := fixture.generated.ExecutionScenario()
	components, err := newFullRuntimeComponents(
		ctx, request.Pool, request.Client, brain, request.RatGeneration.Fixed.Brain, request.HostStore,
		scenario, episode, request.Surface,
	)
	if err != nil {
		return FullCognitionRunResult{}, err
	}
	if err := components.store.AuthorizeAttempt(ctx, binding.Attempt); err != nil {
		return FullCognitionRunResult{}, fmt.Errorf("authorize full cognition attempt: %w", err)
	}
	workingSet, err := components.repository.CurrentWorkingSet(ctx, request.Attempt.JobID)
	if err != nil {
		return FullCognitionRunResult{}, fmt.Errorf("load full cognition Working Set: %w", err)
	}
	if workingSet.Budget.MaxBytes != authority.Budget.WorkingSetBytes || string(workingSet.Status) != "active" {
		return FullCognitionRunResult{}, fmt.Errorf("full cognition Working Set changed its frozen budget or is not active")
	}
	start, err := components.environment.Start(ctx, scenario.Ref())
	if err != nil {
		return FullCognitionRunResult{}, fmt.Errorf("start durable Labyrinth episode: %w", err)
	}
	stored, err := startFullCognitionEpisode(
		ctx, components.store, request, authority, components.frozenBrain, episode, scenario, start,
	)
	if err != nil {
		return FullCognitionRunResult{}, err
	}
	components, err = activateRuntimeComponents(ctx, components, stored, binding.Attempt)
	if err != nil {
		return FullCognitionRunResult{}, err
	}

	run, restarts, err := executeFullCognition(
		ctx, request, authority, episode, scenario, brain, binding, components,
	)
	if err != nil {
		if _, registered := classifyRuntimeCancellation(err); !registered {
			return FullCognitionRunResult{}, err
		}
		if cancelErr := cancelFullCognitionRuntimeFailure(ctx, components, binding, err); cancelErr != nil {
			return FullCognitionRunResult{}, cancelErr
		}
	}
	sealed, err := sealFullCognitionExecution(
		ctx, fixture, request, authority, episode, components, run, restarts,
	)
	if err != nil {
		return FullCognitionRunResult{}, err
	}
	return finishFullCognition(request, authority, fixture, sealed)
}

func startFullCognitionEpisode(
	ctx context.Context,
	store *cognitionstore.Store,
	request FullCognitionRunRequest,
	authority PairedRunAuthority,
	brain cognitionpolicy.AttestedBrain,
	episode cognition.EpisodeRef,
	scenario labyrinth.Scenario,
	start cognition.Transition,
) (queue.CognitionEpisode, error) {
	budget, err := authority.Budget.RuntimeBudget()
	if err != nil {
		return queue.CognitionEpisode{}, err
	}
	goal := scenario.Goal()
	completion, err := labyrinth.NewCompletionAuthority(scenario)
	if err != nil {
		return queue.CognitionEpisode{}, err
	}
	check, err := completion.Resolve(goal)
	if err != nil {
		return queue.CognitionEpisode{}, err
	}
	rootID, err := cognition.DeriveObligationID(episode.ID, cognition.InitialObligationGeneration, "", goal, check)
	if err != nil {
		return queue.CognitionEpisode{}, err
	}
	if err := cognitionpolicy.ValidateRuntimeBudget(brain.Ref, budget); err != nil {
		return queue.CognitionEpisode{}, fmt.Errorf("validate full cognition runtime budget: %w", err)
	}
	stored, err := store.StartEpisode(ctx, queue.CognitionEpisodeStart{
		Authority: request.Attempt, EpisodeID: episode.ID, Scenario: scenario.Ref(),
		AttestedBrain: brain,
		Goal:          goal, Completion: completion, ActionCatalog: scenario.Catalog(), Budget: budget,
		Root: cognition.ObligationSpec{
			ID: rootID, Desired: goal, DependsOn: []cognition.ObligationID{},
			SupportingRefs: []cognition.EvidenceRef{}, CompletionCheck: check,
		},
		Transition: start,
	})
	if err != nil {
		return queue.CognitionEpisode{}, fmt.Errorf("start production cognition episode: %w", err)
	}
	return stored, nil
}
