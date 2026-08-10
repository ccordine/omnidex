package labyrinth

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/gryph/omnidex/internal/cognition"
)

func (environment *Environment) commitCandidate(
	ctx context.Context,
	definition ActionDefinition,
	expected cognition.WorldRevision,
	action cognition.RegisteredAction,
	requestSHA256 string,
) (cognition.Transition, error) {
	candidate, changed, err := applyActionDefinition(
		definition, action, environment.entities, environment.predicates, environment.facts,
	)
	if err != nil {
		code, message := cognition.ActionFailureInvalidAction, "The registered action arguments are invalid."
		cause := error(cognition.ErrInvalidAction)
		if errors.Is(err, ErrPrecondition) {
			code, message, cause = cognition.ActionFailurePreconditionFailed,
				"A registered action precondition is not satisfied.", ErrPrecondition
		}
		return cognition.Transition{}, environment.bindFailure(code, action, expected, requestSHA256, message, cause)
	}
	if environment.totalCost > math.MaxInt64-int64(definition.Cost) {
		return cognition.Transition{}, environment.bindFailure(
			cognition.ActionFailureBudget, action, expected, requestSHA256,
			"The cumulative transition cost cannot be represented.", ErrTransitionLimit,
		)
	}
	totalCost := environment.totalCost + int64(definition.Cost)
	if len(candidate) > MaxWorldFacts {
		return cognition.Transition{}, environment.bindFailure(
			cognition.ActionFailureBudget, action, expected, requestSHA256,
			"The bounded symbolic world cannot represent this transition.", ErrWorldLimit,
		)
	}
	terminal := goalSatisfied(environment.scenario.definition.goal, candidate)
	preparation := environment.surfaceState.clone()
	if environment.surface != nil {
		preparation, err = environment.surface.Apply(
			ctx, environment.scenario, environment.surfaceState.clone(), action,
		)
		if err != nil {
			if contextErr := ctx.Err(); contextErr != nil {
				return cognition.Transition{}, contextErr
			}
			return cognition.Transition{}, environment.bindSurfaceFailure(action, expected, requestSHA256, err)
		}
		if err := preparation.Validate(); err != nil {
			return cognition.Transition{}, environment.bindSurfaceFailure(action, expected, requestSHA256, err)
		}
		if preparation.Version != environment.surfaceState.Version ||
			preparation.Operation != string(action.Request.Kind) {
			return cognition.Transition{}, environment.bindSurfaceFailure(
				action, expected, requestSHA256,
				fmt.Errorf("%w: surface version or operation changed authority", ErrSurfaceOperation),
			)
		}
	}
	revision, err := revisionFor(
		environment.scenario.ref, environment.scenario.definitionSHA256, environment.episode,
		expected.Number+1, expected.SHA256,
		action.ID, requestSHA256, preparation.StateSHA256, candidate, totalCost, terminal,
	)
	if err != nil {
		if errors.Is(err, ErrWorldLimit) {
			return cognition.Transition{}, environment.bindFailure(
				cognition.ActionFailureBudget, action, expected, requestSHA256,
				"The bounded symbolic world cannot represent this transition.", ErrWorldLimit,
			)
		}
		return cognition.Transition{}, err
	}
	var observation cognition.Observation
	if environment.surface == nil {
		observation, err = buildObservation(
			action.ID, revision, candidate, terminal, environment.entities, environment.predicates,
			environment.scenario.descriptor.Records, environment.scenario.artifactCorpus, &action.Request,
		)
	} else {
		observation, err = buildSurfaceObservation(
			action.ID, revision, candidate, terminal, environment.entities, environment.predicates, preparation,
		)
	}
	if err != nil {
		return cognition.Transition{}, environment.bindFailure(
			cognition.ActionFailureBudget, action, expected, requestSHA256,
			"The bounded public observation cannot represent this transition.", ErrObservationLimit,
		)
	}
	effect, err := buildPublicEffect(action.ID, revision, changed)
	if err != nil {
		return cognition.Transition{}, err
	}
	previous := expected
	transition := cognition.Transition{
		ActionID: action.ID, Previous: &previous, Current: revision,
		Observations: []cognition.Observation{observation}, Effects: []cognition.Effect{effect},
		Cost: definition.Cost, Terminal: terminal, PublicOutcome: PublicOutcomeApplied,
	}
	if terminal {
		transition.PublicOutcome = PublicOutcomeGoalSatisfied
	}
	if err := transition.ValidateApply(environment.episode, expected, action); err != nil {
		return cognition.Transition{}, err
	}
	environment.current = revision
	environment.facts = candidate
	environment.totalCost = totalCost
	environment.terminal = terminal
	environment.surfaceState = preparation.clone()
	environment.observations[observation.ID] = observation
	stored := transition.Clone()
	environment.receipts[action.ID] = actionReceipt{
		requestSHA256: requestSHA256, expected: expected, transition: &stored,
	}
	return transition.Clone(), nil
}
