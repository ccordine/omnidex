package datasource

import (
	"context"
	"fmt"
	"strings"
)

func inspectConstraints(ctx context.Context, queryer catalogQueryer, relations map[uint32]*inspectedRelation) error {
	rows, err := queryer.Query(ctx, `
SELECT con.conrelid::oid, con.contype::text, con.conname,
       COALESCE(ARRAY(
         SELECT a.attname
         FROM unnest(con.conkey) WITH ORDINALITY AS keys(attnum, position)
         JOIN pg_catalog.pg_attribute AS a ON a.attrelid = con.conrelid AND a.attnum = keys.attnum
         ORDER BY keys.position
       ), ARRAY[]::name[])::text[],
       con.confrelid::oid,
       COALESCE(ARRAY(
         SELECT a.attname
         FROM unnest(con.confkey) WITH ORDINALITY AS keys(attnum, position)
         JOIN pg_catalog.pg_attribute AS a ON a.attrelid = con.confrelid AND a.attnum = keys.attnum
         ORDER BY keys.position
       ), ARRAY[]::name[])::text[],
       pg_catalog.pg_get_constraintdef(con.oid, true), con.condeferrable,
       con.confmatchtype::text, con.confupdtype::text, con.confdeltype::text
FROM pg_catalog.pg_constraint AS con
WHERE con.contype IN ('p', 'u', 'f', 'c')
ORDER BY con.conrelid, con.contype, con.conname`)
	if err != nil {
		return fmt.Errorf("inspect PostgreSQL constraints: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var relationOID, referencedOID uint32
		var kind, name, definition, matchCode, updateCode, deleteCode string
		var columns, referencedColumns []string
		var deferrable bool
		if err := rows.Scan(&relationOID, &kind, &name, &columns, &referencedOID, &referencedColumns, &definition, &deferrable, &matchCode, &updateCode, &deleteCode); err != nil {
			return fmt.Errorf("scan PostgreSQL constraint: %w", err)
		}
		relation, exists := relations[relationOID]
		if !exists {
			continue
		}
		switch kind {
		case "p":
			relation.definition.PrimaryKeyName = name
			relation.definition.PrimaryKey = append([]string(nil), columns...)
		case "u":
			relation.definition.UniqueConstraints = append(relation.definition.UniqueConstraints, UniqueConstraintDefinition{Name: name, Columns: columns})
		case "c":
			relation.definition.CheckConstraints = append(relation.definition.CheckConstraints, CheckConstraintDefinition{Name: name, Expression: definition})
		case "f":
			referenced, exists := relations[referencedOID]
			if !exists {
				return fmt.Errorf("foreign key %q references inaccessible relation OID %d", name, referencedOID)
			}
			relation.definition.ForeignKeys = append(relation.definition.ForeignKeys, ForeignKeyDefinition{
				Name: name, Columns: columns, ReferencedSchema: referenced.definition.Schema,
				ReferencedRelation: referenced.definition.Name, ReferencedColumns: referencedColumns,
				MatchType: postgresMatchType(matchCode), OnUpdate: postgresAction(updateCode),
				OnDelete: postgresAction(deleteCode), Deferrable: deferrable,
			})
		default:
			return fmt.Errorf("unsupported PostgreSQL constraint kind %q", kind)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate PostgreSQL constraints: %w", err)
	}
	return nil
}

func inspectIndexes(ctx context.Context, queryer catalogQueryer, relations map[uint32]*inspectedRelation) error {
	rows, err := queryer.Query(ctx, `
SELECT idx.indrelid::oid, index_class.relname, idx.indisunique, idx.indisprimary,
       COALESCE(ARRAY(
         SELECT attribute.attname
         FROM unnest(idx.indkey) WITH ORDINALITY AS keys(attnum, position)
         JOIN pg_catalog.pg_attribute AS attribute
           ON attribute.attrelid = idx.indrelid AND attribute.attnum = keys.attnum
         WHERE keys.attnum > 0
         ORDER BY keys.position
       ), ARRAY[]::name[])::text[],
       CASE WHEN 0 = ANY(idx.indkey) THEN pg_catalog.pg_get_indexdef(idx.indexrelid) ELSE '' END,
       COALESCE(pg_catalog.pg_get_expr(idx.indpred, idx.indrelid), '')
FROM pg_catalog.pg_index AS idx
JOIN pg_catalog.pg_class AS index_class ON index_class.oid = idx.indexrelid
ORDER BY idx.indrelid, index_class.relname`)
	if err != nil {
		return fmt.Errorf("inspect PostgreSQL indexes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var relationOID uint32
		var index IndexDefinition
		if err := rows.Scan(&relationOID, &index.Name, &index.Unique, &index.Primary, &index.Columns, &index.Expression, &index.Predicate); err != nil {
			return fmt.Errorf("scan PostgreSQL index: %w", err)
		}
		if relation, exists := relations[relationOID]; exists {
			relation.definition.Indexes = append(relation.definition.Indexes, index)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate PostgreSQL indexes: %w", err)
	}
	return nil
}

func postgresMatchType(code string) string {
	switch code {
	case "f":
		return "full"
	case "p":
		return "partial"
	case "s", "":
		return "simple"
	default:
		return "unknown_" + strings.TrimSpace(code)
	}
}

func postgresAction(code string) string {
	switch code {
	case "a":
		return "no_action"
	case "r":
		return "restrict"
	case "c":
		return "cascade"
	case "n":
		return "set_null"
	case "d":
		return "set_default"
	case "":
		return ""
	default:
		return "unknown_" + strings.TrimSpace(code)
	}
}
