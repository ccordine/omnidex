package datasource

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	MaxSchemaRelations = 2048
	MaxSchemaColumns   = 20000
)

type catalogQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type inspectedRelation struct {
	oid        uint32
	definition RelationDefinition
}

func InspectCatalog(ctx context.Context, pool *pgxpool.Pool, sourceID, sourceName string) (SchemaSnapshot, error) {
	if pool == nil {
		return SchemaSnapshot{}, fmt.Errorf("inspect PostgreSQL catalog requires a pool")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return SchemaSnapshot{}, fmt.Errorf("begin schema inspection transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	snapshot, err := inspectCatalog(ctx, tx, sourceID, sourceName, time.Now().UTC())
	if err != nil {
		return SchemaSnapshot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SchemaSnapshot{}, fmt.Errorf("commit schema inspection transaction: %w", err)
	}
	return snapshot, nil
}

func inspectCatalog(ctx context.Context, queryer catalogQueryer, sourceID, sourceName string, capturedAt time.Time) (SchemaSnapshot, error) {
	relations, err := inspectRelations(ctx, queryer)
	if err != nil {
		return SchemaSnapshot{}, err
	}
	if err := inspectColumns(ctx, queryer, relations); err != nil {
		return SchemaSnapshot{}, err
	}
	if err := inspectConstraints(ctx, queryer, relations); err != nil {
		return SchemaSnapshot{}, err
	}
	if err := inspectIndexes(ctx, queryer, relations); err != nil {
		return SchemaSnapshot{}, err
	}
	definitions := make([]RelationDefinition, 0, len(relations))
	for _, relation := range relations {
		definitions = append(definitions, relation.definition)
	}
	return NewSchemaSnapshot(sourceID, sourceName, definitions, capturedAt)
}

func inspectRelations(ctx context.Context, queryer catalogQueryer) (map[uint32]*inspectedRelation, error) {
	rows, err := queryer.Query(ctx, `
SELECT c.oid::oid, n.nspname, c.relname, c.relkind::text,
       GREATEST(COALESCE(c.reltuples::bigint, 0), 0)
FROM pg_catalog.pg_class AS c
JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r', 'p', 'v', 'm', 'f')
  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
  AND n.nspname NOT LIKE 'pg_toast%'
  AND n.nspname NOT LIKE 'pg_temp_%'
  AND pg_catalog.has_schema_privilege(n.oid, 'USAGE')
  AND pg_catalog.has_table_privilege(c.oid, 'SELECT')
ORDER BY n.nspname, c.relname`)
	if err != nil {
		return nil, fmt.Errorf("inspect PostgreSQL relations: %w", err)
	}
	defer rows.Close()
	relations := map[uint32]*inspectedRelation{}
	for rows.Next() {
		var oid uint32
		var schema, name, kindCode string
		var estimate int64
		if err := rows.Scan(&oid, &schema, &name, &kindCode, &estimate); err != nil {
			return nil, fmt.Errorf("scan PostgreSQL relation: %w", err)
		}
		kind, err := relationKindFromPostgres(kindCode)
		if err != nil {
			return nil, err
		}
		if _, duplicate := relations[oid]; duplicate {
			return nil, fmt.Errorf("PostgreSQL catalog repeated relation OID %d", oid)
		}
		relations[oid] = &inspectedRelation{oid: oid, definition: RelationDefinition{Schema: schema, Name: name, Kind: kind, RowEstimate: estimate}}
		if len(relations) > MaxSchemaRelations {
			return nil, fmt.Errorf("PostgreSQL catalog exceeds %d relations", MaxSchemaRelations)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate PostgreSQL relations: %w", err)
	}
	if len(relations) == 0 {
		return nil, fmt.Errorf("PostgreSQL catalog contains no selectable user relations")
	}
	return relations, nil
}

func inspectColumns(ctx context.Context, queryer catalogQueryer, relations map[uint32]*inspectedRelation) error {
	rows, err := queryer.Query(ctx, `
SELECT a.attrelid::oid, a.attnum::integer, a.attname,
       pg_catalog.format_type(a.atttypid, a.atttypmod), t.typcategory::text,
       NOT a.attnotnull, a.attgenerated::text <> '', a.attidentity::text <> '',
       COALESCE(ARRAY(
         SELECT enum.enumlabel
         FROM pg_catalog.pg_enum AS enum
         WHERE enum.enumtypid = a.atttypid
         ORDER BY enum.enumsortorder
       ), ARRAY[]::name[])::text[]
FROM pg_catalog.pg_attribute AS a
JOIN pg_catalog.pg_type AS t ON t.oid = a.atttypid
JOIN pg_catalog.pg_class AS c ON c.oid = a.attrelid
JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
WHERE a.attnum > 0 AND NOT a.attisdropped
  AND c.relkind IN ('r', 'p', 'v', 'm', 'f')
  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
  AND n.nspname NOT LIKE 'pg_toast%'
  AND n.nspname NOT LIKE 'pg_temp_%'
  AND pg_catalog.has_schema_privilege(n.oid, 'USAGE')
  AND pg_catalog.has_table_privilege(c.oid, 'SELECT')
ORDER BY a.attrelid, a.attnum`)
	if err != nil {
		return fmt.Errorf("inspect PostgreSQL columns: %w", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var oid uint32
		var column ColumnDefinition
		var categoryCode string
		if err := rows.Scan(&oid, &column.Ordinal, &column.Name, &column.DataType, &categoryCode, &column.Nullable, &column.Generated, &column.Identity, &column.AllowedValues); err != nil {
			return fmt.Errorf("scan PostgreSQL column: %w", err)
		}
		relation, exists := relations[oid]
		if !exists {
			return fmt.Errorf("column references relation OID %d outside inspected catalog", oid)
		}
		column.TypeCategory = postgresTypeCategory(categoryCode, column.DataType)
		relation.definition.Columns = append(relation.definition.Columns, column)
		count++
		if count > MaxSchemaColumns {
			return fmt.Errorf("PostgreSQL catalog exceeds %d columns", MaxSchemaColumns)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate PostgreSQL columns: %w", err)
	}
	return nil
}

func relationKindFromPostgres(code string) (RelationKind, error) {
	switch code {
	case "r":
		return RelationTable, nil
	case "p":
		return RelationPartitionedTable, nil
	case "v":
		return RelationView, nil
	case "m":
		return RelationMaterializedView, nil
	case "f":
		return RelationForeignTable, nil
	default:
		return "", fmt.Errorf("unsupported PostgreSQL relation kind %q", code)
	}
}

func postgresTypeCategory(code, dataType string) ColumnTypeCategory {
	lowerType := strings.ToLower(dataType)
	if strings.Contains(lowerType, "uuid") {
		return TypeUUID
	}
	if strings.Contains(lowerType, "json") {
		return TypeJSON
	}
	if strings.Contains(lowerType, "bytea") {
		return TypeBinary
	}
	if strings.Contains(lowerType, "interval") {
		return TypeOther
	}
	if lowerType == "date" {
		return TypeDate
	}
	if strings.Contains(lowerType, "timestamp") || strings.Contains(lowerType, "time ") || lowerType == "time" {
		return TypeTemporal
	}
	switch code {
	case "N":
		if strings.Contains(lowerType, "int") {
			return TypeInteger
		}
		return TypeDecimal
	case "S", "V":
		return TypeText
	case "B":
		return TypeBoolean
	case "E":
		return TypeText
	case "D":
		return TypeDate
	case "T":
		return TypeTemporal
	default:
		return TypeOther
	}
}
