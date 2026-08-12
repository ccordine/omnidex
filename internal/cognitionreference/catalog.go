package cognitionreference

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidCatalog   = errors.New("invalid cognition reference catalog")
	ErrInvalidObjective = errors.New("invalid cognition reference objective")
)

type Catalog struct {
	facts      map[FactID]FactDefinition
	predicates map[PredicateID]PredicateDefinition
	operations map[OperationID]Operation
	producers  map[FactID][]OperationID
	achievers  map[PredicateID][]OperationID
}

func NewCatalog(
	facts []FactDefinition,
	predicates []PredicateDefinition,
	operations []Operation,
) (Catalog, error) {
	catalog := Catalog{
		facts:      make(map[FactID]FactDefinition, len(facts)),
		predicates: make(map[PredicateID]PredicateDefinition, len(predicates)),
		operations: make(map[OperationID]Operation, len(operations)),
		producers:  make(map[FactID][]OperationID),
		achievers:  make(map[PredicateID][]OperationID),
	}
	for _, definition := range facts {
		if err := definition.validate(); err != nil {
			return Catalog{}, err
		}
		if _, duplicate := catalog.facts[definition.ID]; duplicate {
			return Catalog{}, fmt.Errorf("%w: duplicate fact %q", ErrInvalidCatalog, definition.ID)
		}
		catalog.facts[definition.ID] = definition
	}
	for _, definition := range predicates {
		if err := definition.validate(); err != nil {
			return Catalog{}, err
		}
		if _, duplicate := catalog.predicates[definition.ID]; duplicate {
			return Catalog{}, fmt.Errorf("%w: duplicate predicate %q", ErrInvalidCatalog, definition.ID)
		}
		catalog.predicates[definition.ID] = definition
	}
	if len(operations) == 0 {
		return Catalog{}, fmt.Errorf("%w: at least one operation is required", ErrInvalidCatalog)
	}
	for _, operation := range operations {
		if err := catalog.validateOperation(operation); err != nil {
			return Catalog{}, err
		}
		if _, duplicate := catalog.operations[operation.ID]; duplicate {
			return Catalog{}, fmt.Errorf("%w: duplicate operation %q", ErrInvalidCatalog, operation.ID)
		}
		cloned := cloneOperation(operation)
		catalog.operations[operation.ID] = cloned
		for _, fact := range operation.Provides {
			catalog.producers[fact] = append(catalog.producers[fact], operation.ID)
		}
		for _, predicate := range operation.Achieves {
			catalog.achievers[predicate] = append(catalog.achievers[predicate], operation.ID)
		}
	}
	for fact := range catalog.producers {
		sortOperationIDs(catalog.producers[fact])
	}
	for predicate := range catalog.achievers {
		sortOperationIDs(catalog.achievers[predicate])
	}
	return catalog, nil
}

func (catalog Catalog) validateOperation(operation Operation) error {
	if err := validIdentity(string(operation.ID)); err != nil {
		return fmt.Errorf("%w: operation ID: %v", ErrInvalidCatalog, err)
	}
	if operation.Execute == nil {
		return fmt.Errorf("%w: operation %q has no code-owned executor", ErrInvalidCatalog, operation.ID)
	}
	if len(operation.Provides) == 0 && len(operation.Achieves) == 0 {
		return fmt.Errorf("%w: operation %q has no declared effect", ErrInvalidCatalog, operation.ID)
	}
	if err := validateUniqueFacts(operation.Requires, catalog.facts, "required"); err != nil {
		return fmt.Errorf("%w: operation %q: %v", ErrInvalidCatalog, operation.ID, err)
	}
	if err := validateUniqueFacts(operation.Provides, catalog.facts, "provided"); err != nil {
		return fmt.Errorf("%w: operation %q: %v", ErrInvalidCatalog, operation.ID, err)
	}
	if err := validateUniquePredicates(operation.Achieves, catalog.predicates); err != nil {
		return fmt.Errorf("%w: operation %q: %v", ErrInvalidCatalog, operation.ID, err)
	}
	seenArguments := make(map[ArgumentName]struct{}, len(operation.Bindings))
	for _, binding := range operation.Bindings {
		if err := validIdentity(string(binding.Argument)); err != nil {
			return fmt.Errorf("%w: operation %q binding: %v", ErrInvalidCatalog, operation.ID, err)
		}
		if _, duplicate := seenArguments[binding.Argument]; duplicate {
			return fmt.Errorf("%w: operation %q duplicates argument %q", ErrInvalidCatalog, operation.ID, binding.Argument)
		}
		seenArguments[binding.Argument] = struct{}{}
		if _, exists := catalog.facts[binding.Fact]; !exists {
			return fmt.Errorf("%w: operation %q binding fact %q is not registered", ErrInvalidCatalog, operation.ID, binding.Fact)
		}
		if !containsFactID(operation.Requires, binding.Fact) {
			return fmt.Errorf("%w: operation %q binds non-required fact %q", ErrInvalidCatalog, operation.ID, binding.Fact)
		}
	}
	return nil
}

func (definition FactDefinition) validate() error {
	if err := validIdentity(string(definition.ID)); err != nil {
		return fmt.Errorf("%w: fact ID: %v", ErrInvalidCatalog, err)
	}
	if definition.Kind != FactText || definition.MaxBytes < 1 || definition.MaxBytes > maxFactBytes {
		return fmt.Errorf("%w: fact %q has invalid value schema", ErrInvalidCatalog, definition.ID)
	}
	return nil
}

func (definition PredicateDefinition) validate() error {
	if err := validIdentity(string(definition.ID)); err != nil {
		return fmt.Errorf("%w: predicate ID: %v", ErrInvalidCatalog, err)
	}
	return nil
}

func (objective Objective) validate(catalog Catalog) error {
	if err := validIdentity(string(objective.ID)); err != nil {
		return fmt.Errorf("%w: objective ID: %v", ErrInvalidObjective, err)
	}
	if _, exists := catalog.predicates[objective.Desired]; !exists {
		return fmt.Errorf("%w: desired predicate %q is not registered", ErrInvalidObjective, objective.Desired)
	}
	return nil
}

func validIdentity(value string) error {
	if value == "" || len(value) > maxIdentityBytes || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00 \t\r\n") {
		return errors.New("identity must be nonempty bounded UTF-8 without whitespace")
	}
	return nil
}

func validateUniqueFacts(values []FactID, registered map[FactID]FactDefinition, label string) error {
	seen := make(map[FactID]struct{}, len(values))
	for _, value := range values {
		if _, exists := registered[value]; !exists {
			return fmt.Errorf("%s fact %q is not registered", label, value)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s fact %q is duplicated", label, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateUniquePredicates(values []PredicateID, registered map[PredicateID]PredicateDefinition) error {
	seen := make(map[PredicateID]struct{}, len(values))
	for _, value := range values {
		if _, exists := registered[value]; !exists {
			return fmt.Errorf("achieved predicate %q is not registered", value)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("achieved predicate %q is duplicated", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func cloneOperation(operation Operation) Operation {
	operation.Requires = append([]FactID{}, operation.Requires...)
	operation.Provides = append([]FactID{}, operation.Provides...)
	operation.Achieves = append([]PredicateID{}, operation.Achieves...)
	operation.Bindings = append([]Binding{}, operation.Bindings...)
	return operation
}

func sortOperationIDs(values []OperationID) {
	sort.Slice(values, func(left, right int) bool { return values[left] < values[right] })
}

func containsFactID(values []FactID, wanted FactID) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
