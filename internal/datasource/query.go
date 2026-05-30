package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/omni"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	JobSource          = "omni-data-source"
	MetadataSourceID   = "data_source_id"
	MetadataSourceName = "data_source_name"
	MetadataQuestion   = "question"
	MetadataChannelID  = "channel_id"
	MetadataMode       = "mode"
	JobModeExplore     = "explore"
	MaxQueryRows       = 500
)

type QueryResult struct {
	Question     string           `json:"question,omitempty"`
	SQL          string           `json:"sql,omitempty"`
	Answer       string           `json:"answer,omitempty"`
	Columns      []string         `json:"columns"`
	Rows         []map[string]any `json:"rows"`
	Count        int              `json:"count"`
	HardFacts    []string         `json:"hard_facts,omitempty"`
	QuerySteps   []QueryStep      `json:"query_steps,omitempty"`
	TextInsights []TextInsight    `json:"text_insights,omitempty"`
	Evidence     QueryEvidence    `json:"evidence,omitempty"`
}

func JobMetadata(sourceID, sourceName, question, channelID string) ([]byte, error) {
	payload := map[string]any{
		"source":             JobSource,
		MetadataSourceID:   strings.TrimSpace(sourceID),
		MetadataSourceName: strings.TrimSpace(sourceName),
		MetadataQuestion:   strings.TrimSpace(question),
	}
	if strings.TrimSpace(channelID) != "" {
		payload[MetadataChannelID] = strings.TrimSpace(channelID)
	}
	return json.Marshal(payload)
}

func ParseChannelID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(stringFromAny(payload[MetadataChannelID]))
}

func IsJobMetadata(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	return strings.TrimSpace(stringFromAny(payload["source"])) == JobSource
}

func ParseJobMetadata(raw json.RawMessage) (sourceID, sourceName, question string, err error) {
	if len(raw) == 0 {
		return "", "", "", fmt.Errorf("job metadata is empty")
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", "", fmt.Errorf("parse job metadata: %w", err)
	}
	sourceID = strings.TrimSpace(stringFromAny(payload[MetadataSourceID]))
	sourceName = strings.TrimSpace(stringFromAny(payload[MetadataSourceName]))
	question = strings.TrimSpace(stringFromAny(payload[MetadataQuestion]))
	if sourceID == "" {
		return "", "", "", fmt.Errorf("data_source_id is required")
	}
	if question == "" {
		return "", "", "", fmt.Errorf("question is required")
	}
	return sourceID, sourceName, question, nil
}

func ConnectReadOnly(ctx context.Context, conn Connection) (*pgxpool.Pool, error) {
	if !conn.ReadOnly {
		return nil, fmt.Errorf("only read-only data sources are supported")
	}
	driver := strings.ToLower(strings.TrimSpace(conn.Driver))
	if driver != "" && driver != "postgres" {
		return nil, fmt.Errorf("only postgres data sources are supported")
	}
	dsn, err := BuildPostgresDSN(conn)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if _, err := pool.Exec(ctx, "SET default_transaction_read_only = on"); err != nil {
		pool.Close()
		return nil, err
	}
	if _, err := pool.Exec(ctx, "SELECT 1"); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func InspectSchema(ctx context.Context, conn Connection) ([]omni.DBSchemaTable, error) {
	pool, err := ConnectReadOnly(ctx, conn)
	if err != nil {
		return nil, err
	}
	defer pool.Close()
	return omni.InspectPostgresSchema(ctx, omni.NewPgxMemoryRunner(pool))
}

func RunSQL(ctx context.Context, conn Connection, sqlText string) (QueryResult, error) {
	sqlText = strings.TrimSpace(sqlText)
	if err := omni.ValidateReadOnlyPostgresQuery(sqlText); err != nil {
		return QueryResult{}, err
	}
	sqlText = enforceQueryLimit(sqlText, MaxQueryRows)
	pool, err := ConnectReadOnly(ctx, conn)
	if err != nil {
		return QueryResult{}, err
	}
	defer pool.Close()
	rows, err := omni.NewPgxMemoryRunner(pool).Query(ctx, sqlText)
	if err != nil {
		return QueryResult{}, err
	}
	columns, publicRows := rowsToColumns(rows)
	return QueryResult{
		SQL:     sqlText,
		Columns: columns,
		Rows:    publicRows,
		Count:   len(publicRows),
	}, nil
}

func AskQuestion(ctx context.Context, conn Connection, question string, llm omni.DBManagerLLMClient) (QueryResult, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return QueryResult{}, fmt.Errorf("question is required")
	}
	pool, err := ConnectReadOnly(ctx, conn)
	if err != nil {
		return QueryResult{}, err
	}
	defer pool.Close()
	result, err := omni.RunDBManagerQuery(ctx, question, omni.NewPgxMemoryRunner(pool), llm)
	if err != nil {
		return QueryResult{}, err
	}
	columns, publicRows := rowsToColumns(result.Rows)
	return QueryResult{
		Question: result.Question,
		SQL:      result.SQL,
		Answer:   result.Answer,
		Columns:  columns,
		Rows:     publicRows,
		Count:    len(publicRows),
	}, nil
}

func FormatJobResult(result QueryResult) (summary string, payload string, err error) {
	result = FinalizeQueryResult(result)
	if strings.TrimSpace(result.Answer) == "" && result.Count > 0 {
		result.Answer = fmt.Sprintf("%d row(s) returned.", result.Count)
	}
	blob, err := json.Marshal(result)
	if err != nil {
		return "", "", err
	}
	lines := []string{}
	if strings.TrimSpace(result.Question) != "" {
		lines = append(lines, "Q: "+strings.TrimSpace(result.Question))
	}
	if strings.TrimSpace(result.Answer) != "" {
		lines = append(lines, "", strings.TrimSpace(result.Answer))
	}
	if result.Evidence.StepCount > 0 {
		lines = append(lines, "", fmt.Sprintf("Evidence: %d queries · confidence %s · %d rows", result.Evidence.StepCount, result.Evidence.Confidence, result.Evidence.RowCount))
	}
	if len(result.TextInsights) > 0 {
		lines = append(lines, "", "Text insights:")
		for _, insight := range result.TextInsights {
			line := fmt.Sprintf("- %s (%d samples): %s", insight.Field, insight.Samples, insight.Summary)
			if len(insight.Themes) > 0 {
				line += " · themes: " + strings.Join(insight.Themes, ", ")
			}
			lines = append(lines, line)
		}
	}
	if len(result.HardFacts) > 0 {
		limit := result.HardFacts
		if len(limit) > 8 {
			limit = limit[:8]
		}
		lines = append(lines, "", "Hard facts:")
		for _, fact := range limit {
			lines = append(lines, "- "+fact)
		}
		if len(result.HardFacts) > 8 {
			lines = append(lines, fmt.Sprintf("- … +%d more facts", len(result.HardFacts)-8))
		}
	}
	if len(result.QuerySteps) > 1 {
		lines = append(lines, "", fmt.Sprintf("%d investigation queries executed.", len(result.QuerySteps)))
	}
	if strings.TrimSpace(result.SQL) != "" {
		lines = append(lines, "", "SQL:", result.SQL)
	}
	if result.Count > 0 {
		lines = append(lines, "", fmt.Sprintf("%d row(s) returned.", result.Count))
	}
	return strings.Join(lines, "\n"), string(blob), nil
}

func ExploreJobMetadata(sourceID, sourceName string) ([]byte, error) {
	payload := map[string]any{
		"source":             JobSource,
		MetadataSourceID:   strings.TrimSpace(sourceID),
		MetadataSourceName: strings.TrimSpace(sourceName),
		MetadataMode:       JobModeExplore,
	}
	return json.Marshal(payload)
}

func IsExploreJobMetadata(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	if strings.TrimSpace(stringFromAny(payload["source"])) != JobSource {
		return false
	}
	return strings.TrimSpace(stringFromAny(payload[MetadataMode])) == JobModeExplore
}

func ParseExploreMetadata(raw json.RawMessage) (sourceID, sourceName string, err error) {
	if len(raw) == 0 {
		return "", "", fmt.Errorf("job metadata is empty")
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", fmt.Errorf("parse job metadata: %w", err)
	}
	sourceID = strings.TrimSpace(stringFromAny(payload[MetadataSourceID]))
	sourceName = strings.TrimSpace(stringFromAny(payload[MetadataSourceName]))
	if sourceID == "" {
		return "", "", fmt.Errorf("data_source_id is required")
	}
	return sourceID, sourceName, nil
}

func ExplorePipeline() string {
	return model.PipelineDataExplore
}

func Pipeline() string {
	return model.PipelineDataQuery
}

func enforceQueryLimit(sql string, max int) string {
	if max <= 0 {
		return sql
	}
	lower := strings.ToLower(sql)
	if strings.Contains(lower, " limit ") {
		return sql
	}
	return strings.TrimRight(strings.TrimSpace(sql), ";") + fmt.Sprintf(" LIMIT %d", max)
}

func rowsToColumns(rows []omni.MemorySQLRow) ([]string, []map[string]any) {
	columns := []string{}
	seen := map[string]struct{}{}
	for _, row := range rows {
		for key := range row {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			columns = append(columns, key)
		}
	}
	publicRows := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out := map[string]any{}
		for key, value := range row {
			out[key] = stringifyCell(value)
		}
		publicRows = append(publicRows, out)
	}
	return columns, publicRows
}

func stringifyCell(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case []byte:
		return string(v)
	case time.Time:
		return v.UTC().Format(time.RFC3339)
	default:
		return v
	}
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func ParseJobResult(raw string) (QueryResult, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return QueryResult{}, fmt.Errorf("job result is empty")
	}
	var result QueryResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return QueryResult{}, err
	}
	return result, nil
}
