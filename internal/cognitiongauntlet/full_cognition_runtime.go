package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstore"
	"github.com/gryph/omnidex/internal/labyrinth"
	labyrinthhost "github.com/gryph/omnidex/internal/labyrinth/host"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fullRuntimeComponents struct {
	repository         *queue.Repository
	store              *cognitionstore.Store
	environment        cognition.Environment
	completion         cognitionruntime.CompletionEvaluator
	client             llm.Client
	frozenBrain        cognitionpolicy.AttestedBrain
	frozenFingerprint  BrainFingerprint
	brainBootstrap     cognitionpolicy.BrainBootstrap
	providerActivation cognitionpolicy.ProviderProcessActivation
	runtime            *cognitionruntime.Runtime
	brain              cognitionpolicy.AttestedBrain
	liveStaleProbe     *liveStalePortController
}

func newFullRuntimeComponents(
	ctx context.Context,
	pool *pgxpool.Pool,
	client llm.Client,
	brain cognitionpolicy.BrainRef,
	frozen BrainFingerprint,
	hostStore *labyrinthhost.Store,
	scenario labyrinth.Scenario,
	episode cognition.EpisodeRef,
	authority model.StepAttemptAuthority,
	surface Surface,
) (fullRuntimeComponents, error) {
	surfaceVersion, err := surface.Version()
	if err != nil {
		return fullRuntimeComponents{}, err
	}
	repository := queue.New(pool)
	facts, err := newVisibleObservationFactAuthority()
	if err != nil {
		return fullRuntimeComponents{}, err
	}
	store, err := cognitionstore.New(repository, facts)
	if err != nil {
		return fullRuntimeComponents{}, err
	}
	resolver := func(_ context.Context, reference cognition.ScenarioRef) (labyrinth.Scenario, error) {
		if reference != scenario.Ref() {
			return labyrinth.Scenario{}, fmt.Errorf("sealed scenario resolver rejected another scenario")
		}
		return scenario, nil
	}
	environment, err := labyrinthhost.NewSurfaceEnvironment(
		hostStore, episode, resolver, store.AuthorizeAttempt,
		transactionAttemptAuthorizer(repository), surfaceVersion,
	)
	if err != nil {
		return fullRuntimeComponents{}, err
	}
	return newRuntimeComponents(
		ctx, pool, client, brain, frozen, episode, authority, environment, environment,
	)
}

func newRuntimeComponents(
	ctx context.Context,
	pool *pgxpool.Pool,
	client llm.Client,
	brain cognitionpolicy.BrainRef,
	frozen BrainFingerprint,
	episode cognition.EpisodeRef,
	authority model.StepAttemptAuthority,
	environment cognition.Environment,
	completion cognitionruntime.CompletionEvaluator,
) (fullRuntimeComponents, error) {
	if ctx == nil {
		return fullRuntimeComponents{}, fmt.Errorf("cognition runtime construction context is nil")
	}
	repository := queue.New(pool)
	facts, err := newVisibleObservationFactAuthority()
	if err != nil {
		return fullRuntimeComponents{}, err
	}
	store, err := cognitionstore.New(repository, facts)
	if err != nil {
		return fullRuntimeComponents{}, err
	}
	frozenBrain, err := frozen.attestedBrain()
	if err != nil {
		return fullRuntimeComponents{}, err
	}
	if brain != frozenBrain.Ref {
		return fullRuntimeComponents{}, fmt.Errorf(
			"production brain differs from the frozen Rat authority",
		)
	}
	bootstrap, err := attestPersistedRuntimeBrain(
		ctx, store, client, brain, authority, episode,
	)
	if err != nil {
		return fullRuntimeComponents{}, err
	}
	if !sameFrozenBrain(bootstrap.AttestedBrain, frozenBrain) {
		return fullRuntimeComponents{}, fmt.Errorf(
			"live provider or host differs from the frozen Rat authority",
		)
	}
	return fullRuntimeComponents{
		repository: repository, store: store, environment: environment,
		completion: completion, client: client, frozenBrain: frozenBrain,
		frozenFingerprint: frozen, brainBootstrap: bootstrap,
	}, nil
}

func observeRuntimeProviderActivation(
	ctx context.Context,
	components fullRuntimeComponents,
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
) (cognitionpolicy.ProviderProcessActivation, error) {
	if ctx == nil || nilRunDependency(components.client) {
		return cognitionpolicy.ProviderProcessActivation{}, fmt.Errorf(
			"cognition provider process observation dependencies are incomplete",
		)
	}
	activation, err := observePersistedRuntimeProviderProcess(
		ctx, components.store, components.client, components.brainBootstrap,
		episode, actor,
	)
	if err != nil {
		return cognitionpolicy.ProviderProcessActivation{}, err
	}
	return activation, nil
}

func activateRuntimeComponents(
	ctx context.Context,
	components fullRuntimeComponents,
	episode queue.CognitionEpisode,
	activation cognitionpolicy.ProviderProcessActivation,
) (fullRuntimeComponents, error) {
	if ctx == nil || components.repository == nil || components.store == nil ||
		nilRunDependency(components.client) || nilRunDependency(components.environment) ||
		nilRunDependency(components.completion) {
		return fullRuntimeComponents{}, fmt.Errorf("cognition runtime activation dependencies are incomplete")
	}
	if !sameFrozenBrain(episode.AttestedBrain, components.frozenBrain) {
		return fullRuntimeComponents{}, fmt.Errorf(
			"stored cognition brain differs from the frozen Rat authority",
		)
	}
	if err := activation.ValidateFor(episode.AttestedBrain); err != nil {
		return fullRuntimeComponents{}, fmt.Errorf("validate cognition provider process activation: %w", err)
	}
	if activation.Receipt.EpisodeID != episode.EpisodeID {
		return fullRuntimeComponents{}, fmt.Errorf(
			"cognition provider process activation belongs to another episode",
		)
	}
	activationAuthority, err := activation.Authority()
	if err != nil {
		return fullRuntimeComponents{}, fmt.Errorf("derive cognition provider process activation authority: %w", err)
	}
	var journal cognitionpolicy.CallJournal = components.store
	var environment cognition.Environment = components.environment
	var reconciler cognitionruntime.DecisionReconciler = components.store
	var episodes cognitionruntime.EpisodeJournal = components.store
	if components.liveStaleProbe != nil {
		journal = liveStaleCallJournal{probe: components.liveStaleProbe, base: journal}
		environment = liveStaleEnvironment{probe: components.liveStaleProbe, base: environment}
		reconciler = liveStaleReconciler{probe: components.liveStaleProbe, base: reconciler}
		episodes = liveStaleEpisodeJournal{probe: components.liveStaleProbe, base: episodes}
	}
	policy, err := cognitionpolicy.New(
		components.client, episode.AttestedBrain, activationAuthority, components.store, journal,
	)
	if err != nil {
		return fullRuntimeComponents{}, err
	}
	runtime, err := cognitionruntime.New(cognitionruntime.Dependencies{
		Policy: policy, Environment: environment, Snapshots: components.store,
		Accepted: components.store, PolicyRecovery: components.store,
		Completion: components.completion,
		Episodes:   episodes, Reconciler: reconciler,
		Actions: components.store, TerminalSeal: components.store,
	})
	if err != nil {
		return fullRuntimeComponents{}, err
	}
	components.runtime = runtime
	components.brain = episode.AttestedBrain
	components.providerActivation = activation
	return components, nil
}

func productionBrain(
	generation RatGeneration,
	maxOutputTokens int,
) (cognitionpolicy.BrainRef, error) {
	brain := generation.Fixed.Brain
	if maxOutputTokens <= 0 || maxOutputTokens > brain.Sampling.MaxOutputTokens {
		return cognitionpolicy.BrainRef{}, fmt.Errorf(
			"runtime output ceiling exceeds the frozen Rat sampling identity",
		)
	}
	result, err := cognitionpolicy.NewBrainRef(
		brain.Model, brain.Digest, brain.Quantization, brain.Backend,
		brain.BackendVersion, brain.Hardware, brain.Sampling,
	)
	if err != nil {
		return cognitionpolicy.BrainRef{}, err
	}
	if result.SamplingSHA256 != brain.SamplingSHA256 ||
		result.ContextCeilingBytes != generation.Fixed.ContextCeilingBytes {
		return cognitionpolicy.BrainRef{}, fmt.Errorf(
			"frozen Rat sampling hash differs from the code-owned cognition policy contract",
		)
	}
	return result, nil
}
