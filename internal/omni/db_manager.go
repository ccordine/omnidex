package omni

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type DBSchemaTable struct {
	Schema  string           `json:"schema"`
	Name    string           `json:"name"`
	Columns []DBSchemaColumn `json:"columns"`
}

type DBSchemaColumn struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
	Nullable bool   `json:"nullable"`
}

func InspectPostgresSchema(ctx context.Context, runner MemorySQLRunner) ([]DBSchemaTable, error) {
	if runner == nil {
		return nil, fmt.Errorf("database runner is required")
	}
	rows, err := runner.Query(ctx, `
		SELECT table_schema, table_name, column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
		ORDER BY table_schema, table_name, ordinal_position
	`)
	if err != nil {
		return nil, err
	}
	tableIndex := map[string]int{}
	tables := []DBSchemaTable{}
	for _, row := range rows {
		schemaName := stringFromAny(row["table_schema"])
		tableName := stringFromAny(row["table_name"])
		if schemaName == "" || tableName == "" {
			continue
		}
		key := schemaName + "." + tableName
		index, ok := tableIndex[key]
		if !ok {
			index = len(tables)
			tableIndex[key] = index
			tables = append(tables, DBSchemaTable{Schema: schemaName, Name: tableName})
		}
		tables[index].Columns = append(tables[index].Columns, DBSchemaColumn{
			Name:     stringFromAny(row["column_name"]),
			DataType: stringFromAny(row["data_type"]),
			Nullable: strings.EqualFold(stringFromAny(row["is_nullable"]), "YES"),
		})
	}
	sort.Slice(tables, func(i, j int) bool {
		left := tables[i].Schema + "." + tables[i].Name
		right := tables[j].Schema + "." + tables[j].Name
		return left < right
	})
	return tables, nil
}
