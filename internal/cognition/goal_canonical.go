package cognition

import (
	"encoding/json"
	"fmt"
	"sort"
)

func canonicalGoal(goal GoalExpression) (GoalExpression, error) {
	if err := goal.Validate(); err != nil {
		return GoalExpression{}, err
	}
	return GoalExpression{
		All: canonicalPredicates(goal.All),
		Any: canonicalPredicates(goal.Any),
		Not: canonicalPredicates(goal.Not),
	}, nil
}

func canonicalPredicates(values []Predicate) []Predicate {
	if len(values) == 0 {
		return nil
	}
	result := clonePredicates(values)
	sort.Slice(result, func(left, right int) bool {
		return predicateKey(result[left]) < predicateKey(result[right])
	})
	return result
}

func goalIdentity(goal GoalExpression) (string, error) {
	canonical, err := canonicalGoal(goal)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("marshal canonical cognition goal: %w", err)
	}
	return contentSHA256(string(raw)), nil
}
