package queue

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

type MemoryChunkSummary struct {
	ID         int64           `json:"id"`
	Source     string          `json:"source"`
	Kind       string          `json:"kind"`
	Content    string          `json:"content"`
	Tags       []string        `json:"tags,omitempty"`
	Categories []string        `json:"categories,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}

func (r *Repository) ListMemoryChunks(ctx context.Context, kind string, tags []string, limit int) ([]MemoryChunkSummary, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	kind = strings.TrimSpace(kind)
	tags = cleanTags(tags)
	rows, err := r.pool.Query(ctx, `
		SELECT
			mc.id,
			mc.source,
			mc.kind,
			mc.content,
			mc.created_at,
			COALESCE(array_remove(array_agg(DISTINCT t.name), NULL), ARRAY[]::text[]) AS tags,
			COALESCE(array_remove(array_agg(DISTINCT c.name), NULL), ARRAY[]::text[]) AS categories
		FROM memory_chunks mc
		LEFT JOIN memory_chunk_tags mct ON mct.memory_chunk_id = mc.id
		LEFT JOIN tags t ON t.id = mct.tag_id
		LEFT JOIN memory_chunk_categories mcc ON mcc.memory_chunk_id = mc.id
		LEFT JOIN memory_categories c ON c.id = mcc.category_id
		WHERE (NULLIF($1, '') IS NULL OR mc.kind = $1)
		  AND (
			cardinality($2::text[]) = 0 OR EXISTS (
				SELECT 1
				FROM memory_chunk_tags fmct
				JOIN tags ft ON ft.id = fmct.tag_id
				WHERE fmct.memory_chunk_id = mc.id
				  AND ft.name = ANY($2)
			)
		  )
		GROUP BY mc.id
		ORDER BY mc.created_at DESC, mc.id DESC
		LIMIT $3
	`, kind, tags, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MemoryChunkSummary{}
	for rows.Next() {
		var item MemoryChunkSummary
		if err := rows.Scan(&item.ID, &item.Source, &item.Kind, &item.Content, &item.CreatedAt, &item.Tags, &item.Categories); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) ListTelemetryEvents(ctx context.Context, limit int) ([]TelemetryEventSummary, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, run_id::text, step, event_type, created_at, payload
		FROM omni_run_events
		ORDER BY created_at DESC, id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TelemetryEventSummary{}
	for rows.Next() {
		var item TelemetryEventSummary
		if err := rows.Scan(&item.ID, &item.RunID, &item.Step, &item.EventType, &item.CreatedAt, &item.Payload); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
