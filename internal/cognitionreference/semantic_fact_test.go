package cognitionreference

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestMachineAcceptsOneSemanticFactThenRerunsDeterministicClosure(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		candidate CandidateID
		meaning   string
	}{{candidate: "C17", meaning: "sheltered"}, {candidate: "C23", meaning: "exposed"}} {
		test := test
		t.Run(string(test.candidate), func(t *testing.T) {
			t.Parallel()
			selector := &fakeGapSelector{selected: test.candidate}
			machine, initial, gap, err := semanticFactMachine(selector)
			if err != nil {
				t.Fatal(err)
			}

			result, err := machine.Run(t.Context(), initial)
			if err != nil {
				t.Fatal(err)
			}
			if selector.calls != 1 || !reflect.DeepEqual(selector.received, gap) {
				t.Fatalf("selector calls=%d gap=%#v, want one exact semantic gap", selector.calls, selector.received)
			}
			if !result.Complete || !result.Final.HasPredicate("destination.reached") {
				t.Fatalf("machine did not reach code-owned completion: %#v", result)
			}
			if result.SelectorCalls != 1 || result.InferenceCalls != 1 || len(result.Trace) != 1 {
				t.Fatalf("calls/trace=%d/%d/%d, want 1/1/1", result.SelectorCalls, result.InferenceCalls, len(result.Trace))
			}
			wantArgument := []Argument{{Name: "meaning", Value: test.meaning}}
			if result.Trace[0].Operation != "follow.interpretation" ||
				!reflect.DeepEqual(result.Trace[0].Arguments, wantArgument) {
				t.Fatalf("trace=%#v, want unique code-owned operation with %#v", result.Trace, wantArgument)
			}
			wantResolution := []SemanticResolution{{
				GapID: "gap.route-meaning", CandidateID: test.candidate,
				Fact: Fact{ID: "route.interpretation", Text: test.meaning},
			}}
			if !reflect.DeepEqual(result.SemanticResolutions, wantResolution) {
				t.Fatalf("resolutions=%#v, want %#v", result.SemanticResolutions, wantResolution)
			}
		})
	}
}

func TestMachineRejectsUnknownSemanticSelectionBeforeRealityChanges(t *testing.T) {
	t.Parallel()
	machine, initial, _, err := semanticFactMachine(&fakeGapSelector{selected: "C99"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := machine.Run(t.Context(), initial)
	if !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("Run() error=%v, want ErrInvalidSelection", err)
	}
	if len(result.Trace) != 0 || result.Complete || len(result.SemanticResolutions) != 0 {
		t.Fatalf("invalid selection changed reality or semantic state: %#v", result)
	}
}

func TestMachinePropagatesSelectorFailureWithoutFallback(t *testing.T) {
	t.Parallel()
	want := errors.New("semantic provider failed")
	selector := &fakeGapSelector{selected: "C17", err: want}
	machine, initial, _, err := semanticFactMachine(selector)
	if err != nil {
		t.Fatal(err)
	}
	result, err := machine.Run(t.Context(), initial)
	if !errors.Is(err, want) || selector.calls != 1 {
		t.Fatalf("Run() error=%v calls=%d, want exact selector failure and one call", err, selector.calls)
	}
	if len(result.Trace) != 0 || len(result.SemanticResolutions) != 0 || result.Complete {
		t.Fatalf("selector failure changed state: %#v", result)
	}
}

func TestMachineRejectsGapEvidenceThatDiffersFromCurrentCodeState(t *testing.T) {
	t.Parallel()
	selector := &fakeGapSelector{selected: "C17"}
	machine, initial, _, err := semanticFactMachine(selector)
	if err != nil {
		t.Fatal(err)
	}
	initial.facts["route.clue"] = Fact{ID: "route.clue", Text: "changed authoritative clue"}
	if _, err := machine.Run(t.Context(), initial); !errors.Is(err, ErrInvalidSemanticFactProducer) {
		t.Fatalf("Run() error=%v, want exact evidence mismatch", err)
	}
	if selector.calls != 0 {
		t.Fatalf("selector calls=%d, want zero before authoritative evidence match", selector.calls)
	}
}

func TestSemanticFactContractRequiresExactCandidateAndEvidenceTotality(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*SemanticFactProducer){
		"missing candidate value": func(contract *SemanticFactProducer) {
			contract.Values = contract.Values[:1]
		},
		"duplicate candidate value": func(contract *SemanticFactProducer) {
			contract.Values[1].CandidateID = contract.Values[0].CandidateID
		},
		"duplicate semantic value": func(contract *SemanticFactProducer) {
			contract.Values[1].Fact.Text = contract.Values[0].Fact.Text
		},
		"inexact semantic value": func(contract *SemanticFactProducer) {
			contract.Values[0].Fact.Text = " sheltered "
		},
		"invalid UTF-8 semantic value": func(contract *SemanticFactProducer) {
			contract.Values[0].Fact.Text = string([]byte{0xff})
		},
		"wrong fact": func(contract *SemanticFactProducer) { contract.FactID = "unregistered" },
		"unbound gap evidence": func(contract *SemanticFactProducer) {
			contract.EvidenceBindings = contract.EvidenceBindings[:1]
		},
		"duplicate evidence source": func(contract *SemanticFactProducer) {
			contract.EvidenceBindings[1].FactID = contract.EvidenceBindings[0].FactID
		},
		"wrong objective": func(contract *SemanticFactProducer) {
			contract.Gap.ObjectiveID = "objective.other"
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			catalog, objective, contract, _ := semanticFactFixture()
			mutate(&contract)
			if _, err := NewMachineWithSemanticFacts(
				catalog, objective, Limits{MaxSteps: 4, MaxDepth: 4},
				&fakeGapSelector{selected: "C17"}, []SemanticFactProducer{contract},
			); !errors.Is(err, ErrInvalidSemanticFactProducer) {
				t.Fatalf("constructor error=%v, want ErrInvalidSemanticFactProducer", err)
			}
		})
	}
}

func TestSemanticFactTypesCannotMapCandidatesToOperations(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"semantic_fact.go", "gap.go"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		for _, forbidden := range []string{
			"Operation OperationID", "OperationID `json", "CandidateIDToOperation",
		} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s contains forbidden model-choice operation mapping %q", name, forbidden)
			}
		}
	}
}

func semanticFactMachine(selector Selector) (Machine, State, SemanticGap, error) {
	catalog, objective, contract, initial := semanticFactFixture()
	machine, err := NewMachineWithSemanticFacts(
		catalog, objective, Limits{MaxSteps: 4, MaxDepth: 4}, selector,
		[]SemanticFactProducer{contract},
	)
	return machine, initial, contract.Gap.Clone(), err
}

func semanticFactFixture() (Catalog, Objective, SemanticFactProducer, State) {
	clue := FactDefinition{ID: "route.clue", Kind: FactText, MaxBytes: 128}
	parity := FactDefinition{ID: "route.parity", Kind: FactText, MaxBytes: 128}
	meaning := FactDefinition{ID: "route.interpretation", Kind: FactText, MaxBytes: 32}
	reached := PredicateDefinition{ID: "destination.reached"}
	follow := Operation{
		ID: "follow.interpretation", Requires: []FactID{meaning.ID},
		Achieves: []PredicateID{reached.ID},
		Bindings: []Binding{{Argument: "meaning", Fact: meaning.ID}},
		Execute: func(_ context.Context, input OperationInput) (Transition, error) {
			meaning, exists := input.Argument("meaning")
			if !exists || (meaning != "sheltered" && meaning != "exposed") {
				return Transition{}, errors.New("operation received an unregistered interpretation")
			}
			return input.Transition(nil, []PredicateID{reached.ID}, meaning+" route traversed")
		},
	}
	catalog, err := NewCatalog(
		[]FactDefinition{clue, parity, meaning}, []PredicateDefinition{reached}, []Operation{follow},
	)
	if err != nil {
		panic(err)
	}
	objective := Objective{ID: "objective.reach-destination", Desired: reached.ID}
	gap := validSemanticGap()
	contract := SemanticFactProducer{
		FactID: meaning.ID, Gap: gap,
		EvidenceBindings: []SemanticEvidenceBinding{
			{EvidenceID: "E10", FactID: clue.ID}, {EvidenceID: "E20", FactID: parity.ID},
		},
		Values: []SemanticCandidateValue{
			{CandidateID: "C17", Fact: Fact{ID: meaning.ID, Text: "sheltered"}},
			{CandidateID: "C23", Fact: Fact{ID: meaning.ID, Text: "exposed"}},
		},
	}
	initial, err := catalog.NewState([]Fact{
		{ID: clue.ID, Text: gap.Evidence[0].Content},
		{ID: parity.ID, Text: gap.Evidence[1].Content},
	}, nil)
	if err != nil {
		panic(err)
	}
	return catalog, objective, contract, initial
}
