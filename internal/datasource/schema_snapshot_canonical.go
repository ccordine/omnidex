package datasource

import (
	"fmt"
	"sort"
	"strings"
)

func canonicalizeRelationDefinitions(input []RelationDefinition) ([]RelationDefinition, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("schema snapshot requires 1..%d relations", MaxSchemaRelations)
	}
	definitions := cloneRelationDefinitions(input)
	sort.Slice(definitions, func(i, j int) bool {
		return relationDefinitionKey(definitions[i].Schema, definitions[i].Name) < relationDefinitionKey(definitions[j].Schema, definitions[j].Name)
	})
	seenRelations := map[string]struct{}{}
	totalColumns := 0
	for index := range definitions {
		definition := &definitions[index]
		definition.Schema = strings.TrimSpace(definition.Schema)
		definition.Name = strings.TrimSpace(definition.Name)
		key := relationDefinitionKey(definition.Schema, definition.Name)
		if definition.Schema == "" || definition.Name == "" || strings.ContainsRune(definition.Schema, '\x00') || strings.ContainsRune(definition.Name, '\x00') {
			return nil, fmt.Errorf("relation %d requires valid schema and name", index)
		}
		if !validRelationKind(definition.Kind) {
			return nil, fmt.Errorf("relation %s has unsupported kind %q", key, definition.Kind)
		}
		if _, exists := seenRelations[key]; exists {
			return nil, fmt.Errorf("duplicate relation %q", key)
		}
		seenRelations[key] = struct{}{}
		if err := canonicalizeRelationDefinition(definition); err != nil {
			return nil, fmt.Errorf("relation %s: %w", key, err)
		}
		totalColumns += len(definition.Columns)
		if totalColumns > MaxSchemaColumns {
			return nil, fmt.Errorf("schema snapshot exceeds %d columns", MaxSchemaColumns)
		}
	}
	return definitions, nil
}

func canonicalizeRelationDefinition(definition *RelationDefinition) error {
	if len(definition.Columns) == 0 {
		return fmt.Errorf("requires at least one column")
	}
	sort.Slice(definition.Columns, func(i, j int) bool { return definition.Columns[i].Ordinal < definition.Columns[j].Ordinal })
	columns := map[string]struct{}{}
	ordinals := map[int]struct{}{}
	for index := range definition.Columns {
		column := &definition.Columns[index]
		column.Name = strings.TrimSpace(column.Name)
		column.DataType = strings.TrimSpace(column.DataType)
		if column.Name == "" || column.Ordinal <= 0 || column.DataType == "" || strings.ContainsRune(column.Name, '\x00') || strings.ContainsRune(column.DataType, '\x00') || !validTypeCategory(column.TypeCategory) {
			return fmt.Errorf("column %d has invalid name, ordinal, data type, or category", index)
		}
		if _, exists := columns[column.Name]; exists {
			return fmt.Errorf("duplicate column %q", column.Name)
		}
		if _, exists := ordinals[column.Ordinal]; exists {
			return fmt.Errorf("duplicate column ordinal %d", column.Ordinal)
		}
		if err := canonicalizeAllowedValues(column); err != nil {
			return err
		}
		columns[column.Name], ordinals[column.Ordinal] = struct{}{}, struct{}{}
	}
	if err := validatePrimaryKeyDefinition(definition, columns); err != nil {
		return err
	}

	metadataNames := map[string]string{}
	if definition.PrimaryKeyName != "" {
		if err := registerSchemaMetadataName(metadataNames, "constraint", definition.PrimaryKeyName); err != nil {
			return err
		}
	}
	if err := canonicalizeForeignKeys(definition, columns, metadataNames); err != nil {
		return err
	}
	if err := canonicalizeConstraintsAndIndexes(definition, columns, metadataNames); err != nil {
		return err
	}
	sortSchemaMetadata(definition)
	return nil
}

func canonicalizeAllowedValues(column *ColumnDefinition) error {

	seen := map[string]struct{}{}
	for index := range column.AllowedValues {
		value := strings.TrimSpace(column.AllowedValues[index])
		if value == "" || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("column %q has an invalid allowed value", column.Name)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("column %q repeats allowed value %q", column.Name, value)
		}
		seen[value] = struct{}{}
		column.AllowedValues[index] = value
	}
	return nil
}

func validatePrimaryKeyDefinition(definition *RelationDefinition, columns map[string]struct{}) error {
	if err := validateColumnNames("primary key", definition.PrimaryKey, columns, true); err != nil {
		return err
	}
	definition.PrimaryKeyName = strings.TrimSpace(definition.PrimaryKeyName)
	if strings.ContainsRune(definition.PrimaryKeyName, '\x00') {
		return fmt.Errorf("primary key name is invalid")
	}
	if len(definition.PrimaryKey) == 0 && definition.PrimaryKeyName != "" {
		return fmt.Errorf("primary key name exists without primary key columns")
	}
	if len(definition.PrimaryKey) > 0 && definition.PrimaryKeyName == "" {
		definition.PrimaryKeyName = "__primary_key__"
	}
	normalizeNames(definition.PrimaryKey)
	return nil
}

func canonicalizeForeignKeys(definition *RelationDefinition, columns map[string]struct{}, metadataNames map[string]string) error {
	for index := range definition.ForeignKeys {
		fk := &definition.ForeignKeys[index]
		fk.Name, fk.ReferencedSchema, fk.ReferencedRelation = strings.TrimSpace(fk.Name), strings.TrimSpace(fk.ReferencedSchema), strings.TrimSpace(fk.ReferencedRelation)
		if fk.Name == "" || fk.ReferencedSchema == "" || fk.ReferencedRelation == "" || strings.ContainsRune(fk.Name+fk.ReferencedSchema+fk.ReferencedRelation, '\x00') || len(fk.Columns) != len(fk.ReferencedColumns) {
			return fmt.Errorf("foreign key %d has invalid identity or column cardinality", index)
		}
		if err := validateColumnNames("foreign key "+fk.Name, fk.Columns, columns, false); err != nil {
			return err
		}
		normalizeNames(fk.Columns)
		normalizeNames(fk.ReferencedColumns)
		fk.MatchType, fk.OnUpdate, fk.OnDelete = strings.TrimSpace(fk.MatchType), strings.TrimSpace(fk.OnUpdate), strings.TrimSpace(fk.OnDelete)
		if err := registerSchemaMetadataName(metadataNames, "constraint", fk.Name); err != nil {
			return err
		}
	}
	return nil
}

func canonicalizeConstraintsAndIndexes(definition *RelationDefinition, columns map[string]struct{}, metadataNames map[string]string) error {
	for index := range definition.UniqueConstraints {
		unique := &definition.UniqueConstraints[index]
		unique.Name = strings.TrimSpace(unique.Name)
		if unique.Name == "" || strings.ContainsRune(unique.Name, '\x00') {
			return fmt.Errorf("unique constraint requires a valid name")
		}
		if err := validateColumnNames("unique constraint "+unique.Name, unique.Columns, columns, false); err != nil {
			return err
		}
		normalizeNames(unique.Columns)
		if err := registerSchemaMetadataName(metadataNames, "constraint", unique.Name); err != nil {
			return err
		}
	}
	for index := range definition.CheckConstraints {
		check := &definition.CheckConstraints[index]
		check.Name, check.Expression = strings.TrimSpace(check.Name), strings.TrimSpace(check.Expression)
		if check.Name == "" || check.Expression == "" || strings.ContainsRune(check.Expression, '\x00') {
			return fmt.Errorf("check constraint requires a valid name and expression")
		}
		if err := registerSchemaMetadataName(metadataNames, "constraint", check.Name); err != nil {
			return err
		}
	}
	for index := range definition.Indexes {
		schemaIndex := &definition.Indexes[index]
		schemaIndex.Name, schemaIndex.Expression, schemaIndex.Predicate = strings.TrimSpace(schemaIndex.Name), strings.TrimSpace(schemaIndex.Expression), strings.TrimSpace(schemaIndex.Predicate)
		if schemaIndex.Name == "" || strings.ContainsRune(schemaIndex.Expression+schemaIndex.Predicate, '\x00') {
			return fmt.Errorf("index requires valid bounded metadata")
		}
		if err := validateColumnNames("index "+schemaIndex.Name, schemaIndex.Columns, columns, true); err != nil {
			return err
		}
		normalizeNames(schemaIndex.Columns)
		if len(schemaIndex.Columns) == 0 && schemaIndex.Expression == "" {
			return fmt.Errorf("index %s requires columns or an expression", schemaIndex.Name)
		}
		if err := registerSchemaMetadataName(metadataNames, "index", schemaIndex.Name); err != nil {
			return err
		}
	}
	return nil
}
