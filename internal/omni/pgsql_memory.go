package omni

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type MemorySQLRunner interface {
	Exec(ctx context.Context, sql string, args ...any) error
	Query(ctx context.Context, sql string, args ...any) ([]MemorySQLRow, error)
}

type MemorySQLRow map[string]any

type PGMemoryStore struct {
	runner MemorySQLRunner
}

type MemoryRecord struct {
	ID            int64
	AgentID       string
	Source        string
	Kind          string
	Content       string
	Tags          []string
	Priority      int
	SupersededAt  time.Time
	SupersededBy  int64
	StalenessNote string
	CreatedAt     time.Time
}

func NewPGMemoryStore(runner MemorySQLRunner) *PGMemoryStore {
	return &PGMemoryStore{runner: runner}
}

func (s *PGMemoryStore) EnsureSchema(ctx context.Context) error {
	if s == nil || s.runner == nil {
		return fmt.Errorf("memory store requires SQL runner")
	}
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS tags (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS memory_chunks (
			id BIGSERIAL PRIMARY KEY,
			source TEXT NOT NULL DEFAULT 'manual',
			kind TEXT NOT NULL DEFAULT 'episodic',
			content TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE memory_chunks ADD COLUMN IF NOT EXISTS agent_id TEXT NOT NULL DEFAULT 'unknown'`,
		`ALTER TABLE memory_chunks ADD COLUMN IF NOT EXISTS priority INTEGER NOT NULL DEFAULT 100`,
		`ALTER TABLE memory_chunks ADD COLUMN IF NOT EXISTS superseded_at TIMESTAMPTZ`,
		`ALTER TABLE memory_chunks ADD COLUMN IF NOT EXISTS superseded_by BIGINT REFERENCES memory_chunks(id) ON DELETE SET NULL`,
		`ALTER TABLE memory_chunks ADD COLUMN IF NOT EXISTS staleness_note TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE IF NOT EXISTS memory_chunk_tags (
			id BIGSERIAL PRIMARY KEY,
			memory_chunk_id BIGINT NOT NULL REFERENCES memory_chunks(id) ON DELETE CASCADE,
			tag_id BIGINT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(memory_chunk_id, tag_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_chunks_agent_kind_created ON memory_chunks(agent_id, kind, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_chunks_content_trgm ON memory_chunks USING gin (content gin_trgm_ops)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_chunk_tags_tag_id ON memory_chunk_tags(tag_id, memory_chunk_id)`,
	} {
		if err := s.runner.Exec(ctx, stmt); err != nil {
			if strings.Contains(err.Error(), "gin_trgm_ops") {
				continue
			}
			return err
		}
	}
	return nil
}

func (s *PGMemoryStore) AddMemory(ctx context.Context, agentID, kind, content string, tags []string) (MemoryRecord, error) {
	agentID = strings.TrimSpace(agentID)
	kind = strings.TrimSpace(kind)
	content = strings.TrimSpace(content)
	tags = cleanMemoryTags(tags)
	if agentID == "" {
		return MemoryRecord{}, fmt.Errorf("agent id is required")
	}
	if kind == "" {
		kind = "episodic"
	}
	if content == "" {
		return MemoryRecord{}, fmt.Errorf("memory content is required")
	}

	rows, err := s.runner.Query(ctx, `
		INSERT INTO memory_chunks (agent_id, source, kind, content)
		VALUES ($1, $2, $3, $4)
		RETURNING id, agent_id, source, kind, content, created_at
	`, agentID, "omni", kind, content)
	if err != nil {
		return MemoryRecord{}, err
	}
	if len(rows) != 1 {
		return MemoryRecord{}, fmt.Errorf("memory insert returned %d rows", len(rows))
	}
	record := memoryRecordFromRow(rows[0])

	for _, tag := range tags {
		tagRows, err := s.runner.Query(ctx, `
			INSERT INTO tags (name)
			VALUES ($1)
			ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id
		`, tag)
		if err != nil {
			return MemoryRecord{}, err
		}
		if len(tagRows) != 1 {
			return MemoryRecord{}, fmt.Errorf("tag upsert returned %d rows", len(tagRows))
		}
		tagID := int64FromAny(tagRows[0]["id"])
		if err := s.runner.Exec(ctx, `
			INSERT INTO memory_chunk_tags (memory_chunk_id, tag_id)
			VALUES ($1, $2)
			ON CONFLICT (memory_chunk_id, tag_id) DO NOTHING
		`, record.ID, tagID); err != nil {
			return MemoryRecord{}, err
		}
	}
	record.Tags = tags
	return record, nil
}

func (s *PGMemoryStore) UpdateMemoryContent(ctx context.Context, id int64, agentID, content string, tags []string) (MemoryRecord, error) {
	if s == nil || s.runner == nil {
		return MemoryRecord{}, fmt.Errorf("memory store requires SQL runner")
	}
	agentID = strings.TrimSpace(agentID)
	content = strings.TrimSpace(content)
	tags = cleanMemoryTags(tags)
	if id <= 0 {
		return MemoryRecord{}, fmt.Errorf("memory id is required")
	}
	if agentID == "" {
		return MemoryRecord{}, fmt.Errorf("agent id is required")
	}
	if content == "" {
		return MemoryRecord{}, fmt.Errorf("memory content is required")
	}
	rows, err := s.runner.Query(ctx, `
		UPDATE memory_chunks
		SET content = $2,
		    agent_id = $3,
		    priority = GREATEST(priority, 100),
		    superseded_at = NULL,
		    superseded_by = NULL,
		    staleness_note = ''
		WHERE id = $1
		RETURNING id, agent_id, source, kind, content, priority, superseded_at, superseded_by, staleness_note, created_at
	`, id, content, agentID)
	if err != nil {
		return MemoryRecord{}, err
	}
	if len(rows) != 1 {
		return MemoryRecord{}, fmt.Errorf("memory update returned %d rows", len(rows))
	}
	record := memoryRecordFromRow(rows[0])
	for _, tag := range tags {
		tagRows, err := s.runner.Query(ctx, `
			INSERT INTO tags (name)
			VALUES ($1)
			ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id
		`, tag)
		if err != nil {
			return MemoryRecord{}, err
		}
		if len(tagRows) != 1 {
			return MemoryRecord{}, fmt.Errorf("tag upsert returned %d rows", len(tagRows))
		}
		tagID := int64FromAny(tagRows[0]["id"])
		if err := s.runner.Exec(ctx, `
			INSERT INTO memory_chunk_tags (memory_chunk_id, tag_id)
			VALUES ($1, $2)
			ON CONFLICT (memory_chunk_id, tag_id) DO NOTHING
		`, record.ID, tagID); err != nil {
			return MemoryRecord{}, err
		}
	}
	record.Tags = append(record.Tags, tags...)
	record.Tags = cleanMemoryTags(record.Tags)
	return record, nil
}

func (s *PGMemoryStore) DeprioritizeMemory(ctx context.Context, id int64, supersededBy int64, note string) (MemoryRecord, error) {
	if s == nil || s.runner == nil {
		return MemoryRecord{}, fmt.Errorf("memory store requires SQL runner")
	}
	note = strings.TrimSpace(note)
	if id <= 0 {
		return MemoryRecord{}, fmt.Errorf("memory id is required")
	}
	if note == "" {
		return MemoryRecord{}, fmt.Errorf("staleness note is required")
	}
	rows, err := s.runner.Query(ctx, `
		UPDATE memory_chunks
		SET priority = LEAST(priority, 10),
		    superseded_at = NOW(),
		    superseded_by = NULLIF($2, 0),
		    staleness_note = $3
		WHERE id = $1
		RETURNING id, agent_id, source, kind, content, priority, superseded_at, superseded_by, staleness_note, created_at
	`, id, supersededBy, note)
	if err != nil {
		return MemoryRecord{}, err
	}
	if len(rows) != 1 {
		return MemoryRecord{}, fmt.Errorf("memory deprioritize returned %d rows", len(rows))
	}
	return memoryRecordFromRow(rows[0]), nil
}

func (s *PGMemoryStore) ListTags(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.runner.Query(ctx, `
		SELECT name
		FROM tags
		ORDER BY name ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	tags := make([]string, 0, len(rows))
	for _, row := range rows {
		tags = append(tags, stringFromAny(row["name"]))
	}
	return tags, nil
}

func (s *PGMemoryStore) SearchMemory(ctx context.Context, query string, tags []string, limit int) ([]MemoryRecord, error) {
	query = strings.TrimSpace(query)
	tags = cleanMemoryTags(tags)
	if limit <= 0 {
		limit = 8
	}
	if query == "" && len(tags) == 0 {
		return nil, fmt.Errorf("memory search requires query or tags")
	}

	rows, err := s.runner.Query(ctx, `
		SELECT mc.id, mc.agent_id, mc.source, mc.kind, mc.content, mc.priority,
		       mc.superseded_at, mc.superseded_by, mc.staleness_note, mc.created_at,
		       COALESCE(array_remove(array_agg(DISTINCT t.name), NULL), ARRAY[]::text[]) AS tags
		FROM memory_chunks mc
		LEFT JOIN memory_chunk_tags mct ON mct.memory_chunk_id = mc.id
		LEFT JOIN tags t ON t.id = mct.tag_id
		WHERE ($1 = '' OR mc.content ILIKE '%' || $1 || '%')
		  AND (
			cardinality($2::text[]) = 0 OR EXISTS (
				SELECT 1
				FROM memory_chunk_tags fmct
				JOIN tags ft ON ft.id = fmct.tag_id
				WHERE fmct.memory_chunk_id = mc.id
				  AND ft.name = ANY($2::text[])
			)
		  )
		GROUP BY mc.id, mc.agent_id, mc.source, mc.kind, mc.content, mc.created_at
		ORDER BY mc.priority DESC, mc.created_at DESC, mc.id DESC
		LIMIT $3
	`, query, tags, limit)
	if err != nil {
		return nil, err
	}

	records := make([]MemoryRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, memoryRecordFromRow(row))
	}
	return records, nil
}

type PgxMemoryRunner struct {
	pool *pgxpool.Pool
}

func NewPgxMemoryRunner(pool *pgxpool.Pool) *PgxMemoryRunner {
	return &PgxMemoryRunner{pool: pool}
}

func (r *PgxMemoryRunner) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := r.pool.Exec(ctx, sql, args...)
	return err
}

func (r *PgxMemoryRunner) Query(ctx context.Context, sql string, args ...any) ([]MemorySQLRow, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fieldDescriptions := rows.FieldDescriptions()
	out := []MemorySQLRow{}
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := MemorySQLRow{}
		for i, field := range fieldDescriptions {
			row[string(field.Name)] = values[i]
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func cleanMemoryTags(tags []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		clean := strings.ToLower(strings.TrimSpace(tag))
		clean = strings.ReplaceAll(clean, " ", "-")
		if clean == "" {
			continue
		}
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	sort.Strings(out)
	return out
}

func memoryRecordFromRow(row MemorySQLRow) MemoryRecord {
	priority := int(int64FromAny(row["priority"]))
	if priority == 0 {
		priority = 100
	}
	return MemoryRecord{
		ID:            int64FromAny(row["id"]),
		AgentID:       stringFromAny(row["agent_id"]),
		Source:        stringFromAny(row["source"]),
		Kind:          stringFromAny(row["kind"]),
		Content:       stringFromAny(row["content"]),
		Tags:          stringSliceFromAny(row["tags"]),
		Priority:      priority,
		SupersededAt:  timeFromAny(row["superseded_at"]),
		SupersededBy:  int64FromAny(row["superseded_by"]),
		StalenessNote: stringFromAny(row["staleness_note"]),
		CreatedAt:     timeFromAny(row["created_at"]),
	}
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	default:
		return 0
	}
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%v", value)
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, stringFromAny(item))
		}
		return out
	default:
		return nil
	}
}

func timeFromAny(value any) time.Time {
	if typed, ok := value.(time.Time); ok {
		return typed
	}
	return time.Time{}
}
