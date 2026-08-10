package cognition

import (
	"errors"
	"testing"
)

func TestCompletionAuthorityResolvesOnlyExplicitPredicateCapabilities(t *testing.T) {
	t.Parallel()
	check := testCompletionCheck("check.generic-predicate")
	authority, err := NewCompletionAuthority(
		check,
		[]PredicateName{"condition.second", "condition.subgoal"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := authority.Validate(); err != nil {
		t.Fatalf("validate authority: %v", err)
	}
	if got := authority.SupportedPredicates; len(got) != 2 || got[0] != "condition.second" {
		t.Fatalf("supported predicates are not canonical: %#v", got)
	}
	resolved, err := authority.Resolve(testGoalExpression(t, "condition.subgoal"))
	if err != nil {
		t.Fatalf("resolve registered predicate: %v", err)
	}
	if resolved != check {
		t.Fatalf("resolved check = %#v, want %#v", resolved, check)
	}
	if _, err := authority.Resolve(testGoalExpression(t, "condition.unsupported")); !errors.Is(err, ErrUnsupportedCompletionPredicate) {
		t.Fatalf("unsupported predicate error = %v", err)
	}

	tampered := authority.Clone()
	tampered.SupportedPredicates[0] = "condition.changed"
	if err := tampered.Validate(); !errors.Is(err, ErrInvalidCompletionAuthority) {
		t.Fatalf("tampered authority error = %v", err)
	}
}

func TestCompletionAuthorityRejectsDuplicateAndEmptyCapabilities(t *testing.T) {
	t.Parallel()
	check := testCompletionCheck("check.generic-predicate")
	if _, err := NewCompletionAuthority(check, nil); !errors.Is(err, ErrInvalidCompletionAuthority) {
		t.Fatalf("empty authority error = %v", err)
	}
	if _, err := NewCompletionAuthority(check, []PredicateName{"condition.a", "condition.a"}); !errors.Is(err, ErrInvalidCompletionAuthority) {
		t.Fatalf("duplicate authority error = %v", err)
	}
}
