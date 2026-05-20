package omni

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestDBManagerScansSchemaAndRunsLLMGeneratedMemoryQuery(t *testing.T) {
	runner := newFakeDBManagerRunner()
	runner.schemaRows = []MemorySQLRow{
		schemaRow("public", "memory_chunks", "id", "bigint", "NO"),
		schemaRow("public", "memory_chunks", "agent_id", "text", "NO"),
		schemaRow("public", "memory_chunks", "kind", "text", "NO"),
		schemaRow("public", "memory_chunks", "content", "text", "NO"),
		schemaRow("public", "memory_chunks", "created_at", "timestamp with time zone", "NO"),
		schemaRow("public", "tags", "id", "bigint", "NO"),
		schemaRow("public", "tags", "name", "text", "NO"),
		schemaRow("public", "memory_chunk_tags", "memory_chunk_id", "bigint", "NO"),
		schemaRow("public", "memory_chunk_tags", "tag_id", "bigint", "NO"),
	}
	runner.queryResults["SELECT mc.id, mc.agent_id, mc.kind, mc.content FROM memory_chunks mc WHERE mc.content ILIKE '%postgres%' ORDER BY mc.created_at DESC LIMIT 5"] = []MemorySQLRow{
		{"id": int64(7), "agent_id": "doc_manager", "kind": "documentation_research", "content": "PostgreSQL required validation rule: field must be present and not empty."},
	}
	client := &fakeCommandDecisionClient{responses: []string{
		`{"sql":"SELECT mc.id, mc.agent_id, mc.kind, mc.content FROM memory_chunks mc WHERE mc.content ILIKE '%postgres%' ORDER BY mc.created_at DESC LIMIT 5","answer":"Search memory chunks for PostgreSQL documentation."}`,
	}}

	result, err := RunDBManagerQuery(context.Background(), "Find saved PostgreSQL validation documentation memories.", runner, client)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 {
		t.Fatalf("llm calls = %d, want 1", client.calls)
	}
	if len(result.Schema) != 3 {
		t.Fatalf("schema tables = %#v", result.Schema)
	}
	if len(result.Rows) != 1 || !strings.Contains(stringFromAny(result.Rows[0]["content"]), "PostgreSQL required validation") {
		t.Fatalf("unexpected query rows: %#v", result.Rows)
	}
	if !runner.SawSQL("information_schema.columns") {
		t.Fatalf("manager did not inspect schema first; log=%#v", runner.SQLLog)
	}
	if !runner.SawSQL("FROM memory_chunks") {
		t.Fatalf("manager did not execute LLM-generated memory query; log=%#v", runner.SQLLog)
	}
	if len(client.requests) != 1 || !strings.Contains(client.requests[0].Messages[1].Content, "memory_chunks") {
		t.Fatalf("LLM request did not include schema context: %#v", client.requests)
	}
}

func TestDBManagerCanPointAtDifferentPostgresSchema(t *testing.T) {
	runner := newFakeDBManagerRunner()
	runner.schemaRows = []MemorySQLRow{
		schemaRow("app", "projects", "id", "bigint", "NO"),
		schemaRow("app", "projects", "name", "text", "NO"),
		schemaRow("app", "projects", "framework", "text", "YES"),
		schemaRow("app", "project_notes", "project_id", "bigint", "NO"),
		schemaRow("app", "project_notes", "body", "text", "NO"),
	}
	sql := "SELECT p.name, p.framework, n.body FROM app.projects p JOIN app.project_notes n ON n.project_id = p.id WHERE n.body ILIKE '%dashboard%' LIMIT 10"
	runner.queryResults[sql] = []MemorySQLRow{
		{"name": "billing-console", "framework": "PostgreSQL", "body": "Dashboard pattern uses dense tables and restrained controls."},
	}
	client := &fakeCommandDecisionClient{responses: []string{
		`{"sql":` + quoteJSONForGoCLITest(sql) + `,"answer":"Query the external project database for dashboard notes."}`,
	}}

	result, err := RunDBManagerQuery(context.Background(), "Which past project notes mention dashboard design patterns?", runner, client)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("rows = %#v", result.Rows)
	}
	if stringFromAny(result.Rows[0]["name"]) != "billing-console" {
		t.Fatalf("unexpected external project row: %#v", result.Rows[0])
	}
	if !strings.Contains(client.requests[0].Messages[1].Content, "project_notes") {
		t.Fatalf("external schema was not provided to LLM: %s", client.requests[0].Messages[1].Content)
	}
}

func TestDBManagerRejectsLLMGeneratedWriteSQL(t *testing.T) {
	runner := newFakeDBManagerRunner()
	runner.schemaRows = []MemorySQLRow{schemaRow("public", "memory_chunks", "content", "text", "NO")}
	client := &fakeCommandDecisionClient{responses: []string{
		`{"sql":"DELETE FROM memory_chunks","answer":"bad"}`,
	}}

	_, err := RunDBManagerQuery(context.Background(), "Remove irrelevant memories.", runner, client)
	if err == nil {
		t.Fatal("expected write SQL to be rejected")
	}
	if !strings.Contains(err.Error(), "SELECT or WITH") {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.SawSQL("DELETE FROM memory_chunks") {
		t.Fatalf("destructive SQL was executed; log=%#v", runner.SQLLog)
	}
}

func TestDBManagerRequiresLLMForDynamicQuerySelection(t *testing.T) {
	runner := newFakeDBManagerRunner()
	runner.schemaRows = []MemorySQLRow{schemaRow("public", "memory_chunks", "content", "text", "NO")}
	_, err := RunDBManagerQuery(context.Background(), "Find memory about Tailwind.", runner, nil)
	if err == nil {
		t.Fatal("expected missing LLM to fail")
	}
	if runner.SawSQL("FROM memory_chunks") {
		t.Fatalf("manager selected query without LLM; log=%#v", runner.SQLLog)
	}
}

func TestDBManagerSourceAuditHasNoNaturalLanguagePromptRouting(t *testing.T) {
	source, err := osReadFileForDBManagerAudit("db_manager.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`strings.Contains(question`,
		`strings.Contains(strings.ToLower(question`,
		`switch question`,
		`case "memory"`,
		`case "postgres"`,
		`case "tailwind"`,
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("db manager contains forbidden prompt routing %q", forbidden)
		}
	}
}

type fakeDBManagerRunner struct {
	schemaRows   []MemorySQLRow
	queryResults map[string][]MemorySQLRow
	SQLLog       []string
}

func newFakeDBManagerRunner() *fakeDBManagerRunner {
	return &fakeDBManagerRunner{queryResults: map[string][]MemorySQLRow{}}
}

func (r *fakeDBManagerRunner) Exec(ctx context.Context, sql string, args ...any) error {
	r.SQLLog = append(r.SQLLog, normalizeSQLForTest(sql))
	return nil
}

func (r *fakeDBManagerRunner) Query(ctx context.Context, sql string, args ...any) ([]MemorySQLRow, error) {
	normalized := normalizeSQLForTest(sql)
	r.SQLLog = append(r.SQLLog, normalized)
	if strings.Contains(normalized, "information_schema.columns") {
		return r.schemaRows, nil
	}
	if rows, ok := r.queryResults[normalized]; ok {
		return rows, nil
	}
	return nil, nil
}

func (r *fakeDBManagerRunner) SawSQL(fragment string) bool {
	for _, sql := range r.SQLLog {
		if strings.Contains(sql, fragment) {
			return true
		}
	}
	return false
}

func schemaRow(schema, table, column, dataType, nullable string) MemorySQLRow {
	return MemorySQLRow{
		"table_schema": schema,
		"table_name":   table,
		"column_name":  column,
		"data_type":    dataType,
		"is_nullable":  nullable,
	}
}

func osReadFileForDBManagerAudit(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n"), nil
}
