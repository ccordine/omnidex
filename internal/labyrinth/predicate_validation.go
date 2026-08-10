package labyrinth

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func validateGroundPredicates(
	values []cognition.Predicate,
	entities map[EntityID]Entity,
	schemas map[cognition.PredicateName]PredicateSchema,
	label string,
) error {
	previous := ""
	for index, predicate := range values {
		if err := validateGroundPredicate(predicate, entities, schemas); err != nil {
			return fmt.Errorf("%w: %s %d: %v", ErrInvalidDefinition, label, index, err)
		}
		key := predicateKey(predicate)
		if index > 0 && key == previous {
			return fmt.Errorf("%w: duplicate %s at index %d", ErrInvalidDefinition, label, index)
		}
		previous = key
	}
	return nil
}

func validateGroundPredicate(
	predicate cognition.Predicate,
	entities map[EntityID]Entity,
	schemas map[cognition.PredicateName]PredicateSchema,
) error {
	if err := predicate.Validate(); err != nil {
		return err
	}
	schema, exists := schemas[predicate.Name]
	if !exists || len(predicate.Args) != len(schema.ArgumentKinds) {
		return fmt.Errorf("predicate does not match a registered schema")
	}
	for index, argument := range predicate.Args {
		entity, exists := entities[EntityID(argument)]
		if !exists || entity.Kind != schema.ArgumentKinds[index] {
			return fmt.Errorf("predicate argument %d does not match a registered entity", index)
		}
	}
	return nil
}

func predicateIsPublic(
	predicate cognition.Predicate,
	entities map[EntityID]Entity,
	schemas map[cognition.PredicateName]PredicateSchema,
) bool {
	schema, exists := schemas[predicate.Name]
	if !exists || !schema.Public {
		return false
	}
	for _, argument := range predicate.Args {
		entity, exists := entities[EntityID(argument)]
		if !exists || !entity.Public {
			return false
		}
	}
	return true
}
