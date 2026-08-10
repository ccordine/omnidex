package cognition

import (
	"encoding/json"
	"fmt"
)

func NewPredicate(name PredicateName, args []string) (Predicate, error) {
	predicate := Predicate{Name: name, Args: cloneSlice(args)}
	if err := predicate.Validate(); err != nil {
		return Predicate{}, err
	}
	return predicate, nil
}

func (predicate Predicate) Validate() error {
	if err := validateIdentity(string(predicate.Name), "predicate name"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPredicate, err)
	}
	if len(predicate.Args) > MaxPredicateArgs {
		return fmt.Errorf("%w: argument count exceeds %d", ErrInvalidPredicate, MaxPredicateArgs)
	}
	for index, argument := range predicate.Args {
		if err := validateExactText(argument, "predicate argument", MaxPredicateArgBytes); err != nil {
			return fmt.Errorf("%w: argument %d: %v", ErrInvalidPredicate, index, err)
		}
	}
	return nil
}

func (predicate Predicate) Clone() Predicate {
	predicate.Args = cloneSlice(predicate.Args)
	return predicate
}

func NewGoalExpression(all, any, not []Predicate) (GoalExpression, error) {
	goal := GoalExpression{
		All: clonePredicates(all),
		Any: clonePredicates(any),
		Not: clonePredicates(not),
	}
	if err := goal.Validate(); err != nil {
		return GoalExpression{}, err
	}
	return goal, nil
}

func (goal GoalExpression) Validate() error {
	total := len(goal.All) + len(goal.Any) + len(goal.Not)
	if total == 0 || total > MaxGoalPredicates {
		return fmt.Errorf("%w: predicate count must be between 1 and %d", ErrInvalidGoal, MaxGoalPredicates)
	}
	seen := make(map[string]string, total)
	for _, group := range []struct {
		name       string
		predicates []Predicate
	}{{"all", goal.All}, {"any", goal.Any}, {"not", goal.Not}} {
		for index, predicate := range group.predicates {
			if err := predicate.Validate(); err != nil {
				return fmt.Errorf("%w: %s predicate %d: %v", ErrInvalidGoal, group.name, index, err)
			}
			key := predicateKey(predicate)
			if previous, exists := seen[key]; exists {
				return fmt.Errorf("%w: predicate %q appears in both %s and %s", ErrInvalidGoal, predicate.Name, previous, group.name)
			}
			seen[key] = group.name
		}
	}
	return nil
}

func (goal GoalExpression) Clone() GoalExpression {
	return GoalExpression{
		All: clonePredicates(goal.All),
		Any: clonePredicates(goal.Any),
		Not: clonePredicates(goal.Not),
	}
}

func clonePredicates(values []Predicate) []Predicate {
	if values == nil {
		return nil
	}
	cloned := make([]Predicate, len(values))
	for index, value := range values {
		cloned[index] = value.Clone()
	}
	return cloned
}

func predicateKey(predicate Predicate) string {
	raw, err := json.Marshal(predicate)
	if err != nil {
		panic(fmt.Sprintf("marshal validated predicate identity: %v", err))
	}
	return string(raw)
}
