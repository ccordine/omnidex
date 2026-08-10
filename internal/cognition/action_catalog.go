package cognition

import (
	"encoding/json"
	"fmt"
	"sort"
)

func NewActionCatalog(id ActionCatalogID, version string, schemas []ActionSchema) (ActionCatalog, error) {
	schemas = cloneActionSchemas(schemas)
	sort.Slice(schemas, func(left, right int) bool {
		if schemas[left].Kind != schemas[right].Kind {
			return schemas[left].Kind < schemas[right].Kind
		}
		if schemas[left].ID != schemas[right].ID {
			return schemas[left].ID < schemas[right].ID
		}
		return schemas[left].Version < schemas[right].Version
	})
	catalog := ActionCatalog{ID: id, Version: version, Schemas: schemas}
	catalog.SHA256 = actionCatalogSHA256(catalog)
	if err := catalog.Validate(); err != nil {
		return ActionCatalog{}, err
	}
	return catalog, nil
}

func (catalog ActionCatalog) Validate() error {
	if err := validateIdentity(string(catalog.ID), "action catalog ID"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidActionCatalog, err)
	}
	if err := validateVersion(catalog.Version, "action catalog version"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidActionCatalog, err)
	}
	if len(catalog.Schemas) == 0 || len(catalog.Schemas) > MaxActionSchemas {
		return fmt.Errorf("%w: schema count must be between 1 and %d", ErrInvalidActionCatalog, MaxActionSchemas)
	}
	seenKinds := make(map[ActionKind]struct{}, len(catalog.Schemas))
	var previous string
	for index, schema := range catalog.Schemas {
		if err := schema.Validate(); err != nil {
			return fmt.Errorf("%w: schema %d: %v", ErrInvalidActionCatalog, index, err)
		}
		if _, duplicate := seenKinds[schema.Kind]; duplicate {
			return fmt.Errorf("%w: action kind %q is duplicated", ErrInvalidActionCatalog, schema.Kind)
		}
		seenKinds[schema.Kind] = struct{}{}
		key := string(schema.Kind) + "\x00" + string(schema.ID) + "\x00" + schema.Version
		if index > 0 && key <= previous {
			return fmt.Errorf("%w: schemas must be uniquely sorted", ErrInvalidActionCatalog)
		}
		previous = key
	}
	if !validSHA256(catalog.SHA256) || actionCatalogSHA256(catalog) != catalog.SHA256 {
		return fmt.Errorf("%w: catalog hash does not bind the exact catalog", ErrInvalidActionCatalog)
	}
	return nil
}

func (catalog ActionCatalog) Clone() ActionCatalog {
	catalog.Schemas = cloneActionSchemas(catalog.Schemas)
	return catalog
}

func (catalog ActionCatalog) Schema(kind ActionKind) (ActionSchema, bool) {
	for _, schema := range catalog.Schemas {
		if schema.Kind == kind {
			return schema.Clone(), true
		}
	}
	return ActionSchema{}, false
}

func cloneActionSchemas(schemas []ActionSchema) []ActionSchema {
	if schemas == nil {
		return nil
	}
	cloned := make([]ActionSchema, len(schemas))
	for index, schema := range schemas {
		cloned[index] = schema.Clone()
	}
	return cloned
}

func actionCatalogSHA256(catalog ActionCatalog) string {
	payload := struct {
		ID      ActionCatalogID `json:"id"`
		Version string          `json:"version"`
		Schemas []ActionSchema  `json:"schemas"`
	}{catalog.ID, catalog.Version, catalog.Schemas}
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("marshal action catalog identity: %v", err))
	}
	return contentSHA256(string(raw))
}
