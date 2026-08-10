package labyrinth

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

const omittedActionAvailable cognition.PredicateName = "counterfactual.available"

// VerifyExtendedOmissionRails proves both the exact failing trace and, with
// the omitted registered operation disabled, the absence of any alternate
// symbolic path to the goal. The counterfactual remains private evaluator
// state and never enters the public scenario or model context.
func VerifyExtendedOmissionRails(
	ctx context.Context,
	generated ExtendedCase,
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
) error {
	if err := generated.Validate(); err != nil {
		return err
	}
	for index, rail := range generated.oracle.OmissionRails {
		if err := verifyOmittedWitnessTrace(ctx, generated, episode, actor, index, rail); err != nil {
			return err
		}
		unreachable, err := goalUnreachableWithoutOperation(generated.execution, rail.OmittedKind)
		if err != nil {
			return err
		}
		if !unreachable {
			return fmt.Errorf("%w: omission rail %s retained an alternate goal path", ErrGeneration, rail.ID)
		}
	}
	return nil
}

func verifyOmittedWitnessTrace(
	ctx context.Context,
	generated ExtendedCase,
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
	ordinal int,
	rail ExtendedOmissionRail,
) error {
	railEpisode := cognition.EpisodeRef{ID: cognition.EpisodeID(fmt.Sprintf("%s-omit-%d", episode.ID, ordinal+1))}
	environment, err := newExtendedOracleEnvironment(generated, railEpisode, actor)
	if err != nil {
		return err
	}
	started, err := environment.Start(ctx, generated.execution.Ref())
	if err != nil {
		return err
	}
	current := started.Current
	observations := make(map[cognition.ActionID][]cognition.Observation)
	for index, witness := range generated.oracle.Witness {
		if index == rail.OmittedAction {
			continue
		}
		transition, applyErr := applyExtendedWitnessAction(
			ctx, environment, railEpisode, actor, generated, witness, current, observations,
		)
		if index == rail.FailureAction {
			var failure cognition.ActionFailure
			if !errors.As(applyErr, &failure) || failure.Code != rail.FailureCode || transition.Terminal {
				return fmt.Errorf("%w: omission rail %s failure=%v", ErrGeneration, rail.ID, applyErr)
			}
			return nil
		}
		if applyErr != nil {
			return fmt.Errorf("%w: omission rail %s failed early at %d: %v", ErrGeneration, rail.ID, index, applyErr)
		}
		if transition.Terminal {
			return fmt.Errorf("%w: omission rail %s reached the goal", ErrGeneration, rail.ID)
		}
		current = transition.Current
		observations[witness.ID] = append([]cognition.Observation{}, transition.Observations...)
	}
	return fmt.Errorf("%w: omission rail %s had no registered failure", ErrGeneration, rail.ID)
}

func goalUnreachableWithoutOperation(
	scenario Scenario,
	omittedKind cognition.ActionKind,
) (bool, error) {
	definition := scenario.definition.clone()
	definition.predicateSchemas = append(definition.predicateSchemas, PredicateSchema{
		Name: omittedActionAvailable, ArgumentKinds: []EntityKind{}, Public: false,
	})
	found := false
	for index := range definition.actions {
		if definition.actions[index].Schema.Kind != omittedKind {
			continue
		}
		definition.actions[index].Preconditions = append(definition.actions[index].Preconditions,
			condition(ConditionPresent, omittedActionAvailable),
		)
		found = true
	}
	if !found {
		return false, fmt.Errorf("%w: omission rail operation is absent", ErrGeneration)
	}
	definition.sha256 = ""
	if err := definition.reseal(); err != nil {
		return false, err
	}
	counterfactual, err := NewScenario(
		cognition.ScenarioID(string(scenario.Ref().ID)+"-omission"), definition, scenario.descriptor,
	)
	if err != nil {
		return false, err
	}
	_, err = Solve(counterfactual, SolverBounds{MaxStates: 20_000, MaxGroundActions: MaxSolverGroundActions})
	if errors.Is(err, ErrUnsolvable) {
		return true, nil
	}
	return false, err
}
