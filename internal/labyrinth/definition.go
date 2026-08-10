package labyrinth

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func NewDefinition(
	catalog cognition.ActionCatalog,
	entities []Entity,
	predicateSchemas []PredicateSchema,
	initialFacts []cognition.Predicate,
	actions []ActionDefinition,
	goal cognition.GoalExpression,
) (Definition, error) {
	definition := Definition{
		catalog:          catalog.Clone(),
		entities:         cloneEntities(entities),
		predicateSchemas: clonePredicateSchemas(predicateSchemas),
		initialFacts:     clonePredicates(initialFacts),
		actions:          cloneActions(actions),
		goal:             goal.Clone(),
	}
	if err := definition.reseal(); err != nil {
		return Definition{}, err
	}
	return definition, nil
}

func (definition *Definition) reseal() error {
	definition.canonicalize()
	if err := definition.validateContents(); err != nil {
		return err
	}
	digest, raw, err := digestJSON(definition.identity())
	if err != nil {
		return fmt.Errorf("%w: encode canonical definition: %v", ErrInvalidDefinition, err)
	}
	if len(raw) > MaxDefinitionBytes {
		return fmt.Errorf("%w: canonical definition exceeds %d bytes", ErrInvalidDefinition, MaxDefinitionBytes)
	}
	definition.sha256 = digest
	return nil
}

func (definition Definition) Validate() error {
	if definition.sha256 == "" {
		return fmt.Errorf("%w: content hash is missing", ErrInvalidDefinition)
	}
	canonical := definition.clone()
	canonical.sha256 = ""
	if err := canonical.reseal(); err != nil {
		return err
	}
	if canonicalJSON(definition.identity()) != canonicalJSON(canonical.identity()) {
		return fmt.Errorf("%w: contents are not in canonical order", ErrInvalidDefinition)
	}
	if definition.sha256 != canonical.sha256 {
		return fmt.Errorf("%w: content hash does not bind the exact definition", ErrInvalidDefinition)
	}
	return nil
}

func (definition Definition) SHA256() string {
	return definition.sha256
}

func (definition Definition) Catalog() cognition.ActionCatalog {
	return definition.catalog.Clone()
}

func (definition Definition) MarshalJSON() ([]byte, error) {
	return nil, ErrPrivateSerialization
}

// MarshalPrivateJSON serializes the complete sealed definition for an
// explicitly private environment store. Public JSON marshaling is rejected.
func (definition Definition) MarshalPrivateJSON() ([]byte, error) {
	if err := definition.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(definition.identity())
}
