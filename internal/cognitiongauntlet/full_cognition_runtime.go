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
	repository  *queue.Repository
	store       *cognitionstore.Store
	environment cognition.Environment
	runtime     *cognitionruntime.Runtime
	brain       cognitionpolicy.AttestedBrain
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
	repository := queue.New(pool)
	facts, err := newVisibleObservationFactAuthority()
	if err != nil {
		return fullRuntimeComponents{}, err
	}
	store, err := cognitionstore.New(repository, facts)
	if err != nil {
		return fullRuntimeComponents{}, err
	}
	attestedBrain, err := cognitionpolicy.AttestBrain(ctx, client, brain)
	if err != nil {
		return fullRuntimeComponents{}, err
	}
	frozenBrain, err := frozen.attestedBrain()
	if err != nil {
		return fullRuntimeComponents{}, err
	}
	if attestedBrain != frozenBrain {
		return fullRuntimeComponents{}, fmt.Errorf(
			"live provider or host attestation differs from the frozen Rat authority",
		)
	}
	policy, err := cognitionpolicy.New(client, attestedBrain, store, store)
	if err != nil {
		return fullRuntimeComponents{}, err
	}
	runtime, err := cognitionruntime.New(cognitionruntime.Dependencies{
		Policy: policy, Environment: environment, Snapshots: store, Accepted: store, PolicyRecovery: store,
		Completion: completion,
		Episodes:   store, Reconciler: store, Actions: store, TerminalSeal: store,
	})
	if err != nil {
		return fullRuntimeComponents{}, err
	}
	return fullRuntimeComponents{
		repository: repository, store: store, environment: environment, runtime: runtime,
		brain: attestedBrain,
	}, nil
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
