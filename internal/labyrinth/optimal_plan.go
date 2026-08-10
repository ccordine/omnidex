package labyrinth

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func buildSolverOptimalPlan(
	scenario Scenario,
	result SolverResult,
) ([]WitnessAction, error) {
	if !result.Optimal || result.Cost <= 0 || len(result.Actions) == 0 ||
		len(result.Actions) > MaxSolutionDepth {
		return nil, fmt.Errorf("%w: solver result cannot authorize an optimal plan", ErrGeneration)
	}
	plan := make([]WitnessAction, len(result.Actions))
	for index, request := range result.Actions {
		schema, exists := scenario.Catalog().Schema(request.Kind)
		action, defined := actionDefinitionForKind(scenario.definition, request.Kind)
		if !exists || !defined || schema.Ref() != action.Schema.Ref() {
			return nil, fmt.Errorf(
				"%w: solver optimal action %d lacks its exact schema", ErrGeneration, index,
			)
		}
		plan[index] = WitnessAction{
			ID:     cognition.ActionID(fmt.Sprintf("optimal-action-%03d", index)),
			Schema: schema.Ref(), Request: request.Clone(), Cost: action.Cost,
		}
	}
	transition, cost, err := verifyScenarioWitness(scenario, plan)
	if err != nil || !transition.Terminal || cost != result.Cost {
		return nil, fmt.Errorf(
			"%w: solver optimal plan failed exact replay: terminal=%t cost=%d want=%d: %v",
			ErrGeneration, transition.Terminal, cost, result.Cost, err,
		)
	}
	return plan, nil
}
