package labyrinth

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

// EvaluateGoal compares a registered desired predicate with exact in-memory
// world truth and returns only the boolean result.
func (environment *Environment) EvaluateGoal(
	ctx context.Context,
	episode cognition.EpisodeRef,
	revision cognition.WorldRevision,
	desired cognition.GoalExpression,
) (bool, error) {
	if ctx == nil || environment == nil {
		return false, fmt.Errorf("%w: completion environment is unavailable", cognition.ErrInvalidRevision)
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := episode.Validate(); err != nil {
		return false, err
	}
	if err := revision.Validate(); err != nil {
		return false, err
	}
	if err := desired.Validate(); err != nil {
		return false, err
	}
	environment.mu.Lock()
	defer environment.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !environment.started {
		return false, ErrNotStarted
	}
	if episode != environment.episode || revision != environment.current {
		return false, fmt.Errorf("%w: completion revision is not current", cognition.ErrInvalidRevision)
	}
	predicates := make([]cognition.Predicate, 0, len(desired.All)+len(desired.Any)+len(desired.Not))
	predicates = append(predicates, desired.All...)
	predicates = append(predicates, desired.Any...)
	predicates = append(predicates, desired.Not...)
	if err := validateGroundPredicates(
		predicates, environment.entities, environment.predicates, "completion predicate",
	); err != nil {
		return false, err
	}
	return goalSatisfied(desired, environment.facts), nil
}
