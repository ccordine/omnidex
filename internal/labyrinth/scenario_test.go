package labyrinth

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestDefinitionIsCanonicalContentAddressedAndDefensivelyCopied(t *testing.T) {
	t.Parallel()
	forward, inputs := orderedDefinition(t, false)
	reversed, _ := orderedDefinition(t, true)
	if forward.SHA256() != reversed.SHA256() {
		t.Fatalf("equivalent definitions have different hashes: %s != %s", forward.SHA256(), reversed.SHA256())
	}
	if !reflect.DeepEqual(forward.Catalog(), reversed.Catalog()) {
		t.Fatalf("equivalent catalogs differ: %#v != %#v", forward.Catalog(), reversed.Catalog())
	}

	sealedHash := forward.SHA256()
	inputs.entities[0].ID = "caller-mutated"
	inputs.predicates[0].ArgumentKinds[0] = "caller-mutated"
	inputs.initial[0].Args[0] = "caller-mutated"
	inputs.actions[0].Cost++
	inputs.goal.All[0].Args[0] = "caller-mutated"
	if forward.SHA256() != sealedHash {
		t.Fatalf("definition hash changed through caller-owned input: %s != %s", forward.SHA256(), sealedHash)
	}
	if err := forward.Validate(); err != nil {
		t.Fatalf("sealed definition became invalid: %v", err)
	}
}

func TestInitialRevisionBindsPrivateDefinitionWithoutExposingIt(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	changed := world.kernel.definition.clone()
	changed.actions[0].Cost++
	if err := changed.reseal(); err != nil {
		t.Fatalf("reseal private rule variant: %v", err)
	}
	variant, err := NewScenario(world.scenario.ID, changed, world.kernel.descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if variant.Ref() != world.scenario {
		t.Fatalf("private rule altered public identity: %#v != %#v", variant.Ref(), world.scenario)
	}
	authorize := func(context.Context, cognition.AttemptRef) error { return nil }
	left, err := NewEnvironment(world.kernel, world.episode, authorize)
	if err != nil {
		t.Fatal(err)
	}
	right, err := NewEnvironment(variant, world.episode, authorize)
	if err != nil {
		t.Fatal(err)
	}
	leftStart, err := left.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatal(err)
	}
	rightStart, err := right.Start(context.Background(), variant.Ref())
	if err != nil {
		t.Fatal(err)
	}
	if leftStart.Current.SHA256 == rightStart.Current.SHA256 {
		t.Fatal("world revision did not bind the complete private definition")
	}
}

func TestScenarioRefBindsExactDefinition(t *testing.T) {
	t.Parallel()
	definition, _ := orderedDefinition(t, false)
	scenario, err := NewScenario("symbolic-content-v1", definition, testPublicDescriptor())
	if err != nil {
		t.Fatalf("new scenario: %v", err)
	}
	if scenario.Ref().SHA256 == definition.SHA256() {
		t.Fatal("public scenario hash was derived from the complete private definition")
	}
	if err := scenario.Validate(); err != nil {
		t.Fatalf("validate scenario: %v", err)
	}
}

func TestPublicScenarioIdentityDoesNotCommitHiddenWorldState(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	changed := world.kernel.definition.clone()
	for index := range changed.entities {
		if changed.entities[index].ID == EntityID(hiddenStateCanary) {
			changed.entities[index].ID = "different-hidden-state"
		}
	}
	for index := range changed.initialFacts {
		for argument := range changed.initialFacts[index].Args {
			if changed.initialFacts[index].Args[argument] == hiddenStateCanary {
				changed.initialFacts[index].Args[argument] = "different-hidden-state"
			}
		}
	}
	if err := changed.reseal(); err != nil {
		t.Fatalf("reseal private variant: %v", err)
	}
	if changed.SHA256() == world.kernel.definition.SHA256() {
		t.Fatal("private definition change did not alter its private content address")
	}
	variant, err := NewScenario(world.scenario.ID, changed, world.kernel.descriptor)
	if err != nil {
		t.Fatalf("new private variant: %v", err)
	}
	if variant.Ref() != world.scenario {
		t.Fatalf("hidden state altered public scenario identity: %#v != %#v", variant.Ref(), world.scenario)
	}
	tampered := world.kernel
	tampered.definition = changed
	if err := tampered.Validate(); !errors.Is(err, cognition.ErrInvalidScenario) {
		t.Fatalf("replaced private definition error = %v, want ErrInvalidScenario", err)
	}
	if _, err := NewEnvironment(
		tampered, world.episode, func(context.Context, cognition.AttemptRef) error { return nil },
	); !errors.Is(err, cognition.ErrInvalidScenario) {
		t.Fatalf("environment accepted replaced private definition: %v", err)
	}
}

func TestDefinitionRejectsInvalidBindingsAndUnregisteredFacts(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	definition := world.kernel.definition.clone()
	definition.entities = append(definition.entities, Entity{ID: "unit-public", Kind: "unit"})
	if err := definition.reseal(); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("duplicate entity error = %v, want ErrInvalidDefinition", err)
	}

	definition = world.kernel.definition.clone()
	definition.initialFacts[0].Args[0] = "missing-entity"
	if err := definition.reseal(); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("unknown entity error = %v, want ErrInvalidDefinition", err)
	}

	definition = world.kernel.definition.clone()
	definition.actions[0].Effects[0].Predicate.Arguments[0] = PatternArgument{
		Parameter: "target", Entity: "unit-public",
	}
	if err := definition.reseal(); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("ambiguous binding error = %v, want ErrInvalidDefinition", err)
	}

	definition = world.kernel.definition.clone()
	definition.actions[0].Cost = 0
	if err := definition.reseal(); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("zero cost error = %v, want ErrInvalidDefinition", err)
	}
}

type definitionInputs struct {
	entities   []Entity
	predicates []PredicateSchema
	initial    []cognition.Predicate
	actions    []ActionDefinition
	goal       cognition.GoalExpression
}

func orderedDefinition(t *testing.T, reverse bool) (Definition, definitionInputs) {
	t.Helper()
	parameter := []cognition.ActionParameterSpec{{Name: "target", Required: true, MaxBytes: 64}}
	set := mustActionSchema(t, "symbolic.set.v1", "set", parameter)
	clear := mustActionSchema(t, "symbolic.clear.v1", "clear", parameter)
	catalog, err := cognition.NewActionCatalog("symbolic.ordered.v1", "1.0.0", []cognition.ActionSchema{set, clear})
	if err != nil {
		t.Fatal(err)
	}
	entities := []Entity{{ID: "entity-a", Kind: "unit", Public: true}, {ID: "entity-b", Kind: "unit"}}
	predicates := []PredicateSchema{
		{Name: "state.flag", ArgumentKinds: []EntityKind{"unit"}, Public: true},
		{Name: "state.marker", ArgumentKinds: []EntityKind{"unit"}},
	}
	flagA := mustPredicate(t, "state.flag", "entity-a")
	markerB := mustPredicate(t, "state.marker", "entity-b")
	initial := []cognition.Predicate{markerB, flagA}
	argument := PatternArgument{Parameter: "target"}
	actions := []ActionDefinition{
		{Schema: set, Effects: []Effect{{Mode: EffectAssert, Predicate: PredicatePattern{
			Name: "state.flag", Arguments: []PatternArgument{argument},
		}}}, Cost: 1},
		{Schema: clear, Effects: []Effect{{Mode: EffectRetract, Predicate: PredicatePattern{
			Name: "state.flag", Arguments: []PatternArgument{argument},
		}}}, Cost: 1},
	}
	goal, err := cognition.NewGoalExpression([]cognition.Predicate{flagA, markerB}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reverse {
		reverseEntities(entities)
		reversePredicateSchemas(predicates)
		reversePredicates(initial)
		reverseActions(actions)
		reversePredicates(goal.All)
	}
	inputs := definitionInputs{entities, predicates, initial, actions, goal}
	definition, err := NewDefinition(catalog, entities, predicates, initial, actions, goal)
	if err != nil {
		t.Fatalf("new ordered definition: %v", err)
	}
	return definition, inputs
}

func mustPredicate(t *testing.T, name cognition.PredicateName, args ...string) cognition.Predicate {
	t.Helper()
	predicate, err := cognition.NewPredicate(name, args)
	if err != nil {
		t.Fatalf("new predicate: %v", err)
	}
	return predicate
}

func reverseEntities(values []Entity) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reversePredicateSchemas(values []PredicateSchema) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reversePredicates(values []cognition.Predicate) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseActions(values []ActionDefinition) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}
