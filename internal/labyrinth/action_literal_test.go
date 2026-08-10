package labyrinth

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestLiteralActionParameterAcceptsRuntimeTextOutsideSolverRepresentatives(t *testing.T) {
	t.Parallel()
	definition, schema := literalActionDefinition(t)
	action, err := cognition.NewRegisteredAction(
		"literal-runtime-action", solverActor, schema,
		cognition.ActionRequest{Kind: schema.Kind, Arguments: []cognition.ActionArgument{
			{Name: "query", Value: "artifact-0999999"},
		}}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	entities, _, err := validateEntities(definition.entities)
	if err != nil {
		t.Fatal(err)
	}
	predicates, err := validatePredicateSchemas(definition.predicateSchemas, entityKinds(entities))
	if err != nil {
		t.Fatal(err)
	}
	if _, changed, err := applyActionDefinition(
		definition.actions[0], action, entities, predicates, newFactSet(definition.initialFacts),
	); err != nil || !changed {
		t.Fatalf("runtime literal changed=%t error=%v", changed, err)
	}
}

func TestSolverGroundsOnlySealedLiteralRepresentatives(t *testing.T) {
	t.Parallel()
	definition, _ := literalActionDefinition(t)
	requests, err := groundLegalRequests(definition, newFactSet(definition.initialFacts), 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 || actionArgument(requests[0], "query") != "registered-query" {
		t.Fatalf("grounded requests = %#v", requests)
	}
}

func TestDefinitionRejectsInvalidLiteralParameterAuthority(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*ActionDefinition){
		"unknown parameter": func(action *ActionDefinition) {
			action.LiteralParameters[0].Name = "missing"
		},
		"duplicate parameter": func(action *ActionDefinition) {
			action.LiteralParameters = append(action.LiteralParameters, action.LiteralParameters[0])
		},
		"empty representatives": func(action *ActionDefinition) {
			action.LiteralParameters[0].SolverValues = []string{}
		},
		"pattern bound literal": func(action *ActionDefinition) {
			action.Preconditions[0].Predicate.Arguments[0] = PatternArgument{Parameter: "query"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			definition, _ := literalActionDefinition(t)
			mutate(&definition.actions[0])
			if err := definition.reseal(); !errors.Is(err, ErrInvalidDefinition) {
				t.Fatalf("error=%v, want ErrInvalidDefinition", err)
			}
		})
	}
}

func literalActionDefinition(t *testing.T) (Definition, cognition.ActionSchema) {
	t.Helper()
	schema := mustActionSchema(t, "literal.search.v1", "search", []cognition.ActionParameterSpec{
		{Name: "query", Required: true, MaxBytes: 128},
	})
	catalog, err := cognition.NewActionCatalog("literal.catalog.v1", "1.0.0", []cognition.ActionSchema{schema})
	if err != nil {
		t.Fatal(err)
	}
	ready := mustPredicate(t, "state.ready", "unit-a")
	done := mustPredicate(t, "state.done", "unit-a")
	goal := mustGoal(t, done)
	definition, err := NewDefinition(
		catalog,
		[]Entity{{ID: "unit-a", Kind: "unit"}},
		[]PredicateSchema{
			{Name: "state.ready", ArgumentKinds: []EntityKind{"unit"}},
			{Name: "state.done", ArgumentKinds: []EntityKind{"unit"}},
		},
		[]cognition.Predicate{ready},
		[]ActionDefinition{{
			Schema: schema,
			LiteralParameters: []LiteralParameter{{
				Name: "query", SolverValues: []string{"registered-query"},
			}},
			Preconditions: []Condition{{Mode: ConditionPresent, Predicate: PredicatePattern{
				Name: "state.ready", Arguments: []PatternArgument{{Entity: "unit-a"}},
			}}},
			Effects: []Effect{{Mode: EffectAssert, Predicate: PredicatePattern{
				Name: "state.done", Arguments: []PatternArgument{{Entity: "unit-a"}},
			}}},
			Cost: 1,
		}},
		goal,
	)
	if err != nil {
		t.Fatal(err)
	}
	return definition, schema
}
