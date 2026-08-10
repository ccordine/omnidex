package labyrinth

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/cognition"
)

func (definition Definition) validateContents() error {
	if err := definition.catalog.Validate(); err != nil {
		return fmt.Errorf("%w: action catalog: %v", ErrInvalidDefinition, err)
	}
	if len(definition.entities) == 0 || len(definition.entities) > MaxEntities {
		return fmt.Errorf("%w: entity count must be between 1 and %d", ErrInvalidDefinition, MaxEntities)
	}
	if len(definition.predicateSchemas) == 0 || len(definition.predicateSchemas) > MaxPredicateSchemas {
		return fmt.Errorf("%w: predicate schema count must be between 1 and %d", ErrInvalidDefinition, MaxPredicateSchemas)
	}
	if len(definition.initialFacts) > MaxInitialFacts {
		return fmt.Errorf("%w: initial fact count exceeds %d", ErrInvalidDefinition, MaxInitialFacts)
	}
	if len(definition.actions) == 0 || len(definition.actions) > MaxActionDefinitions {
		return fmt.Errorf("%w: action count must be between 1 and %d", ErrInvalidDefinition, MaxActionDefinitions)
	}
	entities, kinds, err := validateEntities(definition.entities)
	if err != nil {
		return err
	}
	schemas, err := validatePredicateSchemas(definition.predicateSchemas, kinds)
	if err != nil {
		return err
	}
	if err := validateGroundPredicates(definition.initialFacts, entities, schemas, "initial fact"); err != nil {
		return err
	}
	if err := definition.goal.Validate(); err != nil {
		return fmt.Errorf("%w: goal: %v", ErrInvalidDefinition, err)
	}
	for _, group := range []struct {
		name       string
		predicates []cognition.Predicate
	}{
		{"all", definition.goal.All},
		{"any", definition.goal.Any},
		{"not", definition.goal.Not},
	} {
		if err := validateGroundPredicates(group.predicates, entities, schemas, "goal "+group.name); err != nil {
			return err
		}
	}
	initial := newFactSet(definition.initialFacts)
	if _, err := publicObservationContent(
		initial, goalSatisfied(definition.goal, initial), entities, schemas, nil, false,
	); err != nil {
		return err
	}
	return validateActions(definition.catalog, definition.actions, entities, schemas)
}

func validateEntities(values []Entity) (map[EntityID]Entity, map[EntityKind]struct{}, error) {
	entities := make(map[EntityID]Entity, len(values))
	kinds := make(map[EntityKind]struct{})
	for index, entity := range values {
		if !validSymbol(string(entity.ID)) || !validSymbol(string(entity.Kind)) {
			return nil, nil, fmt.Errorf("%w: entity %d has an invalid ID or kind", ErrInvalidDefinition, index)
		}
		if _, duplicate := entities[entity.ID]; duplicate {
			return nil, nil, fmt.Errorf("%w: duplicate entity at index %d", ErrInvalidDefinition, index)
		}
		entities[entity.ID] = entity
		kinds[entity.Kind] = struct{}{}
	}
	return entities, kinds, nil
}

func validatePredicateSchemas(
	values []PredicateSchema,
	kinds map[EntityKind]struct{},
) (map[cognition.PredicateName]PredicateSchema, error) {
	schemas := make(map[cognition.PredicateName]PredicateSchema, len(values))
	for index, schema := range values {
		if err := (cognition.Predicate{Name: schema.Name}).Validate(); err != nil {
			return nil, fmt.Errorf("%w: predicate schema %d: %v", ErrInvalidDefinition, index, err)
		}
		if len(schema.ArgumentKinds) > cognition.MaxPredicateArgs {
			return nil, fmt.Errorf("%w: predicate schema %d exceeds argument bounds", ErrInvalidDefinition, index)
		}
		if _, duplicate := schemas[schema.Name]; duplicate {
			return nil, fmt.Errorf("%w: duplicate predicate schema at index %d", ErrInvalidDefinition, index)
		}
		for _, kind := range schema.ArgumentKinds {
			if _, exists := kinds[kind]; !exists {
				return nil, fmt.Errorf(
					"%w: predicate schema %d %q names unknown entity kind %q",
					ErrInvalidDefinition, index, schema.Name, kind,
				)
			}
		}
		schemas[schema.Name] = schema
	}
	return schemas, nil
}

func validSymbol(value string) bool {
	if value == "" || len(value) > cognition.MaxIdentityBytes || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return false
	}
	for index, character := range value {
		if unicode.IsSpace(character) || !(unicode.IsLetter(character) || unicode.IsDigit(character) ||
			index > 0 && strings.ContainsRune("-_.:/", character)) {
			return false
		}
	}
	return true
}
