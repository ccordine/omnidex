package cognitionreference

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var ErrInvalidState = errors.New("invalid cognition reference state")

// NewState constructs an immutable-by-boundary state from this exact catalog.
// No fact or predicate can enter the machine without catalog validation.
func (catalog Catalog) NewState(facts []Fact, predicates []PredicateID) (State, error) {
	state := newEmptyState()
	for index, fact := range facts {
		definition, exists := catalog.facts[fact.ID]
		if !exists {
			return State{}, fmt.Errorf("%w: fact %d ID %q is not registered", ErrInvalidState, index, fact.ID)
		}
		if _, duplicate := state.facts[fact.ID]; duplicate {
			return State{}, fmt.Errorf("%w: fact %q is duplicated", ErrInvalidState, fact.ID)
		}
		if !validFactText(fact.Text, definition.MaxBytes) {
			return State{}, fmt.Errorf("%w: fact %q violates its registered schema", ErrInvalidState, fact.ID)
		}
		state.facts[fact.ID] = fact
	}
	for index, predicate := range predicates {
		if _, exists := catalog.predicates[predicate]; !exists {
			return State{}, fmt.Errorf(
				"%w: predicate %d ID %q is not registered", ErrInvalidState, index, predicate,
			)
		}
		if _, duplicate := state.predicates[predicate]; duplicate {
			return State{}, fmt.Errorf("%w: predicate %q is duplicated", ErrInvalidState, predicate)
		}
		state.predicates[predicate] = struct{}{}
	}
	return state, nil
}

func validFactText(value string, maxBytes int) bool {
	return value != "" && len(value) <= maxBytes && utf8.ValidString(value) &&
		!strings.ContainsRune(value, 0) && strings.TrimSpace(value) == value
}

func (catalog Catalog) validatedState(state State) (State, error) {
	if state.facts == nil || state.predicates == nil {
		return State{}, fmt.Errorf("%w: state maps are not initialized", ErrInvalidState)
	}
	facts := make([]Fact, 0, len(state.facts))
	for id, fact := range state.facts {
		if fact.ID != id {
			return State{}, fmt.Errorf("%w: fact map key %q differs from fact ID %q", ErrInvalidState, id, fact.ID)
		}
		facts = append(facts, fact)
	}
	predicates := make([]PredicateID, 0, len(state.predicates))
	for predicate := range state.predicates {
		predicates = append(predicates, predicate)
	}
	return catalog.NewState(facts, predicates)
}
