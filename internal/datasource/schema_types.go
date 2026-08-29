package datasource

import (
	"fmt"
	"strings"
	"time"
)

const SchemaSnapshotV1 = "omnidex.datasource-schema.v1"

type RelationKind string

const (
	RelationTable            RelationKind = "table"
	RelationPartitionedTable RelationKind = "partitioned_table"
	RelationView             RelationKind = "view"
	RelationMaterializedView RelationKind = "materialized_view"
	RelationForeignTable     RelationKind = "foreign_table"
)

type ColumnTypeCategory string

const (
	TypeInteger  ColumnTypeCategory = "integer"
	TypeDecimal  ColumnTypeCategory = "decimal"
	TypeText     ColumnTypeCategory = "text"
	TypeBoolean  ColumnTypeCategory = "boolean"
	TypeTemporal ColumnTypeCategory = "temporal"
	TypeDate     ColumnTypeCategory = "date"
	TypeUUID     ColumnTypeCategory = "uuid"
	TypeJSON     ColumnTypeCategory = "json"
	TypeBinary   ColumnTypeCategory = "binary"
	TypeOther    ColumnTypeCategory = "other"
)

type ColumnDefinition struct {
	Name          string
	Ordinal       int
	DataType      string
	TypeCategory  ColumnTypeCategory
	Nullable      bool
	Generated     bool
	Identity      bool
	AllowedValues []string
}

type ForeignKeyDefinition struct {
	Name               string
	Columns            []string
	ReferencedSchema   string
	ReferencedRelation string
	ReferencedColumns  []string
	MatchType          string
	OnUpdate           string
	OnDelete           string
	Deferrable         bool
}

type UniqueConstraintDefinition struct {
	Name    string
	Columns []string
}

type CheckConstraintDefinition struct {
	Name       string
	Expression string
}

type IndexDefinition struct {
	Name       string
	Columns    []string
	Unique     bool
	Primary    bool
	Expression string
	Predicate  string
}

type RelationDefinition struct {
	Schema            string
	Name              string
	Kind              RelationKind
	RowEstimate       int64
	Columns           []ColumnDefinition
	PrimaryKeyName    string
	PrimaryKey        []string
	ForeignKeys       []ForeignKeyDefinition
	UniqueConstraints []UniqueConstraintDefinition
	CheckConstraints  []CheckConstraintDefinition
	Indexes           []IndexDefinition
}

type SchemaColumn struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Ordinal       int                `json:"ordinal"`
	DataType      string             `json:"data_type"`
	TypeCategory  ColumnTypeCategory `json:"type_category"`
	Nullable      bool               `json:"nullable"`
	Generated     bool               `json:"generated"`
	Identity      bool               `json:"identity"`
	AllowedValues []string           `json:"allowed_values,omitempty"`
}

type SchemaForeignKey struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	ColumnIDs            []string `json:"column_ids"`
	ReferencedRelationID string   `json:"referenced_relation_id"`
	ReferencedColumnIDs  []string `json:"referenced_column_ids"`
	MatchType            string   `json:"match_type,omitempty"`
	OnUpdate             string   `json:"on_update,omitempty"`
	OnDelete             string   `json:"on_delete,omitempty"`
	Deferrable           bool     `json:"deferrable"`
}

type SchemaUniqueConstraint struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	ColumnIDs []string `json:"column_ids"`
}

type SchemaCheckConstraint struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Expression string `json:"expression"`
}

type SchemaIndex struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	ColumnIDs  []string `json:"column_ids,omitempty"`
	Unique     bool     `json:"unique"`
	Primary    bool     `json:"primary"`
	Expression string   `json:"expression,omitempty"`
	Predicate  string   `json:"predicate,omitempty"`
}

type SchemaRelation struct {
	ID                string                   `json:"id"`
	Schema            string                   `json:"schema"`
	Name              string                   `json:"name"`
	Kind              RelationKind             `json:"kind"`
	RowEstimate       int64                    `json:"row_estimate"`
	Columns           []SchemaColumn           `json:"columns"`
	PrimaryKeyID      string                   `json:"primary_key_id,omitempty"`
	PrimaryKeyName    string                   `json:"primary_key_name,omitempty"`
	PrimaryKey        []string                 `json:"primary_key"`
	ForeignKeys       []SchemaForeignKey       `json:"foreign_keys"`
	UniqueConstraints []SchemaUniqueConstraint `json:"unique_constraints"`
	CheckConstraints  []SchemaCheckConstraint  `json:"check_constraints"`
	Indexes           []SchemaIndex            `json:"indexes"`
}

type SchemaSnapshot struct {
	Schema      string           `json:"schema"`
	SourceID    string           `json:"source_id"`
	SourceName  string           `json:"source_name"`
	Driver      string           `json:"driver"`
	Fingerprint string           `json:"fingerprint"`
	CapturedAt  time.Time        `json:"captured_at"`
	Relations   []SchemaRelation `json:"relations"`
}

func (snapshot SchemaSnapshot) Relation(id string) (SchemaRelation, error) {
	for _, relation := range snapshot.Relations {
		if relation.ID == id {
			return relation, nil
		}
	}
	return SchemaRelation{}, fmt.Errorf("unknown relation ID %q for schema fingerprint %q", id, snapshot.Fingerprint)
}

func (snapshot SchemaSnapshot) Column(id string) (SchemaRelation, SchemaColumn, error) {
	for _, relation := range snapshot.Relations {
		for _, column := range relation.Columns {
			if column.ID == id {
				return relation, column, nil
			}
		}
	}
	return SchemaRelation{}, SchemaColumn{}, fmt.Errorf("unknown column ID %q for schema fingerprint %q", id, snapshot.Fingerprint)
}

func relationDefinitionKey(schema, name string) string {
	return strings.TrimSpace(schema) + "." + strings.TrimSpace(name)
}
