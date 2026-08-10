package labyrinth

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

const hiddenStateCanary = "hidden-state-canary-7f6e"

type testWorld struct {
	scenario cognition.ScenarioRef
	kernel   Scenario
	episode  cognition.EpisodeRef
	actor    cognition.AttemptRef
	schemas  map[cognition.ActionKind]cognition.ActionSchema
}

func newTestWorld(t *testing.T) testWorld {
	t.Helper()
	parameter := []cognition.ActionParameterSpec{{Name: "target", Required: true, MaxBytes: 128}}
	enable := mustActionSchema(t, "symbolic.enable.v1", "enable", parameter)
	disable := mustActionSchema(t, "symbolic.disable.v1", "disable", parameter)
	finish := mustActionSchema(t, "symbolic.finish.v1", "finish", parameter)
	catalog, err := cognition.NewActionCatalog(
		"symbolic.catalog.v1",
		"1.0.0",
		[]cognition.ActionSchema{finish, disable, enable},
	)
	if err != nil {
		t.Fatalf("new action catalog: %v", err)
	}
	publicUnit := Entity{ID: "unit-public", Kind: "unit", Public: true}
	hiddenUnit := Entity{ID: hiddenStateCanary, Kind: "unit"}
	active := PredicateSchema{Name: "state.active", ArgumentKinds: []EntityKind{"unit"}, Public: true}
	protected := PredicateSchema{Name: "state.protected", ArgumentKinds: []EntityKind{"unit"}}
	complete := PredicateSchema{Name: "state.complete", ArgumentKinds: []EntityKind{"unit"}}
	target := PatternArgument{Parameter: "target"}
	actions := []ActionDefinition{
		{
			Schema: enable,
			Preconditions: []Condition{
				{Mode: ConditionAbsent, Predicate: PredicatePattern{Name: active.Name, Arguments: []PatternArgument{target}}},
			},
			Effects: []Effect{
				{Mode: EffectAssert, Predicate: PredicatePattern{Name: active.Name, Arguments: []PatternArgument{target}}},
			},
			Cost: 2,
		},
		{
			Schema: disable,
			Preconditions: []Condition{
				{Mode: ConditionPresent, Predicate: PredicatePattern{Name: active.Name, Arguments: []PatternArgument{target}}},
			},
			Effects: []Effect{
				{Mode: EffectRetract, Predicate: PredicatePattern{Name: active.Name, Arguments: []PatternArgument{target}}},
			},
			Cost: 3,
		},
		{
			Schema: finish,
			Preconditions: []Condition{
				{Mode: ConditionPresent, Predicate: PredicatePattern{Name: active.Name, Arguments: []PatternArgument{target}}},
			},
			Effects: []Effect{
				{Mode: EffectAssert, Predicate: PredicatePattern{Name: complete.Name, Arguments: []PatternArgument{target}}},
			},
			Cost: 5,
		},
	}
	initial, err := cognition.NewPredicate(protected.Name, []string{string(hiddenUnit.ID)})
	if err != nil {
		t.Fatal(err)
	}
	goalPredicate, err := cognition.NewPredicate(complete.Name, []string{string(publicUnit.ID)})
	if err != nil {
		t.Fatal(err)
	}
	goal, err := cognition.NewGoalExpression([]cognition.Predicate{goalPredicate}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := NewDefinition(
		catalog,
		[]Entity{hiddenUnit, publicUnit},
		[]PredicateSchema{complete, protected, active},
		[]cognition.Predicate{initial},
		actions,
		goal,
	)
	if err != nil {
		t.Fatalf("new definition: %v", err)
	}
	kernel, err := NewScenario("symbolic-scenario-v1", definition, testPublicDescriptor())
	if err != nil {
		t.Fatalf("new scenario: %v", err)
	}
	return testWorld{
		scenario: kernel.Ref(),
		kernel:   kernel,
		episode:  cognition.EpisodeRef{ID: "episode-test-1"},
		actor: cognition.AttemptRef{
			JobID: 41, Generation: 2, StepID: 7, Attempt: 1, WorkerID: "worker-test-1",
		},
		schemas: map[cognition.ActionKind]cognition.ActionSchema{
			enable.Kind: enable, disable.Kind: disable, finish.Kind: finish,
		},
	}
}

func testPublicDescriptor() PublicDescriptor {
	return PublicDescriptor{
		Suite:          SuiteRetrieve,
		FormatVersion:  "test-public.v1",
		SurfaceVersion: "test-symbolic.v1",
		GrammarVersion: "test-grammar.v1",
		Goal:           "Reach the registered terminal predicate.",
		Difficulty: PublicDifficulty{
			WorldSize: 25, EvidenceArtifacts: 3, DecisionDepth: 4,
			BranchingFactor: 1, DependencyCount: 2,
		},
	}
}

func mustActionSchema(
	t *testing.T,
	id cognition.ActionSchemaID,
	kind cognition.ActionKind,
	parameters []cognition.ActionParameterSpec,
) cognition.ActionSchema {
	t.Helper()
	schema, err := cognition.NewActionSchema(id, "1.0.0", kind, parameters, cognition.EvidenceOptional)
	if err != nil {
		t.Fatalf("new action schema: %v", err)
	}
	return schema
}

func (world testWorld) newEnvironment(t *testing.T) *Environment {
	t.Helper()
	environment, err := NewEnvironment(world.kernel, world.episode, func(_ context.Context, actor cognition.AttemptRef) error {
		if actor != world.actor {
			return cognition.ErrAuthorityDenied
		}
		return nil
	})
	if err != nil {
		t.Fatalf("new environment: %v", err)
	}
	return environment
}

func (world testWorld) action(
	t *testing.T,
	id cognition.ActionID,
	kind cognition.ActionKind,
	target EntityID,
) cognition.RegisteredAction {
	t.Helper()
	action, err := cognition.NewRegisteredAction(
		id,
		world.actor,
		world.schemas[kind],
		cognition.ActionRequest{
			Kind:      kind,
			Arguments: []cognition.ActionArgument{{Name: "target", Value: string(target)}},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("new registered action: %v", err)
	}
	return action
}
