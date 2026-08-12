package cognitionreference

import (
	"errors"
	"testing"
)

func TestSemanticSelectorIsNotFallbackForUnrelatedClosureFailures(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		facts      []FactDefinition
		operations []Operation
		want       error
	}{
		{
			name:  "unrelated fact has no producer",
			facts: []FactDefinition{{ID: "unrelated.missing", Kind: FactText, MaxBytes: 32}},
			operations: []Operation{
				testOperation("finish", []FactID{"unrelated.missing"}, nil, []PredicateID{"destination.reached"}),
			},
			want: ErrNoProducer,
		},
		{
			name: "unrelated deterministic cycle",
			facts: []FactDefinition{
				{ID: "cycle.first", Kind: FactText, MaxBytes: 32},
				{ID: "cycle.second", Kind: FactText, MaxBytes: 32},
			},
			operations: []Operation{
				testOperation("produce.first", []FactID{"cycle.second"}, []FactID{"cycle.first"}, nil),
				testOperation("produce.second", []FactID{"cycle.first"}, []FactID{"cycle.second"}, nil),
				testOperation("finish", []FactID{"cycle.first"}, nil, []PredicateID{"destination.reached"}),
			},
			want: ErrCausalCycle,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			selector := &fakeGapSelector{selected: "C17"}
			machine, initial := semanticMachineWithUnrelatedPath(t, selector, test.facts, test.operations)
			result, err := machine.Run(t.Context(), initial)
			if !errors.Is(err, test.want) {
				t.Fatalf("Run() error=%v, want %v", err, test.want)
			}
			if selector.calls != 0 || result.SelectorCalls != 0 || result.InferenceCalls != 0 {
				t.Fatalf("selector/inference calls=%d/%d/%d, want 0/0/0", selector.calls, result.SelectorCalls, result.InferenceCalls)
			}
			if len(result.Trace) != 0 || len(result.SemanticResolutions) != 0 || result.Complete {
				t.Fatalf("unrelated closure failure changed state: %#v", result)
			}
		})
	}
}

func TestSemanticResolutionsConsumeTheSameClosureBudgetAsOperations(t *testing.T) {
	t.Parallel()
	baseCatalog, objective, first, initial := semanticFactFixture()
	secondFact := FactDefinition{ID: "route.emphasis", Kind: FactText, MaxBytes: 32}
	second := first.Clone()
	second.FactID = secondFact.ID
	second.Gap.ID = "gap.route-emphasis"
	second.Values[0].Fact = Fact{ID: secondFact.ID, Text: "quiet"}
	second.Values[1].Fact = Fact{ID: secondFact.ID, Text: "bold"}
	executions := 0
	finish := countedTestOperation(
		"finish",
		[]FactID{first.FactID, second.FactID},
		nil,
		[]PredicateID{objective.Desired},
		&executions,
	)
	catalog, err := NewCatalog(
		[]FactDefinition{
			baseCatalog.facts["route.clue"],
			baseCatalog.facts["route.parity"],
			baseCatalog.facts[first.FactID],
			secondFact,
		},
		[]PredicateDefinition{{ID: objective.Desired}},
		[]Operation{finish},
	)
	if err != nil {
		t.Fatal(err)
	}
	selector := &fakeGapSelector{selected: "C17"}
	machine, err := NewMachineWithSemanticFacts(
		catalog,
		objective,
		Limits{MaxSteps: 1, MaxDepth: 8},
		selector,
		[]SemanticFactProducer{first, second},
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := machine.Run(t.Context(), initial)
	if !errors.Is(err, ErrClosureBound) {
		t.Fatalf("Run() error=%v, want ErrClosureBound", err)
	}
	if selector.calls != 1 || executions != 0 {
		t.Fatalf("selector calls=%d executions=%d, want one bounded resolution and zero operations", selector.calls, executions)
	}
	if len(result.Trace) != 0 || len(result.SemanticResolutions) != 0 || result.Complete {
		t.Fatalf("bounded failed run exposed committed state: %#v", result)
	}
}

func semanticMachineWithUnrelatedPath(
	t *testing.T,
	selector Selector,
	extraFacts []FactDefinition,
	operations []Operation,
) (Machine, State) {
	t.Helper()
	baseCatalog, objective, contract, initial := semanticFactFixture()
	facts := []FactDefinition{
		baseCatalog.facts["route.clue"],
		baseCatalog.facts["route.parity"],
		baseCatalog.facts["route.interpretation"],
	}
	facts = append(facts, extraFacts...)
	catalog, err := NewCatalog(
		facts,
		[]PredicateDefinition{{ID: objective.Desired}},
		operations,
	)
	if err != nil {
		t.Fatal(err)
	}
	machine, err := NewMachineWithSemanticFacts(
		catalog,
		objective,
		Limits{MaxSteps: 8, MaxDepth: 8},
		selector,
		[]SemanticFactProducer{contract},
	)
	if err != nil {
		t.Fatal(err)
	}
	return machine, initial
}
