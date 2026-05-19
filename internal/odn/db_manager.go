package odn

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type DBManagerLLMClient interface {
	ChatRaw(ctx context.Context, req OllamaChatRequest) (OllamaChatResponse, error)
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

type DBManagerQueryResult struct {
	Question string
	SQL      string
	Rows     []MemorySQLRow
	Schema   []DBSchemaTable
	Answer   string
}

type dbManagerSQLPayload struct {
	SQL    string `json:"sql"`
	Answer string `json:"answer"`
}

var readOnlySQLPrefixRe = regexp.MustCompile(`(?is)^\s*(select|with)\b`)

func RunDBManagerQuery(ctx context.Context, question string, runner MemorySQLRunner, llm DBManagerLLMClient) (DBManagerQueryResult, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return DBManagerQueryResult{}, fmt.Errorf("question is required")
	}
	if runner == nil {
		return DBManagerQueryResult{}, fmt.Errorf("database runner is required")
	}
	if llm == nil {
		return DBManagerQueryResult{}, fmt.Errorf("llm client is required")
	}

	schema, err := InspectPostgresSchema(ctx, runner)
	if err != nil {
		return DBManagerQueryResult{}, err
	}
	resp, err := llm.ChatRaw(ctx, buildDBManagerRequest(question, schema))
	if err != nil {
		return DBManagerQueryResult{}, err
	}
	payload, err := parseDBManagerSQLPayload(resp.Content)
	if err != nil {
		return DBManagerQueryResult{}, err
	}
	sql := strings.TrimSpace(payload.SQL)
	if err := validateReadOnlyPostgresQuery(sql); err != nil {
		return DBManagerQueryResult{}, err
	}
	rows, err := runner.Query(ctx, sql)
	if err != nil {
		return DBManagerQueryResult{}, err
	}
	return DBManagerQueryResult{
		Question: question,
		SQL:      sql,
		Rows:     rows,
		Schema:   schema,
		Answer:   strings.TrimSpace(payload.Answer),
	}, nil
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

func buildDBManagerRequest(question string, schema []DBSchemaTable) OllamaChatRequest {
	blob, _ := json.Marshal(struct {
		Question string          `json:"question"`
		Schema   []DBSchemaTable `json:"schema"`
	}{
		Question: question,
		Schema:   schema,
	})
	return OllamaChatRequest{
		Messages: []OllamaMessage{
			{
				Role: "system",
				Content: strings.Join([]string{
					"Return JSON only.",
					"Schema: {\"sql\":\"read-only PostgreSQL query\",\"answer\":\"brief explanation of query intent\"}.",
					"You are the DB manager for memories, documents, research, and project history.",
					"Use the provided schema only; do not invent tables or columns.",
					"Generate one read-only PostgreSQL query that answers the user question.",
					"Only SELECT or WITH queries are allowed.",
					"Do not generate INSERT, UPDATE, DELETE, DROP, ALTER, CREATE, TRUNCATE, GRANT, REVOKE, VACUUM, COPY, CALL, or DO.",
					"Prefer explicit columns and LIMIT for exploratory searches.",
					"No markdown.",
				}, "\n"),
			},
			{Role: "user", Content: string(blob)},
		},
		Format: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"sql":    map[string]interface{}{"type": "string"},
				"answer": map[string]interface{}{"type": "string"},
			},
			"required": []string{"sql", "answer"},
		},
		Options: map[string]interface{}{"temperature": 0},
	}
}

func parseDBManagerSQLPayload(raw string) (dbManagerSQLPayload, error) {
	var payload dbManagerSQLPayload
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
		return payload, fmt.Errorf("parse db manager payload: %w", err)
	}
	if strings.TrimSpace(payload.SQL) == "" {
		return payload, fmt.Errorf("db manager payload sql is empty")
	}
	return payload, nil
}

func validateReadOnlyPostgresQuery(sql string) error {
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
