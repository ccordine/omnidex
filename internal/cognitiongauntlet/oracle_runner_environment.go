package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
)

type environmentClose func() error

func newBenchmarkEnvironment(
	fixture MicrogauntletCase,
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
	surface Surface,
) (cognition.Environment, environmentClose, error) {
	authorize := func(_ context.Context, candidate cognition.AttemptRef) error {
		if candidate != actor {
			return cognition.ErrAuthorityDenied
		}
		return nil
	}
	return newBenchmarkEnvironmentWithAuthorizer(fixture, episode, authorize, surface)
}

func newBenchmarkEnvironmentWithAuthorizer(
	fixture MicrogauntletCase,
	episode cognition.EpisodeRef,
	authorize labyrinth.AttemptAuthorizer,
	surface Surface,
) (cognition.Environment, environmentClose, error) {
	return newScenarioEnvironmentWithAuthorizer(
		fixture.SealedEnvironmentScenario(), episode, authorize, surface,
	)
}

func newScenarioEnvironmentWithAuthorizer(
	scenario labyrinth.Scenario,
	episode cognition.EpisodeRef,
	authorize labyrinth.AttemptAuthorizer,
	surface Surface,
) (cognition.Environment, environmentClose, error) {
	if authorize == nil {
		return nil, nil, fmt.Errorf("benchmark environment requires exact attempt authority")
	}
	switch surface {
	case SurfaceSymbolic:
		environment, err := labyrinth.NewEnvironment(scenario, episode, authorize)
		return environment, func() error { return nil }, err
	case SurfaceFilesystem:
		environment, err := labyrinth.NewFilesystemEnvironment(scenario, episode, authorize)
		if err != nil {
			return nil, nil, err
		}
		return environment, environment.Close, nil
	case SurfaceRecord:
		environment, err := labyrinth.NewRecordEnvironment(scenario, episode, authorize)
		if err != nil {
			return nil, nil, err
		}
		return environment, environment.Close, nil
	default:
		return nil, nil, fmt.Errorf("oracle baseline surface %q is not registered", surface)
	}
}
