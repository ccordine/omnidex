package cognitionreference

import (
	"context"
	"unicode/utf8"
)

type FactID string
type PredicateID string
type OperationID string
type ObjectiveID string
type ArgumentName string
type FactKind string

const FactText FactKind = "text"

const (
	maxIdentityBytes = 128
	maxFactBytes     = 4096
	maxOutcomeBytes  = 512
)

type FactDefinition struct {
	ID       FactID
	Kind     FactKind
	MaxBytes int
}

type PredicateDefinition struct {
	ID PredicateID
}

type Fact struct {
	ID   FactID
	Text string
}

type Argument struct {
	Name  ArgumentName
	Value string
}

type Binding struct {
	Argument ArgumentName
	Fact     FactID
}

type Objective struct {
	ID      ObjectiveID
	Desired PredicateID
}

type OperationFunc func(context.Context, OperationInput) (Transition, error)

type Operation struct {
	ID       OperationID
	Requires []FactID
	Provides []FactID
	Achieves []PredicateID
	Bindings []Binding
	Execute  OperationFunc
}

type OperationInput struct {
	operation OperationID
	arguments []Argument
	state     State
}

type Transition struct {
	operation  OperationID
	facts      []Fact
	predicates []PredicateID
	outcome    string
}

type TraceStep struct {
	Sequence   int
	Operation  OperationID
	Arguments  []Argument
	Facts      []Fact
	Predicates []PredicateID
	Outcome    string
}

type Limits struct {
	MaxSteps int
	MaxDepth int
}

type Result struct {
	Trace               []TraceStep
	SemanticResolutions []SemanticResolution
	Final               State
	Complete            bool
	SelectorCalls       int
	InferenceCalls      int
}

type State struct {
	facts      map[FactID]Fact
	predicates map[PredicateID]struct{}
}

func newEmptyState() State {
	return State{facts: make(map[FactID]Fact), predicates: make(map[PredicateID]struct{})}
}

func (state State) Fact(id FactID) (Fact, bool) {
	fact, exists := state.facts[id]
	return fact, exists
}

func (state State) HasPredicate(id PredicateID) bool {
	_, exists := state.predicates[id]
	return exists
}

func (input OperationInput) Argument(name ArgumentName) (string, bool) {
	for _, argument := range input.arguments {
		if argument.Name == name {
			return argument.Value, true
		}
	}
	return "", false
}

func (input OperationInput) Transition(
	facts []Fact,
	predicates []PredicateID,
	outcome string,
) (Transition, error) {
	if len(outcome) == 0 || len(outcome) > maxOutcomeBytes || !utf8.ValidString(outcome) {
		return Transition{}, ErrInvalidTransition
	}
	for _, fact := range facts {
		if !utf8.ValidString(fact.Text) {
			return Transition{}, ErrInvalidTransition
		}
	}
	transition := Transition{
		operation:  input.operation,
		facts:      cloneFacts(facts),
		predicates: clonePredicates(predicates),
		outcome:    outcome,
	}
	return transition, nil
}

func cloneFacts(values []Fact) []Fact {
	if values == nil {
		return []Fact{}
	}
	return append([]Fact{}, values...)
}

func clonePredicates(values []PredicateID) []PredicateID {
	if values == nil {
		return []PredicateID{}
	}
	return append([]PredicateID{}, values...)
}

func cloneArguments(values []Argument) []Argument {
	if values == nil {
		return []Argument{}
	}
	return append([]Argument{}, values...)
}
