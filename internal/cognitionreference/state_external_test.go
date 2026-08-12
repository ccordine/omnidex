package cognitionreference_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreference"
)

func TestCatalogConstructsExactAuthoritativeStateForExternalConsumers(t *testing.T) {
	t.Parallel()
	fact := cognitionreference.FactDefinition{ID: "observation.clue", Kind: cognitionreference.FactText, MaxBytes: 32}
	goal := cognitionreference.PredicateDefinition{ID: "goal.complete"}
	finish := cognitionreference.Operation{
		ID: "finish", Requires: []cognitionreference.FactID{fact.ID},
		Achieves: []cognitionreference.PredicateID{goal.ID},
		Execute: func(_ context.Context, input cognitionreference.OperationInput) (cognitionreference.Transition, error) {
			return input.Transition(nil, []cognitionreference.PredicateID{goal.ID}, "completed")
		},
	}
	catalog, err := cognitionreference.NewCatalog(
		[]cognitionreference.FactDefinition{fact},
		[]cognitionreference.PredicateDefinition{goal},
		[]cognitionreference.Operation{finish},
	)
	if err != nil {
		t.Fatal(err)
	}
	state, err := catalog.NewState(
		[]cognitionreference.Fact{{ID: fact.ID, Text: "bounded clue"}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, exists := state.Fact(fact.ID); !exists || got.Text != "bounded clue" {
		t.Fatalf("Fact(%q)=(%#v,%t), want exact registered fact", fact.ID, got, exists)
	}

	for name, facts := range map[string][]cognitionreference.Fact{
		"unregistered": {{ID: "fabricated", Text: "value"}},
		"duplicate": {
			{ID: fact.ID, Text: "first"},
			{ID: fact.ID, Text: "second"},
		},
		"oversized":     {{ID: fact.ID, Text: "more than thirty-two bytes of content"}},
		"invalid UTF-8": {{ID: fact.ID, Text: string([]byte{0xff})}},
		"NUL":           {{ID: fact.ID, Text: "invalid\x00value"}},
		"inexact text":  {{ID: fact.ID, Text: " padded "}},
	} {
		name, facts := name, facts
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := catalog.NewState(facts, nil); !errors.Is(err, cognitionreference.ErrInvalidState) {
				t.Fatalf("NewState() error=%v, want ErrInvalidState", err)
			}
		})
	}
	for name, predicates := range map[string][]cognitionreference.PredicateID{
		"unregistered predicate": {"fabricated"},
		"duplicate predicate":    {goal.ID, goal.ID},
	} {
		name, predicates := name, predicates
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := catalog.NewState(nil, predicates); !errors.Is(err, cognitionreference.ErrInvalidState) {
				t.Fatalf("NewState() error=%v, want ErrInvalidState", err)
			}
		})
	}
}
