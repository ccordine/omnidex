package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

const defaultContextShrinkHistoryLimit = 100

type ContextShrinkMetricRecord struct {
	Source         string
	CardID         string
	ProjectID      int64
	RawChars       int
	ShrunkChars    int
	ChatMessages   int
	SelectedChunks int
	Metadata       any
	CreatedAt      time.Time
}

type ContextShrinkMetricSummary struct {
	Requests     int     `json:"requests"`
	AvgRawChars  float64 `json:"avg_raw_chars"`
	AvgShrunkChars float64 `json:"avg_shrunk_chars"`
	AvgSavedPct  float64 `json:"avg_saved_pct"`
	MaxRawChars  int     `json:"max_raw_chars"`
	MinShrunkChars int   `json:"min_shrunk_chars"`
}

type ContextShrinkMetricEntry struct {
	ID             string          `json:"id"`
	Source         string          `json:"source"`
	CardID         string          `json:"card_id,omitempty"`
	ProjectID      *int64          `json:"project_id,omitempty"`
	RawChars       int             `json:"raw_chars"`
	ShrunkChars    int             `json:"shrunk_chars"`
	SavedPct       float64         `json:"saved_pct"`
	ChatMessages   int             `json:"chat_messages"`
	SelectedChunks int             `json:"selected_chunks"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

type ContextShrinkDailyPoint struct {
	Day         string  `json:"day"`
	Requests    int     `json:"requests"`
	AvgRawChars float64 `json:"avg_raw_chars"`
	AvgShrunkChars float64 `json:"avg_shrunk_chars"`
	AvgSavedPct float64 `json:"avg_saved_pct"`
}

type ContextShrinkMetricsResponse struct {
	Summary ContextShrinkMetricSummary  `json:"summary"`
	History []ContextShrinkMetricEntry  `json:"history"`
	Daily   []ContextShrinkDailyPoint   `json:"daily"`
}

func contextShrinkSavedPct(raw, shrunk int) float64 {
	if raw <= 0 {
		return 0
	}
	saved := float64(raw-shrunk) / float64(raw) * 100
	if saved < 0 {
		return 0
	}
	if saved > 100 {
		return 100
	}
	return math.Round(saved*100) / 100
}

func (r *Repository) RecordContextShrinkMetric(ctx context.Context, record ContextShrinkMetricRecord) error {
	source := strings.TrimSpace(record.Source)
	if source == "" {
		return fmt.Errorf("context shrink source is required")
	}
	if record.RawChars < 0 || record.ShrunkChars < 0 {
		return fmt.Errorf("context shrink char counts must be non-negative")
	}
	created := record.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	var projectID *int64
	if record.ProjectID > 0 {
		projectID = &record.ProjectID
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO omni_context_shrink_metrics (
			source, card_id, project_id, raw_chars, shrunk_chars, saved_pct,
			chat_messages, selected_chunks, metadata, created_at
		)
		VALUES ($1, NULLIF($2,''), $3, $4, $5, $6, $7, $8, $9, $10)
	`, source, strings.TrimSpace(record.CardID), projectID, record.RawChars, record.ShrunkChars,
		contextShrinkSavedPct(record.RawChars, record.ShrunkChars),
		record.ChatMessages, record.SelectedChunks, jsonParam(record.Metadata), created)
	return err
}

func (r *Repository) ContextShrinkMetrics(ctx context.Context, source string, historyLimit int) (ContextShrinkMetricsResponse, error) {
	if historyLimit <= 0 || historyLimit > 500 {
		historyLimit = defaultContextShrinkHistoryLimit
	}
	source = strings.TrimSpace(source)

	summary := ContextShrinkMetricSummary{}
	summaryQuery := `
		SELECT COUNT(*),
			COALESCE(AVG(raw_chars), 0),
			COALESCE(AVG(shrunk_chars), 0),
			COALESCE(AVG(saved_pct), 0),
			COALESCE(MAX(raw_chars), 0),
			COALESCE(MIN(shrunk_chars), 0)
		FROM omni_context_shrink_metrics
	`
	summaryArgs := []any{}
	if source != "" {
		summaryQuery += ` WHERE source = $1`
		summaryArgs = append(summaryArgs, source)
	}
	if err := r.pool.QueryRow(ctx, summaryQuery, summaryArgs...).Scan(
		&summary.Requests,
		&summary.AvgRawChars,
		&summary.AvgShrunkChars,
		&summary.AvgSavedPct,
		&summary.MaxRawChars,
		&summary.MinShrunkChars,
	); err != nil {
		return ContextShrinkMetricsResponse{}, err
	}
	summary.AvgRawChars = math.Round(summary.AvgRawChars*10) / 10
	summary.AvgShrunkChars = math.Round(summary.AvgShrunkChars*10) / 10
	summary.AvgSavedPct = math.Round(summary.AvgSavedPct*100) / 100

	historyQuery := `
		SELECT id::text, source, COALESCE(card_id,''), project_id, raw_chars, shrunk_chars, saved_pct,
			chat_messages, selected_chunks, metadata, created_at
		FROM omni_context_shrink_metrics
	`
	historyArgs := []any{}
	if source != "" {
		historyQuery += ` WHERE source = $1`
		historyArgs = append(historyArgs, source)
	}
	historyQuery += ` ORDER BY created_at DESC LIMIT ` + fmt.Sprintf("%d", historyLimit)

	rows, err := r.pool.Query(ctx, historyQuery, historyArgs...)
	if err != nil {
		return ContextShrinkMetricsResponse{}, err
	}
	defer rows.Close()

	history := make([]ContextShrinkMetricEntry, 0, historyLimit)
	for rows.Next() {
		var item ContextShrinkMetricEntry
		if err := rows.Scan(
			&item.ID, &item.Source, &item.CardID, &item.ProjectID, &item.RawChars, &item.ShrunkChars,
			&item.SavedPct, &item.ChatMessages, &item.SelectedChunks, &item.Metadata, &item.CreatedAt,
		); err != nil {
			return ContextShrinkMetricsResponse{}, err
		}
		history = append(history, item)
	}
	if err := rows.Err(); err != nil {
		return ContextShrinkMetricsResponse{}, err
	}

	dailyQuery := `
		SELECT to_char(date_trunc('day', created_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS day,
			COUNT(*),
			COALESCE(AVG(raw_chars), 0),
			COALESCE(AVG(shrunk_chars), 0),
			COALESCE(AVG(saved_pct), 0)
		FROM omni_context_shrink_metrics
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
		return ContextShrinkMetricsResponse{}, err
	}
	defer dailyRows.Close()

	daily := make([]ContextShrinkDailyPoint, 0, 31)
	for dailyRows.Next() {
		var item ContextShrinkDailyPoint
		if err := dailyRows.Scan(&item.Day, &item.Requests, &item.AvgRawChars, &item.AvgShrunkChars, &item.AvgSavedPct); err != nil {
			return ContextShrinkMetricsResponse{}, err
		}
		item.AvgRawChars = math.Round(item.AvgRawChars*10) / 10
		item.AvgShrunkChars = math.Round(item.AvgShrunkChars*10) / 10
		item.AvgSavedPct = math.Round(item.AvgSavedPct*100) / 100
		daily = append(daily, item)
	}
	if err := dailyRows.Err(); err != nil {
		return ContextShrinkMetricsResponse{}, err
	}

	return ContextShrinkMetricsResponse{
		Summary: summary,
		History: history,
		Daily:   daily,
	}, nil
}
