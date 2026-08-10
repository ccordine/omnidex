package labyrinth

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestCompletionCheckIdentifiesOneGenericEvaluatorAcrossDesiredPredicates(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	goal := world.kernel.Goal()
	check, err := NewCompletionCheck()
	if err != nil {
		t.Fatal(err)
	}
	if err := check.Validate(); err != nil {
		t.Fatal(err)
	}
	changed := goal.Clone()
	changed.All[0].Args[0] = hiddenStateCanary
	changedCheck, err := NewCompletionCheck()
	if err != nil {
		t.Fatal(err)
	}
	if changedCheck != check {
		t.Fatal("generic completion evaluator identity changed with the desired predicate")
	}
	if world.kernel.Goal().All[0].Args[0] == hiddenStateCanary {
		t.Fatal("Scenario.Goal returned mutable private state")
	}
}

func TestCompletionAuthorityExposesOnlyPublicAndExactGoalPredicateNames(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	authority, err := NewCompletionAuthority(world.kernel)
	if err != nil {
		t.Fatal(err)
	}
	if err := authority.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(authority.SupportedPredicates) != 2 ||
		authority.SupportedPredicates[0] != "state.active" ||
		authority.SupportedPredicates[1] != "state.complete" {
		t.Fatalf("completion predicates = %#v", authority.SupportedPredicates)
	}
	if resolved, err := authority.Resolve(world.kernel.Goal()); err != nil || resolved != authority.Check {
		t.Fatalf("resolve exact public goal = %#v, error = %v", resolved, err)
	}
	hidden, err := cognition.NewPredicate("state.protected", []string{hiddenStateCanary})
	if err != nil {
		t.Fatal(err)
	}
	hiddenGoal, err := cognition.NewGoalExpression([]cognition.Predicate{hidden}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authority.Resolve(hiddenGoal); !errors.Is(err, cognition.ErrUnsupportedCompletionPredicate) {
		t.Fatalf("hidden predicate error = %v", err)
	}
}

func TestEnvironmentEvaluatesGoalAtExactEpisodeRevision(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	environment := world.newEnvironment(t)
	started, err := environment.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatal(err)
	}
	satisfied, err := environment.EvaluateGoal(
		context.Background(), world.episode, started.Current, world.kernel.Goal(),
	)
	if err != nil || satisfied {
		t.Fatalf("initial completion = %t, error = %v", satisfied, err)
	}
	enabled, err := environment.Apply(
		context.Background(), world.episode, started.Current,
		world.action(t, "completion-enable", "enable", "unit-public"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := environment.EvaluateGoal(
		context.Background(), world.episode, started.Current, world.kernel.Goal(),
	); err == nil {
		t.Fatal("completion evaluator accepted a stale world revision")
	}
	finished, err := environment.Apply(
		context.Background(), world.episode, enabled.Current,
		world.action(t, "completion-finish", "finish", "unit-public"),
	)
	if err != nil {
		t.Fatal(err)
	}
	satisfied, err = environment.EvaluateGoal(
		context.Background(), world.episode, finished.Current, world.kernel.Goal(),
	)
	if err != nil || !satisfied {
		t.Fatalf("terminal completion = %t, error = %v", satisfied, err)
	}
}
