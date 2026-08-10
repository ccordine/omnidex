package cognitiongauntlet

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
)

type privateGoalEnvironment interface {
	cognition.Environment
	EvaluateGoal(
		context.Context,
		cognition.EpisodeRef,
		cognition.WorldRevision,
		cognition.GoalExpression,
	) (bool, error)
}

// derivePrivateEvaluationEvidence reconstructs world truth after inference has
// stopped. It replays only sealed successful actions; failed action attempts are
// non-mutating evidence and cannot contribute to the reconstructed state.
func derivePrivateEvaluationEvidence(
	ctx context.Context,
	fixture MicrogauntletCase,
	surfaceVersion string,
	episode SealedEpisode,
) (SymbolicEvaluationEvidence, error) {
	return derivePrivateScenarioEvaluationEvidence(
		ctx, fixture.SealedEnvironmentScenario(), surfaceVersion, episode,
	)
}

func derivePrivateScenarioEvaluationEvidence(
	ctx context.Context,
	scenario labyrinth.Scenario,
	surfaceVersion string,
	episode SealedEpisode,
) (SymbolicEvaluationEvidence, error) {
	if ctx == nil {
		return SymbolicEvaluationEvidence{}, fmt.Errorf("private episode replay context is nil")
	}
	if err := episode.Validate(); err != nil {
		return SymbolicEvaluationEvidence{}, err
	}
	surface, err := surfaceFromVersion(surfaceVersion)
	if err != nil {
		return SymbolicEvaluationEvidence{}, err
	}
	ref := cognition.EpisodeRef{ID: episode.Manifest.EpisodeID}
	actions, actors, err := replayActionTraces(episode, ref)
	if err != nil {
		return SymbolicEvaluationEvidence{}, err
	}
	authorize := func(_ context.Context, actor cognition.AttemptRef) error {
		if err := actor.Validate(); err != nil {
			return cognition.ErrAuthorityDenied
		}
		if _, exists := actors[actor]; !exists {
			return cognition.ErrAuthorityDenied
		}
		return nil
	}
	base, closeEnvironment, err := newScenarioEnvironmentWithAuthorizer(
		scenario, ref, authorize, surface,
	)
	if err != nil {
		return SymbolicEvaluationEvidence{}, err
	}
	environment, ok := base.(privateGoalEnvironment)
	if !ok {
		_ = closeEnvironment()
		return SymbolicEvaluationEvidence{}, fmt.Errorf("private replay environment lacks goal authority")
	}
	started, err := environment.Start(ctx, scenario.Ref())
	if err != nil {
		_ = closeEnvironment()
		return SymbolicEvaluationEvidence{}, fmt.Errorf("start private episode replay: %w", err)
	}
	current, observations := started, make(map[cognition.ObservationID]cognition.Observation)
	if err := collectReplayObservations(observations, started); err != nil {
		_ = closeEnvironment()
		return SymbolicEvaluationEvidence{}, err
	}
	for _, trace := range actions {
		if trace.Transition == nil {
			continue
		}
		if trace.ExpectedRevision != current.Current {
			_ = closeEnvironment()
			return SymbolicEvaluationEvidence{}, fmt.Errorf("sealed accepted action is not contiguous with private world state")
		}
		actual, applyErr := environment.Apply(ctx, ref, current.Current, trace.Action)
		if applyErr != nil {
			_ = closeEnvironment()
			return SymbolicEvaluationEvidence{}, fmt.Errorf("replay sealed accepted action %q: %w", trace.Action.ID, applyErr)
		}
		if !reflect.DeepEqual(actual, *trace.Transition) {
			_ = closeEnvironment()
			return SymbolicEvaluationEvidence{}, fmt.Errorf("sealed action transition %q differs from private replay", trace.Action.ID)
		}
		if err := collectReplayObservations(observations, actual); err != nil {
			_ = closeEnvironment()
			return SymbolicEvaluationEvidence{}, err
		}
		current = actual
	}
	goalSatisfied, err := environment.EvaluateGoal(
		ctx, ref, current.Current, scenario.Goal(),
	)
	closeErr := closeEnvironment()
	if err != nil {
		return SymbolicEvaluationEvidence{}, fmt.Errorf("evaluate private replay goal: %w", err)
	}
	if closeErr != nil {
		return SymbolicEvaluationEvidence{}, fmt.Errorf("close private replay environment: %w", closeErr)
	}
	if goalSatisfied != current.Terminal {
		return SymbolicEvaluationEvidence{}, fmt.Errorf("private replay terminal state disagrees with its exact goal")
	}
	if current.Current != episode.Manifest.FinalRevision {
		return SymbolicEvaluationEvidence{}, fmt.Errorf("sealed final revision differs from private replay")
	}
	if goalSatisfied != episode.Manifest.Outcome.GoalSatisfied {
		return SymbolicEvaluationEvidence{}, fmt.Errorf("sealed goal outcome differs from private replay")
	}
	if err := verifyReplayObservations(episode, observations); err != nil {
		return SymbolicEvaluationEvidence{}, err
	}
	return SymbolicEvaluationEvidence{
		GoalPredicateSatisfied: goalSatisfied,
		ValidTerminalState:     current.Terminal,
		ActualDecisionCost:     int64(episode.Manifest.Resources.ModelDecisions),
		PrivateAuthorityRefs:   []string{},
	}, nil
}

func replayActionTraces(
	episode SealedEpisode,
	ref cognition.EpisodeRef,
) ([]ActionTrace, map[cognition.AttemptRef]struct{}, error) {
	actions := make([]ActionTrace, 0, episode.Manifest.Resources.EnvironmentActions)
	actors := make(map[cognition.AttemptRef]struct{})
	for _, entry := range episode.Manifest.Trace {
		if entry.Kind != TraceAction {
			continue
		}
		trace, err := decodeActionTrace(entry, ref)
		if err != nil {
			return nil, nil, err
		}
		if trace.Transition != nil {
			actors[trace.Action.Actor] = struct{}{}
		}
		actions = append(actions, trace)
	}
	return actions, actors, nil
}

func surfaceFromVersion(version string) (Surface, error) {
	for _, surface := range []Surface{SurfaceSymbolic, SurfaceFilesystem, SurfaceRecord} {
		candidate, err := surface.Version()
		if err == nil && candidate == version {
			return surface, nil
		}
	}
	return "", fmt.Errorf("private replay surface version %q is not registered", version)
}
