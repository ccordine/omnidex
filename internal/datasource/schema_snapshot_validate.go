package datasource

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"time"
)

func (snapshot SchemaSnapshot) ValidateIntegrity() error {
	if snapshot.Schema != SchemaSnapshotV1 || snapshot.Driver != DriverPostgres {
		return fmt.Errorf("schema snapshot has unsupported schema or driver")
	}
	if snapshot.SourceID == "" || snapshot.SourceName == "" || snapshot.SourceName != strings.TrimSpace(snapshot.SourceName) || snapshot.CapturedAt.IsZero() || snapshot.CapturedAt.Location() != time.UTC {
		return fmt.Errorf("schema snapshot authority is incomplete")
	}
	fingerprint, err := hex.DecodeString(snapshot.Fingerprint)
	if err != nil || len(fingerprint) != 32 || snapshot.Fingerprint != hex.EncodeToString(fingerprint) {
		return fmt.Errorf("schema snapshot fingerprint is invalid")
	}
	definitions, err := definitionsFromSnapshot(snapshot)
	if err != nil {
		return err
	}
	rebuilt, err := NewSchemaSnapshot(snapshot.SourceID, snapshot.SourceName, definitions, snapshot.CapturedAt)
	if err != nil {
		return fmt.Errorf("rebuild schema snapshot integrity: %w", err)
	}
	if !reflect.DeepEqual(snapshot, rebuilt) {
		return fmt.Errorf("schema snapshot metadata, opaque IDs, or fingerprint are not canonical")
	}
	return nil
}

func definitionsFromSnapshot(snapshot SchemaSnapshot) ([]RelationDefinition, error) {
	relations := map[string]SchemaRelation{}
	columns := map[string]struct {
		relationID string
		column     SchemaColumn
	}{}
	for _, relation := range snapshot.Relations {
		if relation.ID == "" {
			return nil, fmt.Errorf("schema snapshot contains a blank relation ID")
		}
		if _, duplicate := relations[relation.ID]; duplicate {
			return nil, fmt.Errorf("schema snapshot repeats relation ID %q", relation.ID)
		}
		relations[relation.ID] = relation
		for _, column := range relation.Columns {
			if column.ID == "" {
				return nil, fmt.Errorf("relation %q contains a blank column ID", relation.ID)
			}
			if _, duplicate := columns[column.ID]; duplicate {
				return nil, fmt.Errorf("schema snapshot repeats column ID %q", column.ID)
			}
			columns[column.ID] = struct {
				relationID string
				column     SchemaColumn
			}{relationID: relation.ID, column: column}
		}
	}
	definitions := make([]RelationDefinition, len(snapshot.Relations))
	for index, relation := range snapshot.Relations {
		definition := RelationDefinition{
			Schema: relation.Schema, Name: relation.Name, Kind: relation.Kind, RowEstimate: relation.RowEstimate,
			PrimaryKeyName: relation.PrimaryKeyName,
		}
		for _, column := range relation.Columns {
			definition.Columns = append(definition.Columns, ColumnDefinition{
				Name: column.Name, Ordinal: column.Ordinal, DataType: column.DataType,
				TypeCategory: column.TypeCategory, Nullable: column.Nullable, Generated: column.Generated,
				Identity: column.Identity, AllowedValues: append([]string(nil), column.AllowedValues...),
			})
		}
		var err error
		definition.PrimaryKey, err = snapshotColumnNames(relation.ID, relation.PrimaryKey, columns)
		if err != nil {
			return nil, err
		}
		for _, foreignKey := range relation.ForeignKeys {
			target, exists := relations[foreignKey.ReferencedRelationID]
			if !exists {
				return nil, fmt.Errorf("foreign key %q references unknown relation ID %q", foreignKey.ID, foreignKey.ReferencedRelationID)
			}
			localNames, err := snapshotColumnNames(relation.ID, foreignKey.ColumnIDs, columns)
			if err != nil {
				return nil, err
			}
			targetNames, err := snapshotColumnNames(target.ID, foreignKey.ReferencedColumnIDs, columns)
			if err != nil {
				return nil, err
			}
			definition.ForeignKeys = append(definition.ForeignKeys, ForeignKeyDefinition{
				Name: foreignKey.Name, Columns: localNames, ReferencedSchema: target.Schema,
				ReferencedRelation: target.Name, ReferencedColumns: targetNames, MatchType: foreignKey.MatchType,
				OnUpdate: foreignKey.OnUpdate, OnDelete: foreignKey.OnDelete, Deferrable: foreignKey.Deferrable,
			})
		}
		for _, unique := range relation.UniqueConstraints {
			names, err := snapshotColumnNames(relation.ID, unique.ColumnIDs, columns)
			if err != nil {
				return nil, err
			}
			definition.UniqueConstraints = append(definition.UniqueConstraints, UniqueConstraintDefinition{Name: unique.Name, Columns: names})
		}
		for _, check := range relation.CheckConstraints {
			definition.CheckConstraints = append(definition.CheckConstraints, CheckConstraintDefinition{Name: check.Name, Expression: check.Expression})
		}
		for _, schemaIndex := range relation.Indexes {
			names, err := snapshotColumnNames(relation.ID, schemaIndex.ColumnIDs, columns)
			if err != nil {
				return nil, err
			}
			definition.Indexes = append(definition.Indexes, IndexDefinition{Name: schemaIndex.Name, Columns: names, Unique: schemaIndex.Unique, Primary: schemaIndex.Primary, Expression: schemaIndex.Expression, Predicate: schemaIndex.Predicate})
		}
		definitions[index] = definition
	}
	return definitions, nil
}

func snapshotColumnNames(
	relationID string,
	ids []string,
	columns map[string]struct {
		relationID string
		column     SchemaColumn
	},
) ([]string, error) {
	names := make([]string, len(ids))
	for index, id := range ids {
		resolved, exists := columns[id]
		if !exists || resolved.relationID != relationID {
			return nil, fmt.Errorf("column ID %q is not owned by relation %q", id, relationID)
		}
		names[index] = resolved.column.Name
	}
	return names, nil
}
