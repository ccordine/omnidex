package host

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
)

type replayKernel interface {
	Start(context.Context, cognition.ScenarioRef) (cognition.Transition, error)
	Apply(context.Context, cognition.EpisodeRef, cognition.WorldRevision, cognition.RegisteredAction) (cognition.Transition, error)
	EvaluateGoal(context.Context, cognition.EpisodeRef, cognition.WorldRevision, cognition.GoalExpression) (bool, error)
}

type kernelCandidate struct {
	replayKernel
	close func() error
}

func (candidate kernelCandidate) Close() error {
	if candidate.close == nil {
		return fmt.Errorf("%w: Labyrinth kernel close authority is absent", ErrNotConfigured)
	}
	return candidate.close()
}

type kernelFactory func(
	labyrinth.Scenario,
	cognition.EpisodeRef,
	labyrinth.AttemptAuthorizer,
) (kernelCandidate, error)

func registeredKernelFactory(surface string) (kernelFactory, error) {
	switch surface {
	case "symbolic.v1":
		return func(scenario labyrinth.Scenario, episode cognition.EpisodeRef, authorize labyrinth.AttemptAuthorizer) (kernelCandidate, error) {
			kernel, err := labyrinth.NewEnvironment(scenario, episode, authorize)
			if err != nil {
				return kernelCandidate{}, err
			}
			return kernelCandidate{replayKernel: kernel, close: func() error { return nil }}, nil
		}, nil
	case labyrinth.FilesystemSurfaceVersionV1:
		return func(scenario labyrinth.Scenario, episode cognition.EpisodeRef, authorize labyrinth.AttemptAuthorizer) (kernelCandidate, error) {
			kernel, err := labyrinth.NewFilesystemEnvironment(scenario, episode, authorize)
			if err != nil {
				return kernelCandidate{}, err
			}
			return kernelCandidate{replayKernel: kernel, close: kernel.Close}, nil
		}, nil
	case labyrinth.RecordSurfaceVersionV1:
		return func(scenario labyrinth.Scenario, episode cognition.EpisodeRef, authorize labyrinth.AttemptAuthorizer) (kernelCandidate, error) {
			kernel, err := labyrinth.NewRecordEnvironment(scenario, episode, authorize)
			if err != nil {
				return kernelCandidate{}, err
			}
			return kernelCandidate{replayKernel: kernel, close: kernel.Close}, nil
		}, nil
	default:
		return nil, fmt.Errorf("%w: Labyrinth host surface %q is not registered", ErrNotConfigured, surface)
	}
}
