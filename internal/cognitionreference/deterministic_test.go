package cognitionreference

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestMachineDerivesDeterministicPrerequisiteClosureWithoutInference(t *testing.T) {
	t.Parallel()

	code := FactDefinition{ID: "terminal.unlock_code", Kind: FactText, MaxBytes: 32}
	unlocked := PredicateDefinition{ID: "terminal.unlocked"}
	inspect := Operation{
		ID:       "inspect_hint",
		Provides: []FactID{code.ID},
		Execute: func(_ context.Context, input OperationInput) (Transition, error) {
			return input.Transition(
				[]Fact{{ID: code.ID, Text: "amber"}}, nil, "authoritative hint observed",
			)
		},
	}
	unlock := Operation{
		ID:       "unlock",
		Requires: []FactID{code.ID},
		Achieves: []PredicateID{unlocked.ID},
		Bindings: []Binding{{Argument: "code", Fact: code.ID}},
		Execute: func(_ context.Context, input OperationInput) (Transition, error) {
			got, found := input.Argument("code")
			if !found || got != "amber" {
				t.Fatalf("unlock received bound code %q, want amber", got)
			}
			return input.Transition(nil, []PredicateID{unlocked.ID}, "terminal unlocked")
		},
	}
	catalog, err := NewCatalog(
		[]FactDefinition{code}, []PredicateDefinition{unlocked}, []Operation{unlock, inspect},
	)
	if err != nil {
		t.Fatal(err)
	}
	objective := Objective{ID: "objective.unlock-terminal", Desired: unlocked.ID}
	machine, err := NewMachine(catalog, objective, Limits{MaxSteps: 8, MaxDepth: 8})
	if err != nil {
		t.Fatal(err)
	}

	result, err := machine.Run(t.Context(), mustState(t, machine.catalog, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	wantTrace := []TraceStep{
		{
			Sequence: 1, Operation: inspect.ID, Arguments: []Argument{},
			Facts:      []Fact{{ID: code.ID, Text: "amber"}},
			Predicates: []PredicateID{}, Outcome: "authoritative hint observed",
		},
		{
			Sequence: 2, Operation: unlock.ID,
			Arguments: []Argument{{Name: "code", Value: "amber"}},
			Facts:     []Fact{}, Predicates: []PredicateID{unlocked.ID}, Outcome: "terminal unlocked",
		},
	}
	if !reflect.DeepEqual(result.Trace, wantTrace) {
		t.Fatalf("trace = %#v, want %#v", result.Trace, wantTrace)
	}
	if !result.Complete || !result.Final.HasPredicate(unlocked.ID) {
		t.Fatalf("result did not reach code-owned completion: %#v", result)
	}
	if result.SelectorCalls != 0 || result.InferenceCalls != 0 {
		t.Fatalf(
			"selector/inference calls = %d/%d, want exactly 0/0",
			result.SelectorCalls, result.InferenceCalls,
		)
	}
}

func TestMachineFailsLoudlyWhenDeterministicClosureCannotProceed(t *testing.T) {
	t.Parallel()

	t.Run("no producer", func(t *testing.T) {
		t.Parallel()
		missing := FactDefinition{ID: "missing.fact", Kind: FactText, MaxBytes: 8}
		goal := PredicateDefinition{ID: "goal.complete"}
		finish := testOperation("finish", []FactID{missing.ID}, nil, []PredicateID{goal.ID})
		machine := testMachine(t, []FactDefinition{missing}, []PredicateDefinition{goal}, []Operation{finish}, goal.ID, Limits{8, 8})
		if _, err := machine.Run(t.Context(), mustState(t, machine.catalog, nil, nil)); !errors.Is(err, ErrNoProducer) {
			t.Fatalf("error = %v, want ErrNoProducer", err)
		}
	})

	t.Run("ambiguous producer", func(t *testing.T) {
		t.Parallel()
		choice := FactDefinition{ID: "route.choice", Kind: FactText, MaxBytes: 8}
		goal := PredicateDefinition{ID: "goal.complete"}
		executions := 0
		first := countedTestOperation("produce.first", nil, []FactID{choice.ID}, nil, &executions)
		second := countedTestOperation("produce.second", nil, []FactID{choice.ID}, nil, &executions)
		finish := testOperation("finish", []FactID{choice.ID}, nil, []PredicateID{goal.ID})
		machine := testMachine(t, []FactDefinition{choice}, []PredicateDefinition{goal}, []Operation{first, second, finish}, goal.ID, Limits{8, 8})
		if _, err := machine.Run(t.Context(), mustState(t, machine.catalog, nil, nil)); !errors.Is(err, ErrAmbiguousProducer) {
			t.Fatalf("error = %v, want ErrAmbiguousProducer", err)
		}
		if executions != 0 {
			t.Fatalf("ambiguous closure executed %d candidates before selection", executions)
		}
	})

	t.Run("causal cycle", func(t *testing.T) {
		t.Parallel()
		first := FactDefinition{ID: "cycle.first", Kind: FactText, MaxBytes: 8}
		second := FactDefinition{ID: "cycle.second", Kind: FactText, MaxBytes: 8}
		goal := PredicateDefinition{ID: "goal.complete"}
		produceFirst := testOperation("produce.first", []FactID{second.ID}, []FactID{first.ID}, nil)
		produceSecond := testOperation("produce.second", []FactID{first.ID}, []FactID{second.ID}, nil)
		finish := testOperation("finish", []FactID{first.ID}, nil, []PredicateID{goal.ID})
		machine := testMachine(t, []FactDefinition{first, second}, []PredicateDefinition{goal}, []Operation{produceFirst, produceSecond, finish}, goal.ID, Limits{8, 8})
		if _, err := machine.Run(t.Context(), mustState(t, machine.catalog, nil, nil)); !errors.Is(err, ErrCausalCycle) {
			t.Fatalf("error = %v, want ErrCausalCycle", err)
		}
	})

	t.Run("depth bound", func(t *testing.T) {
		t.Parallel()
		fact := FactDefinition{ID: "deep.fact", Kind: FactText, MaxBytes: 8}
		goal := PredicateDefinition{ID: "goal.complete"}
		produce := testOperation("produce", nil, []FactID{fact.ID}, nil)
		finish := testOperation("finish", []FactID{fact.ID}, nil, []PredicateID{goal.ID})
		machine := testMachine(t, []FactDefinition{fact}, []PredicateDefinition{goal}, []Operation{produce, finish}, goal.ID, Limits{8, 1})
		if _, err := machine.Run(t.Context(), mustState(t, machine.catalog, nil, nil)); !errors.Is(err, ErrClosureBound) {
			t.Fatalf("error = %v, want ErrClosureBound", err)
		}
	})

	t.Run("step bound", func(t *testing.T) {
		t.Parallel()
		fact := FactDefinition{ID: "step.fact", Kind: FactText, MaxBytes: 8}
		goal := PredicateDefinition{ID: "goal.complete"}
		produce := testOperation("produce", nil, []FactID{fact.ID}, nil)
		finish := testOperation("finish", []FactID{fact.ID}, nil, []PredicateID{goal.ID})
		machine := testMachine(t, []FactDefinition{fact}, []PredicateDefinition{goal}, []Operation{produce, finish}, goal.ID, Limits{1, 8})
		if _, err := machine.Run(t.Context(), mustState(t, machine.catalog, nil, nil)); !errors.Is(err, ErrClosureBound) {
			t.Fatalf("error = %v, want ErrClosureBound", err)
		}
	})
}

func TestMachineRejectsExecutorThatLiesAboutDeclaredEffects(t *testing.T) {
	t.Parallel()
	for name, transitionPredicates := range map[string][]PredicateID{
		"missing declared effect": {},
		"undeclared effect":       {"goal.complete", "goal.fabricated"},
	} {
		name, transitionPredicates := name, transitionPredicates
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			goal := PredicateDefinition{ID: "goal.complete"}
			fabricated := PredicateDefinition{ID: "goal.fabricated"}
			lying := Operation{
				ID: "lying.finish", Achieves: []PredicateID{goal.ID},
				Execute: func(_ context.Context, input OperationInput) (Transition, error) {
					return input.Transition(nil, transitionPredicates, "claimed an inexact effect set")
				},
			}
			machine := testMachine(t, nil, []PredicateDefinition{goal, fabricated}, []Operation{lying}, goal.ID, Limits{8, 8})
			if _, err := machine.Run(t.Context(), mustState(t, machine.catalog, nil, nil)); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("error = %v, want ErrInvalidTransition", err)
			}
		})
	}
}

func TestMachineStopsBeforeExecutionWhenObjectiveAlreadySatisfied(t *testing.T) {
	t.Parallel()
	goal := PredicateDefinition{ID: "goal.complete"}
	executions := 0
	finish := countedTestOperation("finish", nil, nil, []PredicateID{goal.ID}, &executions)
	machine := testMachine(t, nil, []PredicateDefinition{goal}, []Operation{finish}, goal.ID, Limits{8, 8})
	state := mustState(t, machine.catalog, nil, []PredicateID{goal.ID})
	result, err := machine.Run(t.Context(), state)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || len(result.Trace) != 0 || executions != 0 ||
		result.SelectorCalls != 0 || result.InferenceCalls != 0 {
		t.Fatalf("already-complete result = %#v executions=%d", result, executions)
	}
}

func testOperation(
	id OperationID,
	requires, provides []FactID,
	achieves []PredicateID,
) Operation {
	return countedTestOperation(id, requires, provides, achieves, nil)
}

func countedTestOperation(
	id OperationID,
	requires, provides []FactID,
	achieves []PredicateID,
	executions *int,
) Operation {
	return Operation{
		ID: id, Requires: requires, Provides: provides, Achieves: achieves,
		Execute: func(_ context.Context, input OperationInput) (Transition, error) {
			if executions != nil {
				*executions++
			}
			facts := make([]Fact, len(provides))
			for index, fact := range provides {
				facts[index] = Fact{ID: fact, Text: "known"}
			}
			return input.Transition(facts, achieves, "declared transition applied")
		},
	}
}

func testMachine(
	t *testing.T,
	facts []FactDefinition,
	predicates []PredicateDefinition,
	operations []Operation,
	desired PredicateID,
	limits Limits,
) Machine {
	t.Helper()
	catalog, err := NewCatalog(facts, predicates, operations)
	if err != nil {
		t.Fatal(err)
	}
	machine, err := NewMachine(catalog, Objective{ID: "objective.test", Desired: desired}, limits)
	if err != nil {
		t.Fatal(err)
	}
	return machine
}

func mustState(
	t *testing.T,
	catalog Catalog,
	facts []Fact,
	predicates []PredicateID,
) State {
	t.Helper()
	state, err := catalog.NewState(facts, predicates)
	if err != nil {
		t.Fatal(err)
	}
	return state
}
