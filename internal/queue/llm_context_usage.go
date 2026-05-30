package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

const defaultLLMContextUsageHistoryLimit = 100

type LLMContextUsageRecord struct {
	Source            string
	Model             string
	Provider          string
	ProjectID         int64
	CardID            string
	RunID             string
	JobID             int64
	StepID            int64
	Scope             string
	Attempt           int
	PromptChars       int
	SentChars         int
	ContextLimitChars int
	Shrunk            bool
	SavedPct          float64
	Success           bool
	ErrorClass        string
	LatencyMS         int64
	DeltaChars        int
	Metadata          any
	CreatedAt         time.Time
}

type LLMContextUsageSummary struct {
	Requests       int     `json:"requests"`
	FailureEvents  int     `json:"failure_events"`
	OverloadEvents int     `json:"overload_events"`
	AvgSentChars   float64 `json:"avg_sent_chars"`
	AvgUtilization float64 `json:"avg_utilization_pct"`
	AvgDeltaChars  float64 `json:"avg_delta_chars"`
	MaxSentChars   int     `json:"max_sent_chars"`
	ContextLimit   int     `json:"context_limit_chars"`
}

type LLMContextUsageBySource struct {
	Source         string  `json:"source"`
	Requests       int     `json:"requests"`
	OverloadEvents int     `json:"overload_events"`
	AvgSentChars   float64 `json:"avg_sent_chars"`
	AvgUtilization float64 `json:"avg_utilization_pct"`
	MaxSentChars   int     `json:"max_sent_chars"`
}

type LLMContextUsageEntry struct {
	ID                string          `json:"id"`
	Source            string          `json:"source"`
	Model             string          `json:"model"`
	Provider          string          `json:"provider"`
	CardID            string          `json:"card_id,omitempty"`
	ProjectID         *int64          `json:"project_id,omitempty"`
	RunID             string          `json:"run_id,omitempty"`
	JobID             *int64          `json:"job_id,omitempty"`
	Scope             string          `json:"scope,omitempty"`
	Attempt           int             `json:"attempt,omitempty"`
	PromptChars       int             `json:"prompt_chars"`
	SentChars         int             `json:"sent_chars"`
	ContextLimitChars int             `json:"context_limit_chars"`
	UtilizationPct    float64         `json:"utilization_pct"`
	Overloaded        bool            `json:"overloaded"`
	Shrunk            bool            `json:"shrunk"`
	SavedPct          float64         `json:"saved_pct"`
	Success           bool            `json:"success"`
	ErrorClass        string          `json:"error_class,omitempty"`
	DeltaChars        int             `json:"delta_chars"`
	Metadata          json.RawMessage `json:"metadata,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
}

type LLMContextUsageDailyPoint struct {
	Day            string  `json:"day"`
	Requests       int     `json:"requests"`
	OverloadEvents int     `json:"overload_events"`
	AvgSentChars   float64 `json:"avg_sent_chars"`
	AvgUtilization float64 `json:"avg_utilization_pct"`
}

type LLMContextUsageMetricsResponse struct {
	Summary  LLMContextUsageSummary    `json:"summary"`
	BySource []LLMContextUsageBySource `json:"by_source"`
	History  []LLMContextUsageEntry    `json:"history"`
	Overloads []LLMContextUsageEntry   `json:"overloads"`
	Daily    []LLMContextUsageDailyPoint `json:"daily"`
}

func llmContextUtilizationPct(sent, limit int) float64 {
	if limit <= 0 || sent <= 0 {
		return 0
	}
	pct := float64(sent) / float64(limit) * 100
	if pct < 0 {
		return 0
	}
	return math.Round(pct*100) / 100
}

func llmContextOverloaded(sent, limit int) bool {
	if limit <= 0 || sent <= 0 {
		return false
	}
	return sent >= int(float64(limit)*0.95)
}

func classifyLLMContextError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "context") && (strings.Contains(msg, "length") || strings.Contains(msg, "overflow") || strings.Contains(msg, "exceed")):
		return "context_overflow"
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline"):
		return "timeout"
	case strings.Contains(msg, "connection") || strings.Contains(msg, "eof") || strings.Contains(msg, "unavailable"):
		return "backend_unavailable"
	case strings.Contains(msg, "json") || strings.Contains(msg, "parse") || strings.Contains(msg, "unmarshal"):
		return "malformed_response"
	default:
		return "llm_error"
	}
}

func (r *Repository) RecordLLMContextUsage(ctx context.Context, record LLMContextUsageRecord) error {
	source := strings.TrimSpace(record.Source)
	if source == "" {
		return fmt.Errorf("llm context usage source is required")
	}
	if record.PromptChars < 0 || record.SentChars < 0 || record.ContextLimitChars < 0 {
		return fmt.Errorf("llm context usage char counts must be non-negative")
	}
	sent := record.SentChars
	if sent <= 0 {
		sent = record.PromptChars
	}
	limit := record.ContextLimitChars
	utilization := llmContextUtilizationPct(sent, limit)
	overloaded := llmContextOverloaded(sent, limit)
	savedPct := record.SavedPct
	if savedPct < 0 {
		savedPct = 0
	}
	if savedPct > 100 {
		savedPct = 100
	}
	created := record.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	var projectID *int64
	if record.ProjectID > 0 {
		projectID = &record.ProjectID
	}
	var jobID *int64
	if record.JobID > 0 {
		jobID = &record.JobID
	}
	var stepID *int64
	if record.StepID > 0 {
		stepID = &record.StepID
	}
	runID := strings.TrimSpace(record.RunID)
	scope := strings.TrimSpace(record.Scope)
	delta := record.DeltaChars
	if delta <= 0 && runID != "" && scope != "" {
		var prev int
		if err := r.pool.QueryRow(ctx, `
			SELECT COALESCE(sent_chars, 0)
			FROM omni_llm_context_usage
			WHERE run_id = $1::uuid AND scope = $2
			ORDER BY created_at DESC
			LIMIT 1
		`, runID, scope).Scan(&prev); err == nil && prev > 0 && sent > prev {
			delta = sent - prev
		}
	}
	attempt := record.Attempt
	if attempt <= 0 {
		attempt = 1
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO omni_llm_context_usage (
			source, model, provider, project_id, card_id, run_id, job_id, step_id, scope, attempt,
			prompt_chars, sent_chars, context_limit_chars, utilization_pct,
			overloaded, shrunk, saved_pct, success, error_class, latency_ms, delta_chars,
			metadata, created_at
		)
		VALUES (
			$1, NULLIF($2,''), NULLIF($3,''), $4, NULLIF($5,''), NULLIF($6,'')::uuid, $7, $8, NULLIF($9,''), $10,
			$11, $12, $13, $14, $15, $16, $17, $18, NULLIF($19,''), NULLIF($20,0), $21, $22, $23
		)
	`, source, strings.TrimSpace(record.Model), strings.TrimSpace(record.Provider), projectID,
		strings.TrimSpace(record.CardID), nullUUID(runID), jobID, stepID, scope, attempt,
		record.PromptChars, sent, limit, utilization, overloaded, record.Shrunk, savedPct,
		record.Success, strings.TrimSpace(record.ErrorClass), record.LatencyMS, delta,
		jsonParam(record.Metadata), created)
	return err
}

func nullUUID(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func (r *Repository) LLMContextUsageMetrics(ctx context.Context, source string, historyLimit int) (LLMContextUsageMetricsResponse, error) {
	if historyLimit <= 0 || historyLimit > 500 {
		historyLimit = defaultLLMContextUsageHistoryLimit
	}
	source = strings.TrimSpace(source)

	summary := LLMContextUsageSummary{}
	summaryQuery := `
		SELECT COUNT(*),
			COUNT(*) FILTER (WHERE NOT success),
			COUNT(*) FILTER (WHERE overloaded),
			COALESCE(AVG(sent_chars), 0),
			COALESCE(AVG(utilization_pct), 0),
			COALESCE(AVG(NULLIF(delta_chars, 0)), 0),
			COALESCE(MAX(sent_chars), 0),
			COALESCE(MAX(context_limit_chars), 0)
		FROM omni_llm_context_usage
	`
	summaryArgs := []any{}
	if source != "" {
		summaryQuery += ` WHERE source = $1`
		summaryArgs = append(summaryArgs, source)
	}
	if err := r.pool.QueryRow(ctx, summaryQuery, summaryArgs...).Scan(
		&summary.Requests,
		&summary.FailureEvents,
		&summary.OverloadEvents,
		&summary.AvgSentChars,
		&summary.AvgUtilization,
		&summary.AvgDeltaChars,
		&summary.MaxSentChars,
		&summary.ContextLimit,
	); err != nil {
		return LLMContextUsageMetricsResponse{}, err
	}
	summary.AvgSentChars = math.Round(summary.AvgSentChars*10) / 10
	summary.AvgUtilization = math.Round(summary.AvgUtilization*100) / 100
	summary.AvgDeltaChars = math.Round(summary.AvgDeltaChars*10) / 10

	usageEntrySelect := `
		id::text, source, COALESCE(model,''), COALESCE(provider,''), COALESCE(card_id,''), project_id,
		COALESCE(run_id::text,''), job_id, COALESCE(scope,''), COALESCE(attempt,1),
		prompt_chars, sent_chars, context_limit_chars, utilization_pct, overloaded, shrunk, saved_pct,
		COALESCE(success, TRUE), COALESCE(error_class,''), COALESCE(delta_chars,0), metadata, created_at
	`

	bySourceQuery := `
		SELECT source,
			COUNT(*),
			COUNT(*) FILTER (WHERE overloaded),
			COALESCE(AVG(sent_chars), 0),
			COALESCE(AVG(utilization_pct), 0),
			COALESCE(MAX(sent_chars), 0)
		FROM omni_llm_context_usage
	`
	bySourceArgs := []any{}
	if source != "" {
		bySourceQuery += ` WHERE source = $1`
		bySourceArgs = append(bySourceArgs, source)
	}
	bySourceQuery += `
		GROUP BY source
		ORDER BY COUNT(*) FILTER (WHERE overloaded) DESC, COUNT(*) DESC
		LIMIT 20
	`
	bySourceRows, err := r.pool.Query(ctx, bySourceQuery, bySourceArgs...)
	if err != nil {
		return LLMContextUsageMetricsResponse{}, err
	}
	defer bySourceRows.Close()

	bySource := make([]LLMContextUsageBySource, 0, 20)
	for bySourceRows.Next() {
		var item LLMContextUsageBySource
		if err := bySourceRows.Scan(
			&item.Source, &item.Requests, &item.OverloadEvents,
			&item.AvgSentChars, &item.AvgUtilization, &item.MaxSentChars,
		); err != nil {
			return LLMContextUsageMetricsResponse{}, err
		}
		item.AvgSentChars = math.Round(item.AvgSentChars*10) / 10
		item.AvgUtilization = math.Round(item.AvgUtilization*100) / 100
		bySource = append(bySource, item)
	}
	if err := bySourceRows.Err(); err != nil {
		return LLMContextUsageMetricsResponse{}, err
	}

	historyQuery := `
		SELECT ` + usageEntrySelect + `
		FROM omni_llm_context_usage
	`
	historyArgs := []any{}
	if source != "" {
		historyQuery += ` WHERE source = $1`
		historyArgs = append(historyArgs, source)
	}
	historyQuery += ` ORDER BY created_at DESC LIMIT ` + fmt.Sprintf("%d", historyLimit)

	historyRows, err := r.pool.Query(ctx, historyQuery, historyArgs...)
	if err != nil {
		return LLMContextUsageMetricsResponse{}, err
	}
	defer historyRows.Close()

	history := make([]LLMContextUsageEntry, 0, historyLimit)
	for historyRows.Next() {
		item, err := scanLLMContextUsageEntry(historyRows)
		if err != nil {
			return LLMContextUsageMetricsResponse{}, err
		}
		history = append(history, item)
	}
	if err := historyRows.Err(); err != nil {
		return LLMContextUsageMetricsResponse{}, err
	}

	overloadQuery := `
		SELECT ` + usageEntrySelect + `
		FROM omni_llm_context_usage
		WHERE overloaded = TRUE OR NOT success OR delta_chars >= 2000
	`
	overloadArgs := []any{}
	if source != "" {
		overloadQuery += ` AND source = $1`
		overloadArgs = append(overloadArgs, source)
	}
	overloadQuery += ` ORDER BY created_at DESC LIMIT 24`

	overloadRows, err := r.pool.Query(ctx, overloadQuery, overloadArgs...)
	if err != nil {
		return LLMContextUsageMetricsResponse{}, err
	}
	defer overloadRows.Close()

	overloads := make([]LLMContextUsageEntry, 0, 24)
	for overloadRows.Next() {
		item, err := scanLLMContextUsageEntry(overloadRows)
		if err != nil {
			return LLMContextUsageMetricsResponse{}, err
		}
		overloads = append(overloads, item)
	}
	if err := overloadRows.Err(); err != nil {
		return LLMContextUsageMetricsResponse{}, err
	}

	dailyQuery := `
		SELECT to_char(date_trunc('day', created_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS day,
			COUNT(*),
			COUNT(*) FILTER (WHERE overloaded),
			COALESCE(AVG(sent_chars), 0),
			COALESCE(AVG(utilization_pct), 0)
		FROM omni_llm_context_usage
		WHERE created_at >= NOW() - INTERVAL '30 days'
	`
	dailyArgs := []any{}
	if source != "" {
		dailyQuery += ` AND source = $1`
		dailyArgs = append(dailyArgs, source)
	}
	dailyQuery += `
		GROUP BY 1
		ORDER BY 1 ASC
	`

	dailyRows, err := r.pool.Query(ctx, dailyQuery, dailyArgs...)
	if err != nil {
		return LLMContextUsageMetricsResponse{}, err
	}
	defer dailyRows.Close()

	daily := make([]LLMContextUsageDailyPoint, 0, 31)
	for dailyRows.Next() {
		var item LLMContextUsageDailyPoint
		if err := dailyRows.Scan(&item.Day, &item.Requests, &item.OverloadEvents, &item.AvgSentChars, &item.AvgUtilization); err != nil {
			return LLMContextUsageMetricsResponse{}, err
		}
		item.AvgSentChars = math.Round(item.AvgSentChars*10) / 10
		item.AvgUtilization = math.Round(item.AvgUtilization*100) / 100
		daily = append(daily, item)
	}
	if err := dailyRows.Err(); err != nil {
		return LLMContextUsageMetricsResponse{}, err
	}

	return LLMContextUsageMetricsResponse{
		Summary:   summary,
		BySource:  bySource,
		History:   history,
		Overloads: overloads,
		Daily:     daily,
	}, nil
}

func (r *Repository) ListRecentLLMActivity(ctx context.Context, limit int) ([]LLMContextUsageEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, source, COALESCE(model,''), COALESCE(provider,''), COALESCE(card_id,''), project_id,
			COALESCE(run_id::text,''), job_id, COALESCE(scope,''), COALESCE(attempt,1),
			prompt_chars, sent_chars, context_limit_chars, utilization_pct, overloaded, shrunk, saved_pct,
			COALESCE(success, TRUE), COALESCE(error_class,''), COALESCE(delta_chars,0), metadata, created_at
		FROM omni_llm_context_usage
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LLMContextUsageEntry, 0, limit)
	for rows.Next() {
		item, err := scanLLMContextUsageEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

type llmContextUsageRowScanner interface {
	Scan(dest ...any) error
}

func scanLLMContextUsageEntry(row llmContextUsageRowScanner) (LLMContextUsageEntry, error) {
	var item LLMContextUsageEntry
	err := row.Scan(
		&item.ID, &item.Source, &item.Model, &item.Provider, &item.CardID, &item.ProjectID,
		&item.RunID, &item.JobID, &item.Scope, &item.Attempt,
		&item.PromptChars, &item.SentChars, &item.ContextLimitChars, &item.UtilizationPct,
		&item.Overloaded, &item.Shrunk, &item.SavedPct, &item.Success, &item.ErrorClass,
		&item.DeltaChars, &item.Metadata, &item.CreatedAt,
	)
	return item, err
}
