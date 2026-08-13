package omni

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

type DBManagerMemoryWriter interface {
	AddMemory(ctx context.Context, input model.MemoryInput) (MemoryRecord, error)
}

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

type DBSchemaMemorySnapshot struct {
	Label       string
	Fingerprint string
	Tables      []DBSchemaTable
	Content     string
	Tags        []string
}

var readOnlySQLPrefixRe = regexp.MustCompile(`(?is)^\s*(select|with)\b`)

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

func BuildDBSchemaMemorySnapshot(label string, schema []DBSchemaTable) DBSchemaMemorySnapshot {
	label = strings.TrimSpace(label)
	if label == "" {
		label = "postgres"
	}
	normalized := normalizeDBSchemaTables(schema)
	fingerprint := fingerprintDBSchema(normalized)
	content := formatDBSchemaMemoryContent(label, fingerprint, normalized)
	return DBSchemaMemorySnapshot{
		Label:       label,
		Fingerprint: fingerprint,
		Tables:      normalized,
		Content:     content,
		Tags:        dbSchemaMemoryTags(label, fingerprint, normalized),
	}
}

func StoreDBSchemaMemorySnapshot(ctx context.Context, writer DBManagerMemoryWriter, scope model.MemoryScope, label string, schema []DBSchemaTable) (MemoryRecord, DBSchemaMemorySnapshot, error) {
	if writer == nil {
		return MemoryRecord{}, DBSchemaMemorySnapshot{}, fmt.Errorf("memory writer is required")
	}
	snapshot := BuildDBSchemaMemorySnapshot(label, schema)
	input := model.MemoryInput{
		Scope: scope, Source: "db_schema_specialist", Kind: model.MemoryKindReference,
		Content: snapshot.Content, Tags: snapshot.Tags,
		Categories: []model.MemoryCategory{model.MemoryCategoryDatabase},
	}
	if err := input.Validate(); err != nil {
		return MemoryRecord{}, snapshot, err
	}
	record, err := writer.AddMemory(ctx, input)
	if err != nil {
		return MemoryRecord{}, snapshot, err
	}
	return record, snapshot, nil
}

func normalizeDBSchemaTables(schema []DBSchemaTable) []DBSchemaTable {
	out := make([]DBSchemaTable, 0, len(schema))
	for _, table := range schema {
		clean := DBSchemaTable{
			Schema:  strings.TrimSpace(table.Schema),
			Name:    strings.TrimSpace(table.Name),
			Columns: make([]DBSchemaColumn, 0, len(table.Columns)),
		}
		if clean.Schema == "" || clean.Name == "" {
			continue
		}
		for _, column := range table.Columns {
			name := strings.TrimSpace(column.Name)
			dataType := strings.TrimSpace(column.DataType)
			if name == "" {
				continue
			}
			clean.Columns = append(clean.Columns, DBSchemaColumn{Name: name, DataType: dataType, Nullable: column.Nullable})
		}
		sort.SliceStable(clean.Columns, func(i, j int) bool {
			return clean.Columns[i].Name < clean.Columns[j].Name
		})
		out = append(out, clean)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := out[i].Schema + "." + out[i].Name
		right := out[j].Schema + "." + out[j].Name
		return left < right
	})
	return out
}

func fingerprintDBSchema(schema []DBSchemaTable) string {
	raw, _ := json.Marshal(schema)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func formatDBSchemaMemoryContent(label, fingerprint string, schema []DBSchemaTable) string {
	lines := []string{
		"Database schema memory",
		"label=" + strings.TrimSpace(label),
		"schema_fingerprint=" + strings.TrimSpace(fingerprint),
		fmt.Sprintf("table_count=%d", len(schema)),
		"",
		"Use this schema only for read-only SQL unless a task explicitly asks for migration design.",
		"Re-inspect information_schema before relying on this memory; if the fingerprint differs, treat this memory as stale.",
		"",
		"Tables:",
	}
	for _, table := range schema {
		lines = append(lines, fmt.Sprintf("- %s.%s", table.Schema, table.Name))
		for _, column := range table.Columns {
			nullable := "not null"
			if column.Nullable {
				nullable = "nullable"
			}
			lines = append(lines, fmt.Sprintf("  - %s %s %s", column.Name, defaultString(column.DataType, "unknown"), nullable))
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func dbSchemaTagToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	clean := strings.Trim(b.String(), "-")
	if clean == "" {
		return "unknown"
	}
	return clean
}

func dbSchemaMemoryTags(label, fingerprint string, schema []DBSchemaTable) []string {
	tags := []string{"db-schema", "schema-memory", "schema:" + dbSchemaTagToken(label)}
	if len(fingerprint) >= 12 {
		tags = append(tags, "schema-fingerprint:"+fingerprint[:12])
	}
	for _, table := range schema {
		tags = append(tags, "table:"+dbSchemaTagToken(table.Schema+"-"+table.Name))
	}
	sort.Strings(tags)
	return tags
}

func ValidateReadOnlyPostgresQuery(sql string) error {
	clean := strings.TrimSpace(sql)
	if clean == "" {
		return fmt.Errorf("sql is empty")
	}
	if !readOnlySQLPrefixRe.MatchString(clean) {
		return fmt.Errorf("only SELECT or WITH queries are allowed")
	}
	trimmed := strings.TrimRight(clean, " \t\r\n;")
	if strings.Contains(trimmed, ";") {
		return fmt.Errorf("multiple statements are not allowed")
	}
	lower := strings.ToLower(clean)
	for _, forbidden := range []string{
		" insert ", " update ", " delete ", " drop ", " alter ", " create ",
		" truncate ", " grant ", " revoke ", " vacuum ", " copy ", " call ", " do ",
		";insert", ";update", ";delete", ";drop", ";alter", ";create",
	} {
		if strings.Contains(" "+lower+" ", forbidden) {
			return fmt.Errorf("write or administrative SQL is not allowed")
		}
	}
	return nil
}
