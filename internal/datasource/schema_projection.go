package datasource

import (
	"encoding/json"
	"fmt"
)

const (
	IntentSchemaProjectionV1       = "omnidex.intent-schema-projection.v1"
	MaxProjectedRelations          = 12
	MaxProjectedColumns            = 120
	MaxProjectedEnumValues         = 512
	MaxIntentSchemaProjectionBytes = 32 * 1024
)

type IntentColumnProjection struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	TypeCategory  ColumnTypeCategory `json:"type_category"`
	Nullable      bool               `json:"nullable"`
	AllowedValues []string           `json:"allowed_values,omitempty"`
}

type IntentForeignKeyProjection struct {
	ID                   string   `json:"id"`
	ColumnIDs            []string `json:"column_ids"`
	ReferencedRelationID string   `json:"referenced_relation_id"`
	ReferencedColumnIDs  []string `json:"referenced_column_ids"`
}

type IntentRelationProjection struct {
	ID          string                       `json:"id"`
	SchemaName  string                       `json:"schema_name"`
	Name        string                       `json:"name"`
	Kind        RelationKind                 `json:"kind"`
	Columns     []IntentColumnProjection     `json:"columns"`
	PrimaryKey  []string                     `json:"primary_key"`
	ForeignKeys []IntentForeignKeyProjection `json:"foreign_keys"`
}

type IntentSchemaProjection struct {
	Schema            string                     `json:"schema"`
	SourceID          string                     `json:"source_id"`
	SchemaFingerprint string                     `json:"schema_fingerprint"`
	Relations         []IntentRelationProjection `json:"relations"`
}

func ProjectSchemaForIntent(snapshot SchemaSnapshot, relationIDs []string) (IntentSchemaProjection, error) {
	if err := snapshot.ValidateIntegrity(); err != nil {
		return IntentSchemaProjection{}, fmt.Errorf("intent schema projection requires a valid snapshot: %w", err)
	}
	if len(relationIDs) == 0 || len(relationIDs) > MaxProjectedRelations {
		return IntentSchemaProjection{}, fmt.Errorf("intent schema projection requires 1..%d relation IDs", MaxProjectedRelations)
	}
	projection := IntentSchemaProjection{Schema: IntentSchemaProjectionV1, SourceID: snapshot.SourceID, SchemaFingerprint: snapshot.Fingerprint}
	seen := map[string]struct{}{}
	columnCount := 0
	enumValueCount := 0
	for _, relationID := range relationIDs {
		if _, duplicate := seen[relationID]; duplicate {
			return IntentSchemaProjection{}, fmt.Errorf("intent schema projection repeats relation ID %q", relationID)
		}
		seen[relationID] = struct{}{}
		relation, err := snapshot.Relation(relationID)
		if err != nil {
			return IntentSchemaProjection{}, err
		}
		columnCount += len(relation.Columns)
		if columnCount > MaxProjectedColumns {
			return IntentSchemaProjection{}, fmt.Errorf("intent schema projection exceeds %d columns", MaxProjectedColumns)
		}
		projected := IntentRelationProjection{ID: relation.ID, SchemaName: relation.Schema, Name: relation.Name, Kind: relation.Kind, PrimaryKey: append([]string(nil), relation.PrimaryKey...)}
		for _, column := range relation.Columns {
			enumValueCount += len(column.AllowedValues)
			if enumValueCount > MaxProjectedEnumValues {
				return IntentSchemaProjection{}, fmt.Errorf("intent schema projection exceeds %d enum values", MaxProjectedEnumValues)
			}
			projected.Columns = append(projected.Columns, IntentColumnProjection{ID: column.ID, Name: column.Name, TypeCategory: column.TypeCategory, Nullable: column.Nullable, AllowedValues: append([]string(nil), column.AllowedValues...)})
		}
		for _, fk := range relation.ForeignKeys {
			projected.ForeignKeys = append(projected.ForeignKeys, IntentForeignKeyProjection{ID: fk.ID, ColumnIDs: append([]string(nil), fk.ColumnIDs...), ReferencedRelationID: fk.ReferencedRelationID, ReferencedColumnIDs: append([]string(nil), fk.ReferencedColumnIDs...)})
		}
		projection.Relations = append(projection.Relations, projected)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		return IntentSchemaProjection{}, fmt.Errorf("encode intent schema projection: %w", err)
	}
	if len(encoded) > MaxIntentSchemaProjectionBytes {
		return IntentSchemaProjection{}, fmt.Errorf("intent schema projection exceeds %d bytes", MaxIntentSchemaProjectionBytes)
	}
	return projection, nil
}
