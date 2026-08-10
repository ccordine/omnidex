package labyrinth

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

var witnessActor = cognition.AttemptRef{
	JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "witness-verifier",
}

func VerifyWitness(generated GeneratedCase) (cognition.Transition, int, error) {
	if err := generated.Validate(); err != nil {
		return cognition.Transition{}, 0, err
	}
	return verifyScenarioWitness(generated.execution, generated.oracle.Witness)
}

func verifyScenarioWitness(
	scenario Scenario,
	witness []WitnessAction,
) (cognition.Transition, int, error) {
	episode := cognition.EpisodeRef{ID: "witness-episode"}
	environment, err := NewEnvironment(scenario, episode, func(_ context.Context, actor cognition.AttemptRef) error {
		if actor != witnessActor {
			return cognition.ErrAuthorityDenied
		}
		return nil
	})
	if err != nil {
		return cognition.Transition{}, 0, err
	}
	transition, err := environment.Start(context.Background(), scenario.Ref())
	if err != nil {
		return cognition.Transition{}, 0, err
	}
	total := 0
	observations := append([]cognition.Observation(nil), transition.Observations...)
	for index, expected := range witness {
		schema, exists := scenario.Catalog().Schema(expected.Request.Kind)
		if !exists || schema.Ref() != expected.Schema {
			return cognition.Transition{}, 0, fmt.Errorf("%w: witness schema %d is not registered", ErrGeneration, index)
		}
		evidence, evidenceErr := registeredWitnessEvidence(schema, observations)
		if evidenceErr != nil {
			return cognition.Transition{}, 0, evidenceErr
		}
		action, registerErr := cognition.NewRegisteredAction(expected.ID, witnessActor, schema, expected.Request, evidence)
		if registerErr != nil {
			return cognition.Transition{}, 0, fmt.Errorf("%w: register witness action %d: %v", ErrGeneration, index, registerErr)
		}
		transition, err = environment.Apply(context.Background(), episode, transition.Current, action)
		if err != nil {
			return cognition.Transition{}, 0, fmt.Errorf("%w: apply witness action %d: %v", ErrGeneration, index, err)
		}
		if transition.Cost != expected.Cost {
			return cognition.Transition{}, 0, fmt.Errorf("%w: witness action %d cost is not exact", ErrGeneration, index)
		}
		total += transition.Cost
		observations = append(observations, transition.Observations...)
	}
	if !transition.Terminal {
		return cognition.Transition{}, 0, fmt.Errorf("%w: witness ended before the terminal goal", ErrGeneration)
	}
	return transition, total, nil
}
