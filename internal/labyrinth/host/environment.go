package host

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
)

// Environment is a stateless, PostgreSQL-authoritative Labyrinth host. Every
// call reconstructs a disposable kernel from the sealed scenario and committed
// successful receipts; no mutated candidate survives a failed database commit.
type Environment struct {
	store                *Store
	episode              cognition.EpisodeRef
	resolve              ScenarioResolver
	authorize            labyrinth.AttemptAuthorizer
	authorizeTransaction TransactionAttemptAuthorizer
	newKernel            kernelFactory
}

var _ cognition.Environment = (*Environment)(nil)

func NewEnvironment(
	store *Store,
	episode cognition.EpisodeRef,
	resolve ScenarioResolver,
	authorize labyrinth.AttemptAuthorizer,
	authorizeTransaction TransactionAttemptAuthorizer,
) (*Environment, error) {
	return NewSurfaceEnvironment(
		store, episode, resolve, authorize, authorizeTransaction, "symbolic.v1",
	)
}

func NewSurfaceEnvironment(
	store *Store,
	episode cognition.EpisodeRef,
	resolve ScenarioResolver,
	authorize labyrinth.AttemptAuthorizer,
	authorizeTransaction TransactionAttemptAuthorizer,
	surfaceVersion string,
) (*Environment, error) {
	if store == nil || store.pool == nil {
		return nil, ErrNotConfigured
	}
	if err := episode.Validate(); err != nil {
		return nil, err
	}
	if resolve == nil {
		return nil, fmt.Errorf("%w: sealed scenario resolver is required", ErrNotConfigured)
	}
	if authorize == nil {
		return nil, fmt.Errorf("%w: attempt authorizer is required", cognition.ErrAuthorityDenied)
	}
	if authorizeTransaction == nil {
		return nil, fmt.Errorf("%w: transactional attempt authorizer is required", cognition.ErrAuthorityDenied)
	}
	factory, err := registeredKernelFactory(surfaceVersion)
	if err != nil {
		return nil, err
	}
	return &Environment{
		store: store, episode: episode, resolve: resolve, authorize: authorize,
		authorizeTransaction: authorizeTransaction,
		newKernel:            factory,
	}, nil
}

func (environment *Environment) validate(ctx context.Context) error {
	if ctx == nil || environment == nil || environment.store == nil || environment.store.pool == nil ||
		environment.resolve == nil || environment.authorize == nil ||
		environment.authorizeTransaction == nil || environment.newKernel == nil {
		return ErrNotConfigured
	}
	if err := environment.episode.Validate(); err != nil {
		return err
	}
	return nil
}

func (environment *Environment) resolveScenario(
	ctx context.Context,
	reference cognition.ScenarioRef,
) (labyrinth.Scenario, error) {
	if err := reference.Validate(); err != nil {
		return labyrinth.Scenario{}, err
	}
	scenario, err := environment.resolve(ctx, reference)
	if err != nil {
		return labyrinth.Scenario{}, fmt.Errorf("resolve sealed Labyrinth scenario %q: %w", reference.ID, err)
	}
	if err := scenario.Validate(); err != nil {
		return labyrinth.Scenario{}, err
	}
	if scenario.Ref() != reference {
		return labyrinth.Scenario{}, fmt.Errorf("%w: resolver returned another public scenario", ErrScenarioConflict)
	}
	return scenario, nil
}

func historicalAuthorizer(_ context.Context, actor cognition.AttemptRef) error {
	return actor.Validate()
}
