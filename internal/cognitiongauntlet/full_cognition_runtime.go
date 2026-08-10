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
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fullRuntimeComponents struct {
	repository     *queue.Repository
	store          *cognitionstore.Store
	environment    cognition.Environment
	completion     cognitionruntime.CompletionEvaluator
	client         llm.Client
	frozenBrain    cognitionpolicy.AttestedBrain
	runtime        *cognitionruntime.Runtime
	brain          cognitionpolicy.AttestedBrain
	liveStaleProbe *liveStalePortController
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
	return newRuntimeComponents(ctx, pool, client, brain, frozen, environment, environment)
}

func newRuntimeComponents(
	ctx context.Context,
	pool *pgxpool.Pool,
	client llm.Client,
	brain cognitionpolicy.BrainRef,
	frozen BrainFingerprint,
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
	return fullRuntimeComponents{
		repository: repository, store: store, environment: environment,
		completion: completion, client: client, frozenBrain: frozenBrain,
	}, nil
}

func activateRuntimeComponents(
	ctx context.Context,
	components fullRuntimeComponents,
	episode queue.CognitionEpisode,
	actor cognition.AttemptRef,
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
	liveHost, err := cognitionpolicy.AttestLocalHostHardware()
	if err != nil {
		return fullRuntimeComponents{}, fmt.Errorf("attest cognition runtime host: %w", err)
	}
	if liveHost != episode.AttestedBrain.Host {
		return fullRuntimeComponents{}, fmt.Errorf(
			"live host differs from the frozen Rat authority",
		)
	}
	observation, err := cognitionpolicy.ObserveProviderProcess(
		ctx, components.client, episode.AttestedBrain,
		cognition.EpisodeRef{ID: episode.EpisodeID}, actor,
		cognitionpolicy.ProviderProcessEpisodeInvocation,
	)
	if err != nil {
		return fullRuntimeComponents{}, fmt.Errorf("observe cognition provider process: %w", err)
	}
	if err := components.store.RecordProviderProcessObservation(ctx, observation); err != nil {
		return fullRuntimeComponents{}, fmt.Errorf("record cognition provider process observation: %w", err)
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
	policy, err := cognitionpolicy.New(components.client, episode.AttestedBrain, components.store, journal)
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
