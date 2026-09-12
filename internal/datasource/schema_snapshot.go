package datasource

import (
	"fmt"
	"strings"
	"time"
)

func NewSchemaSnapshot(
	sourceID string,
	sourceName string,
	definitions []RelationDefinition,
	capturedAt time.Time,
) (SchemaSnapshot, error) {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" || len(sourceID) > 256 || strings.ContainsRune(sourceID, '\x00') {
		return SchemaSnapshot{}, fmt.Errorf("schema snapshot requires a non-blank source ID")
	}
	if strings.TrimSpace(sourceName) == "" || len(strings.TrimSpace(sourceName)) > 256 || strings.ContainsRune(sourceName, '\x00') {
		return SchemaSnapshot{}, fmt.Errorf("schema snapshot requires a non-blank source name")
	}
	if capturedAt.IsZero() {
		return SchemaSnapshot{}, fmt.Errorf("schema snapshot requires a capture time")
	}
	canonical, err := canonicalizeRelationDefinitions(definitions)
	if err != nil {
		return SchemaSnapshot{}, err
	}
	relations, err := resolveSchemaDefinitions(canonical)
	if err != nil {
		return SchemaSnapshot{}, err
	}
	return SchemaSnapshot{
		Schema: SchemaSnapshotV1, SourceID: sourceID, SourceName: strings.TrimSpace(sourceName),
		Driver: DriverPostgres, CapturedAt: capturedAt.UTC(), Relations: relations,
	}, nil
}

func resolveSchemaDefinitions(definitions []RelationDefinition) ([]SchemaRelation, error) {
	relations := make([]SchemaRelation, len(definitions))
	relationIDs := map[string]string{}
	columnIDs := map[string]map[string]string{}
	for index, definition := range definitions {
		key := relationDefinitionKey(definition.Schema, definition.Name)
		relationID := fmt.Sprintf("rel_%d", index+1)
		relationIDs[key] = relationID
		columnIDs[key] = map[string]string{}
		relation := SchemaRelation{ID: relationID, Schema: definition.Schema, Name: definition.Name, Kind: definition.Kind, RowEstimate: definition.RowEstimate}
		for columnIndex, column := range definition.Columns {
			columnID := fmt.Sprintf("%s_col_%d", relationID, columnIndex+1)
			columnIDs[key][column.Name] = columnID
			relation.Columns = append(relation.Columns, SchemaColumn{ID: columnID, Name: column.Name, Ordinal: column.Ordinal, DataType: column.DataType, TypeCategory: column.TypeCategory, Nullable: column.Nullable, Generated: column.Generated, Identity: column.Identity, AllowedValues: append([]string(nil), column.AllowedValues...)})
		}
		relations[index] = relation
	}
	for index, definition := range definitions {
		key := relationDefinitionKey(definition.Schema, definition.Name)
		relation := &relations[index]
		relation.PrimaryKey = resolveColumnNames(columnIDs[key], definition.PrimaryKey)
		if definition.PrimaryKeyName != "" {
			relation.PrimaryKeyID = relation.ID + "_pk"
			relation.PrimaryKeyName = definition.PrimaryKeyName
		}
		for foreignKeyIndex, fk := range definition.ForeignKeys {
			targetKey := relationDefinitionKey(fk.ReferencedSchema, fk.ReferencedRelation)
			targetRelationID, exists := relationIDs[targetKey]
			if !exists {
				return nil, fmt.Errorf("foreign key %s references unknown relation %q", fk.Name, targetKey)
			}
			targetColumns := columnIDs[targetKey]
			if err := validateColumnNames("foreign key "+fk.Name+" referenced", fk.ReferencedColumns, stringSet(targetColumns), false); err != nil {
				return nil, err
			}
			relation.ForeignKeys = append(relation.ForeignKeys, SchemaForeignKey{ID: fmt.Sprintf("%s_fk_%d", relation.ID, foreignKeyIndex+1), Name: fk.Name, ColumnIDs: resolveColumnNames(columnIDs[key], fk.Columns), ReferencedRelationID: targetRelationID, ReferencedColumnIDs: resolveColumnNames(targetColumns, fk.ReferencedColumns), MatchType: fk.MatchType, OnUpdate: fk.OnUpdate, OnDelete: fk.OnDelete, Deferrable: fk.Deferrable})
		}
		for uniqueIndex, unique := range definition.UniqueConstraints {
			relation.UniqueConstraints = append(relation.UniqueConstraints, SchemaUniqueConstraint{ID: fmt.Sprintf("%s_unique_%d", relation.ID, uniqueIndex+1), Name: unique.Name, ColumnIDs: resolveColumnNames(columnIDs[key], unique.Columns)})
		}
		for checkIndex, check := range definition.CheckConstraints {
			relation.CheckConstraints = append(relation.CheckConstraints, SchemaCheckConstraint{ID: fmt.Sprintf("%s_check_%d", relation.ID, checkIndex+1), Name: check.Name, Expression: check.Expression})
		}
		for indexIndex, schemaIndex := range definition.Indexes {
			relation.Indexes = append(relation.Indexes, SchemaIndex{ID: fmt.Sprintf("%s_index_%d", relation.ID, indexIndex+1), Name: schemaIndex.Name, ColumnIDs: resolveColumnNames(columnIDs[key], schemaIndex.Columns), Unique: schemaIndex.Unique, Primary: schemaIndex.Primary, Expression: schemaIndex.Expression, Predicate: schemaIndex.Predicate})
		}
	}
	return relations, nil
}

func resolveColumnNames(ids map[string]string, names []string) []string {
	resolved := make([]string, len(names))
	for index, name := range names {
		resolved[index] = ids[name]
	}
	return resolved
}

func stringSet(values map[string]string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for value := range values {
		result[value] = struct{}{}
	}
	return result
}
