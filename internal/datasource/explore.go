package datasource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/omni"
)

type MemoryWriter interface {
	AddMemory(ctx context.Context, source, kind, content string, tags []string) error
}

type CatalogStore interface {
	Get(ctx context.Context, sourceID string) (SchemaCatalog, bool, error)
	Save(ctx context.Context, catalog SchemaCatalog) error
}

// BuildSchemaCatalog inspects metadata only — no row samples.
func BuildSchemaCatalog(ctx context.Context, conn Connection, profile Profile, sourceID, sourceName string, llm omni.DBManagerLLMClient) (SchemaCatalog, error) {
	profile = NormalizeProfile(profile)
	pool, err := ConnectReadOnly(ctx, conn)
	if err != nil {
		return SchemaCatalog{}, err
	}
	defer pool.Close()
	runner := omni.NewPgxMemoryRunner(pool)

	schema, err := omni.InspectPostgresSchema(ctx, runner)
	if err != nil {
		return SchemaCatalog{}, err
	}
	stats, err := fetchPostgresTableStats(ctx, runner)
	if err != nil {
		return SchemaCatalog{}, err
	}

	tables := make([]TableCatalog, 0, len(schema))
	for _, table := range schema {
		fullName := table.Schema + "." + table.Name
		cols := make([]ColumnCatalog, 0, len(table.Columns))
		for _, col := range table.Columns {
			cols = append(cols, ColumnCatalog{
				Name:        col.Name,
				DataType:    col.DataType,
				Nullable:    col.Nullable,
				Sensitive:   IsSensitiveColumn(col.Name),
				TextContent: IsTextContentColumn(col.Name, col.DataType),
			})
		}
		tables = append(tables, TableCatalog{
			Schema:      table.Schema,
			Name:        table.Name,
			FullName:    fullName,
			RowEstimate: stats[strings.ToLower(fullName)],
			Columns:     cols,
		})
	}
	sort.Slice(tables, func(i, j int) bool {
		if tables[i].RowEstimate != tables[j].RowEstimate {
			return tables[i].RowEstimate > tables[j].RowEstimate
		}
		return tables[i].FullName < tables[j].FullName
	})

	catalog := SchemaCatalog{
		SourceID:    sourceID,
		SourceName:  sourceName,
		Driver:      profile.Driver,
		Domain:      profile.Domain,
		Fingerprint: fingerprintTables(tables),
		Tables:      tables,
	}

	if llm != nil {
		if err := enrichCatalogWithLLM(ctx, llm, profile, &catalog); err != nil {
			catalog.Summary = "Schema catalog built from metadata. LLM table labeling unavailable: " + err.Error()
		}
	}
	if strings.TrimSpace(catalog.Summary) == "" {
		catalog.Summary = fmt.Sprintf("Cataloged %d tables for %s (%s).", len(catalog.Tables), sourceName, profile.Domain)
	}
	return catalog, nil
}

func fetchPostgresTableStats(ctx context.Context, runner omni.MemorySQLRunner) (map[string]int64, error) {
	rows, err := runner.Query(ctx, `
		SELECT schemaname, relname, COALESCE(n_live_tup, 0)::bigint AS row_estimate
		FROM pg_stat_user_tables
	`)
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, row := range rows {
		schema := stringFromAny(row["schemaname"])
		name := stringFromAny(row["relname"])
		if schema == "" || name == "" {
			continue
		}
		key := strings.ToLower(schema + "." + name)
		switch v := row["row_estimate"].(type) {
		case int64:
			out[key] = v
		case int:
			out[key] = int64(v)
		case float64:
			out[key] = int64(v)
		}
	}
	return out, nil
}

func fingerprintTables(tables []TableCatalog) string {
	parts := make([]string, 0, len(tables))
	for _, table := range tables {
		colParts := make([]string, 0, len(table.Columns))
		for _, col := range table.Columns {
			colParts = append(colParts, col.Name+":"+col.DataType)
		}
		parts = append(parts, table.FullName+"["+strings.Join(colParts, ",")+"]")
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:8])
}

func enrichCatalogWithLLM(ctx context.Context, llm omni.DBManagerLLMClient, profile Profile, catalog *SchemaCatalog) error {
	limit := catalog.Tables
	if len(limit) > 40 {
		limit = limit[:40]
	}
	payload, _ := json.Marshal(map[string]any{
		"domain":         profile.Domain,
		"context_prompt": profile.ContextPrompt,
		"tables":         limit,
	})
	resp, err := llm.ChatRaw(ctx, omni.OllamaChatRequest{
		Messages: []omni.OllamaMessage{
			{
				Role: "system",
				Content: strings.Join([]string{
					"Return JSON only.",
					`Schema: {"summary":"one paragraph overview","tables":[{"full_name":"schema.table","purpose":"what this table likely stores","columns":[{"name":"col","hint":"column role"}]}]}`,
					"Infer table/column purposes from names and types only. Do not invent PHI or example values.",
					DomainGuidance(profile.Domain),
				}, "\n"),
			},
			{Role: "user", Content: string(payload)},
		},
		Format: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"summary": map[string]any{"type": "string"},
				"tables": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"full_name": map[string]any{"type": "string"},
							"purpose":   map[string]any{"type": "string"},
							"columns": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"name": map[string]any{"type": "string"},
										"hint": map[string]any{"type": "string"},
									},
								},
							},
						},
					},
				},
			},
			"required": []string{"summary", "tables"},
		},
		Options: map[string]any{"temperature": 0},
	})
	if err != nil {
		return err
	}
	var parsed struct {
		Summary string `json:"summary"`
		Tables  []struct {
			FullName string `json:"full_name"`
			Purpose  string `json:"purpose"`
			Columns  []struct {
				Name string `json:"name"`
				Hint string `json:"hint"`
			} `json:"columns"`
		} `json:"tables"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Content)), &parsed); err != nil {
		return err
	}
	catalog.Summary = strings.TrimSpace(parsed.Summary)
	index := map[string]int{}
	for i, table := range catalog.Tables {
		index[strings.ToLower(table.FullName)] = i
	}
	for _, labeled := range parsed.Tables {
		key := strings.ToLower(strings.TrimSpace(labeled.FullName))
		i, ok := index[key]
		if !ok {
			continue
		}
		catalog.Tables[i].Purpose = strings.TrimSpace(labeled.Purpose)
		colIndex := map[string]int{}
		for j, col := range catalog.Tables[i].Columns {
			colIndex[strings.ToLower(col.Name)] = j
		}
		for _, hint := range labeled.Columns {
			j, ok := colIndex[strings.ToLower(strings.TrimSpace(hint.Name))]
			if !ok {
				continue
			}
			catalog.Tables[i].Columns[j].Hint = strings.TrimSpace(hint.Hint)
		}
	}
	return nil
}

func PersistCatalogMemories(ctx context.Context, writer MemoryWriter, catalog SchemaCatalog) error {
	if writer == nil {
		return nil
	}
	tags := []string{
		"data-source:" + catalog.SourceID,
		"schema-map",
		"domain:" + catalog.Domain,
		"driver:" + catalog.Driver,
	}
	summary := strings.TrimSpace(catalog.Summary)
	if summary == "" {
		summary = fmt.Sprintf("Schema map for %s", catalog.SourceName)
	}
	if err := writer.AddMemory(ctx, "data-source:"+catalog.SourceID+":catalog", "reference", summary, tags); err != nil {
		return err
	}
	limit := catalog.Tables
	if len(limit) > 24 {
		limit = limit[:24]
	}
	for _, table := range limit {
		content := formatTableMemory(catalog, table)
		if strings.TrimSpace(content) == "" {
			continue
		}
		tableTags := append([]string{}, tags...)
		tableTags = append(tableTags, "table:"+table.FullName)
		if err := writer.AddMemory(ctx, "data-source:"+catalog.SourceID+":table:"+table.FullName, "reference", content, tableTags); err != nil {
			return err
		}
	}
	return nil
}

func formatTableMemory(catalog SchemaCatalog, table TableCatalog) string {
	lines := []string{
		fmt.Sprintf("Database: %s (%s)", catalog.SourceName, catalog.Domain),
		fmt.Sprintf("Table: %s", table.FullName),
	}
	if table.Purpose != "" {
		lines = append(lines, "Purpose: "+table.Purpose)
	}
	if table.RowEstimate > 0 {
		lines = append(lines, fmt.Sprintf("Approx rows: %d", table.RowEstimate))
	}
	colLines := make([]string, 0, len(table.Columns))
	for _, col := range table.Columns {
		label := col.Name + " " + col.DataType
		if col.Sensitive {
			label += " [sensitive]"
		}
		if col.Hint != "" {
			label += " — " + col.Hint
		}
		colLines = append(colLines, label)
	}
	if len(colLines) > 0 {
		lines = append(lines, "Columns:", strings.Join(colLines, "; "))
	}
	return strings.Join(lines, "\n")
}
