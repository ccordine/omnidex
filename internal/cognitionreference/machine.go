package cognitionreference

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"unicode/utf8"
)

var (
	ErrInvalidMachine    = errors.New("invalid cognition reference machine")
	ErrNoProducer        = errors.New("no deterministic producer")
	ErrAmbiguousProducer = errors.New("ambiguous deterministic producer")
	ErrCausalCycle       = errors.New("deterministic causal cycle")
	ErrClosureBound      = errors.New("deterministic closure bound exceeded")
	ErrInvalidTransition = errors.New("invalid code-owned transition")
)

type Machine struct {
	catalog           Catalog
	objective         Objective
	limits            Limits
	selector          Selector
	semanticProducers map[FactID]SemanticFactProducer
}

func NewMachine(catalog Catalog, objective Objective, limits Limits) (Machine, error) {
	return newMachine(catalog, objective, limits, nil, nil)
}

func NewMachineWithSemanticFacts(
	catalog Catalog,
	objective Objective,
	limits Limits,
	selector Selector,
	producers []SemanticFactProducer,
) (Machine, error) {
	if selector == nil || len(producers) == 0 {
		return Machine{}, fmt.Errorf("%w: semantic producer path requires selector and producers", ErrInvalidMachine)
	}
	return newMachine(catalog, objective, limits, selector, producers)
}

func newMachine(
	catalog Catalog,
	objective Objective,
	limits Limits,
	selector Selector,
	producers []SemanticFactProducer,
) (Machine, error) {
	if len(catalog.operations) == 0 {
		return Machine{}, fmt.Errorf("%w: catalog is empty", ErrInvalidMachine)
	}
	if err := objective.validate(catalog); err != nil {
		return Machine{}, err
	}
	if limits.MaxSteps < 1 || limits.MaxDepth < 1 {
		return Machine{}, fmt.Errorf("%w: limits must be positive", ErrInvalidMachine)
	}
	semanticProducers, err := validateSemanticFactProducers(catalog, objective, producers)
	if err != nil {
		return Machine{}, err
	}
	return Machine{
		catalog: catalog, objective: objective, limits: limits,
		selector: selector, semanticProducers: semanticProducers,
	}, nil
}

func (machine Machine) Run(ctx context.Context, initial State) (Result, error) {
	if ctx == nil {
		return Result{}, fmt.Errorf("%w: context is required", ErrInvalidMachine)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	state, err := machine.catalog.validatedState(initial)
	if err != nil {
		return Result{}, err
	}
	result := Result{Trace: []TraceStep{}, SemanticResolutions: []SemanticResolution{}, Final: state.clone()}
	for {
		if state.HasPredicate(machine.objective.Desired) {
			result.Final = state.clone()
			result.Complete = true
			return result, nil
		}
		closureSteps := len(result.Trace) + len(result.SemanticResolutions)
		if closureSteps >= machine.limits.MaxSteps {
			return Result{}, fmt.Errorf("%w: exceeded %d steps", ErrClosureBound, machine.limits.MaxSteps)
		}
		operation, missingSemanticFact, err := machine.nextOperation(state)
		if err != nil {
			return Result{}, err
		}
		if missingSemanticFact != "" {
			producer := machine.semanticProducers[missingSemanticFact]
			if err := producer.validateState(state); err != nil {
				return Result{}, err
			}
			selected, err := SelectCandidate(ctx, machine.selector, producer.Gap)
			if err != nil {
				return Result{}, err
			}
			resolution, err := producer.resolve(selected)
			if err != nil {
				return Result{}, err
			}
			state = state.clone()
			state.facts[resolution.Fact.ID] = resolution.Fact
			result.SelectorCalls++
			result.InferenceCalls++
			result.SemanticResolutions = append(result.SemanticResolutions, resolution)
			result.Final = state.clone()
			continue
		}
		arguments, err := machine.bind(operation, state)
		if err != nil {
			return Result{}, err
		}
		input := OperationInput{operation: operation.ID, arguments: arguments, state: state.clone()}
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		transition, executeErr := operation.Execute(ctx, input)
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if executeErr != nil {
			return Result{}, fmt.Errorf("operation %q: %w", operation.ID, executeErr)
		}
		if err := machine.validateTransition(operation, transition); err != nil {
			return Result{}, err
		}
		state = machine.apply(state, transition)
		result.Trace = append(result.Trace, TraceStep{
			Sequence: len(result.Trace) + 1, Operation: operation.ID,
			Arguments: cloneArguments(arguments), Facts: cloneFacts(transition.facts),
			Predicates: clonePredicates(transition.predicates), Outcome: transition.outcome,
		})
	}
}

func (machine Machine) nextOperation(state State) (Operation, FactID, error) {
	achievers := machine.catalog.achievers[machine.objective.Desired]
	if len(achievers) == 0 {
		return Operation{}, "", fmt.Errorf("%w: predicate %q", ErrNoProducer, machine.objective.Desired)
	}
	if len(achievers) > 1 {
		return Operation{}, "", fmt.Errorf("%w: predicate %q", ErrAmbiguousProducer, machine.objective.Desired)
	}
	path := make(map[OperationID]struct{})
	return machine.firstReady(machine.catalog.operations[achievers[0]], state, path, 1)
}

func (machine Machine) firstReady(
	operation Operation,
	state State,
	path map[OperationID]struct{},
	depth int,
) (Operation, FactID, error) {
	if depth > machine.limits.MaxDepth {
		return Operation{}, "", fmt.Errorf("%w: exceeded depth %d", ErrClosureBound, machine.limits.MaxDepth)
	}
	if _, cycle := path[operation.ID]; cycle {
		return Operation{}, "", fmt.Errorf("%w: operation %q", ErrCausalCycle, operation.ID)
	}
	path[operation.ID] = struct{}{}
	defer delete(path, operation.ID)
	for _, requirement := range operation.Requires {
		if _, exists := state.Fact(requirement); exists {
			continue
		}
		producers := machine.catalog.producers[requirement]
		if len(producers) == 0 {
			if _, exists := machine.semanticProducers[requirement]; exists {
				return Operation{}, requirement, nil
			}
			return Operation{}, "", fmt.Errorf("%w: fact %q", ErrNoProducer, requirement)
		}
		if len(producers) > 1 {
			return Operation{}, "", fmt.Errorf("%w: fact %q", ErrAmbiguousProducer, requirement)
		}
		return machine.firstReady(machine.catalog.operations[producers[0]], state, path, depth+1)
	}
	return cloneOperation(operation), "", nil
}

func (machine Machine) bind(operation Operation, state State) ([]Argument, error) {
	arguments := make([]Argument, len(operation.Bindings))
	for index, binding := range operation.Bindings {
		fact, exists := state.Fact(binding.Fact)
		if !exists {
			return nil, fmt.Errorf("%w: operation %q lacks fact %q", ErrInvalidMachine, operation.ID, binding.Fact)
		}
		arguments[index] = Argument{Name: binding.Argument, Value: fact.Text}
	}
	sort.Slice(arguments, func(left, right int) bool { return arguments[left].Name < arguments[right].Name })
	return arguments, nil
}

func (machine Machine) validateTransition(operation Operation, transition Transition) error {
	if transition.operation != operation.ID {
		return fmt.Errorf("%w: transition operation does not match executor", ErrInvalidTransition)
	}
	if len(transition.outcome) == 0 || len(transition.outcome) > maxOutcomeBytes ||
		!utf8.ValidString(transition.outcome) {
		return fmt.Errorf("%w: outcome is empty or oversized", ErrInvalidTransition)
	}
	if !exactFactIDs(transition.facts, operation.Provides) {
		return fmt.Errorf("%w: operation %q did not establish exactly its declared facts", ErrInvalidTransition, operation.ID)
	}
	if !exactPredicateIDs(transition.predicates, operation.Achieves) {
		return fmt.Errorf("%w: operation %q did not establish exactly its declared predicates", ErrInvalidTransition, operation.ID)
	}
	for _, fact := range transition.facts {
		definition := machine.catalog.facts[fact.ID]
		if !validFactText(fact.Text, definition.MaxBytes) {
			return fmt.Errorf("%w: fact %q violates its schema", ErrInvalidTransition, fact.ID)
		}
	}
	return nil
}

func (machine Machine) apply(state State, transition Transition) State {
	next := state.clone()
	for _, fact := range transition.facts {
		next.facts[fact.ID] = fact
	}
	for _, predicate := range transition.predicates {
		next.predicates[predicate] = struct{}{}
	}
	return next
}

func (state State) clone() State {
	cloned := newEmptyState()
	for id, fact := range state.facts {
		cloned.facts[id] = fact
	}
	for predicate := range state.predicates {
		cloned.predicates[predicate] = struct{}{}
	}
	return cloned
}

func exactFactIDs(facts []Fact, expected []FactID) bool {
	got := make(map[FactID]struct{}, len(facts))
	for _, fact := range facts {
		if _, duplicate := got[fact.ID]; duplicate {
			return false
		}
		got[fact.ID] = struct{}{}
	}
	return exactIDs(got, expected)
}

func exactPredicateIDs(values []PredicateID, expected []PredicateID) bool {
	got := make(map[PredicateID]struct{}, len(values))
	for _, value := range values {
		if _, duplicate := got[value]; duplicate {
			return false
		}
		got[value] = struct{}{}
	}
	return exactIDs(got, expected)
}

func exactIDs[T comparable](got map[T]struct{}, expected []T) bool {
	if len(got) != len(expected) {
		return false
	}
	for _, value := range expected {
		if _, exists := got[value]; !exists {
			return false
		}
	}
	return true
}
