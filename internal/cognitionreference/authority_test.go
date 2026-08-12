package cognitionreference

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestMachineCancellationCannotExecuteOrCommit(t *testing.T) {
	t.Parallel()
	goal := PredicateDefinition{ID: "goal.complete"}
	executions := 0
	finish := countedTestOperation("finish", nil, nil, []PredicateID{goal.ID}, &executions)
	machine := testMachine(t, nil, []PredicateDefinition{goal}, []Operation{finish}, goal.ID, Limits{8, 8})

	initial := mustState(t, machine.catalog, nil, nil)
	result, err := machine.Run(nil, initial)
	if !errors.Is(err, ErrInvalidMachine) || executions != 0 || !reflect.DeepEqual(result, Result{}) {
		t.Fatalf("nil context result=%#v error=%v executions=%d", result, err, executions)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	result, err = machine.Run(canceled, initial)
	if !errors.Is(err, context.Canceled) || executions != 0 || !reflect.DeepEqual(result, Result{}) {
		t.Fatalf("pre-canceled result=%#v error=%v executions=%d", result, err, executions)
	}

	during, cancelDuring := context.WithCancel(context.Background())
	canceling := Operation{
		ID: "canceling.finish", Achieves: []PredicateID{goal.ID},
		Execute: func(_ context.Context, input OperationInput) (Transition, error) {
			executions++
			transition, transitionErr := input.Transition(nil, []PredicateID{goal.ID}, "must not commit")
			cancelDuring()
			return transition, transitionErr
		},
	}
	machine = testMachine(t, nil, []PredicateDefinition{goal}, []Operation{canceling}, goal.ID, Limits{8, 8})
	result, err = machine.Run(during, initial)
	if !errors.Is(err, context.Canceled) || executions != 1 || !reflect.DeepEqual(result, Result{}) {
		t.Fatalf("mid-execution cancellation result=%#v error=%v executions=%d", result, err, executions)
	}
}

func TestCatalogAndTransitionsRequireHardBoundedUTF8(t *testing.T) {
	t.Parallel()
	goal := PredicateDefinition{ID: "goal.complete"}
	finish := testOperation("finish", nil, nil, []PredicateID{goal.ID})
	oversized := FactDefinition{ID: "fact.oversized", Kind: FactText, MaxBytes: maxFactBytes + 1}
	if _, err := NewCatalog([]FactDefinition{oversized}, []PredicateDefinition{goal}, []Operation{finish}); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("oversized fact schema error = %v, want ErrInvalidCatalog", err)
	}

	invalidUTF8 := string([]byte{0xff})
	for name, operation := range map[string]Operation{
		"fact": {
			ID: "invalid.fact", Provides: []FactID{"fact.value"}, Achieves: []PredicateID{goal.ID},
			Execute: func(_ context.Context, input OperationInput) (Transition, error) {
				return input.Transition(
					[]Fact{{ID: "fact.value", Text: invalidUTF8}}, []PredicateID{goal.ID}, "invalid fact",
				)
			},
		},
		"outcome": {
			ID: "invalid.outcome", Achieves: []PredicateID{goal.ID},
			Execute: func(_ context.Context, input OperationInput) (Transition, error) {
				return input.Transition(nil, []PredicateID{goal.ID}, invalidUTF8)
			},
		},
	} {
		name, operation := name, operation
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			facts := []FactDefinition{}
			if len(operation.Provides) != 0 {
				facts = []FactDefinition{{ID: "fact.value", Kind: FactText, MaxBytes: 16}}
			}
			machine := testMachine(t, facts, []PredicateDefinition{goal}, []Operation{operation}, goal.ID, Limits{8, 8})
			if _, err := machine.Run(t.Context(), mustState(t, machine.catalog, nil, nil)); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("invalid UTF-8 %s error = %v, want ErrInvalidTransition", name, err)
			}
		})
	}
}
