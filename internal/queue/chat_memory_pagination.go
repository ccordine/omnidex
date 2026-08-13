package queue

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

const maxChatUIPageSize = 100

type MemoryChunkPage struct {
	Items      []MemoryChunkSummary
	NextOffset *int
	HasMore    bool
}

type MemoryCandidatePage struct {
	Items      []model.MemoryCandidate
	NextOffset *int
	HasMore    bool
}

func (r *Repository) ListMemoryChunkPage(
	ctx context.Context,
	kind string,
	tags []string,
	limit, offset int,
) (MemoryChunkPage, error) {
	if err := validateChatUIPageBounds(limit, offset); err != nil {
		return MemoryChunkPage{}, err
	}
	if err := validateChatMemoryKind(kind); err != nil {
		return MemoryChunkPage{}, err
	}
	tags = cleanTags(tags)
	rows, err := r.pool.Query(ctx, `
		SELECT mc.id, mc.project_id, mc.channel_id, mc.source, mc.kind, mc.content, mc.created_at,
		       COALESCE(array_remove(array_agg(DISTINCT t.name), NULL), ARRAY[]::text[]) AS tags,
		       COALESCE(array_remove(array_agg(DISTINCT c.name), NULL), ARRAY[]::text[]) AS categories
		FROM memory_chunks mc
		LEFT JOIN memory_chunk_tags mct ON mct.memory_chunk_id=mc.id
		LEFT JOIN tags t ON t.id=mct.tag_id
		LEFT JOIN memory_chunk_categories mcc ON mcc.memory_chunk_id=mc.id
		LEFT JOIN memory_categories c ON c.id=mcc.category_id
		WHERE (NULLIF($1, '') IS NULL OR mc.kind=$1)
		  AND (cardinality($2::text[])=0 OR EXISTS (
			SELECT 1 FROM memory_chunk_tags fmct
			JOIN tags ft ON ft.id=fmct.tag_id
			WHERE fmct.memory_chunk_id=mc.id AND ft.name=ANY($2)
		  ))
		GROUP BY mc.id
		ORDER BY mc.created_at DESC, mc.id DESC
		LIMIT $3 OFFSET $4
	`, kind, tags, limit+1, offset)
	if err != nil {
		return MemoryChunkPage{}, err
	}
	defer rows.Close()
	items := make([]MemoryChunkSummary, 0, limit+1)
	for rows.Next() {
		var item MemoryChunkSummary
		if err := rows.Scan(&item.ID, &item.Scope.ProjectID, &item.Scope.ChannelID,
			&item.Source, &item.Kind, &item.Content,
			&item.CreatedAt, &item.Tags, &item.Categories); err != nil {
			return MemoryChunkPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return MemoryChunkPage{}, err
	}
	items, next, hasMore := boundedChatUIPage(items, limit, offset)
	return MemoryChunkPage{Items: items, NextOffset: next, HasMore: hasMore}, nil
}

func (r *Repository) ListHistoricalMemoryCandidatePage(
	ctx context.Context,
	jobID int64,
	status string,
	limit, offset int,
) (MemoryCandidatePage, error) {
	if jobID < 0 {
		return MemoryCandidatePage{}, fmt.Errorf("memory candidate job ID must not be negative")
	}
	if err := validateChatUIPageBounds(limit, offset); err != nil {
		return MemoryCandidatePage{}, err
	}
	if err := validateChatMemoryCandidateStatus(status); err != nil {
		return MemoryCandidatePage{}, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, project_id, channel_id, job_id, generation, source_memory_id, candidate_kind, content,
		       provenance, confidence, status, created_at, updated_at
		FROM memory_candidates
		WHERE ($1=0 OR job_id=$1) AND ($2='' OR status=$2)
		ORDER BY confidence DESC, id ASC
		LIMIT $3 OFFSET $4
	`, jobID, status, limit+1, offset)
	if err != nil {
		return MemoryCandidatePage{}, err
	}
	defer rows.Close()
	items := make([]model.MemoryCandidate, 0, limit+1)
	for rows.Next() {
		var item model.MemoryCandidate
		var nullableJobID *int64
		if err := rows.Scan(&item.ID, &item.Scope.ProjectID, &item.Scope.ChannelID,
			&nullableJobID, &item.Generation, &item.SourceMemoryID,
			&item.CandidateKind, &item.Content, &item.Provenance, &item.Confidence,
			&item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return MemoryCandidatePage{}, err
		}
		if nullableJobID != nil {
			item.JobID = *nullableJobID
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return MemoryCandidatePage{}, err
	}
	items, next, hasMore := boundedChatUIPage(items, limit, offset)
	return MemoryCandidatePage{Items: items, NextOffset: next, HasMore: hasMore}, nil
}

func validateChatUIPageBounds(limit, offset int) error {
	if limit < 1 || limit > maxChatUIPageSize {
		return fmt.Errorf("chat UI page limit must be between 1 and %d", maxChatUIPageSize)
	}
	if offset < 0 {
		return fmt.Errorf("chat UI page offset must not be negative")
	}
	return nil
}

func validateChatMemoryKind(kind string) error {
	if kind != strings.TrimSpace(kind) {
		return fmt.Errorf("chat memory kind must be canonical")
	}
	switch kind {
	case "", model.MemoryKindEpisodic, model.MemoryKindProcedural, model.MemoryKindInstruction,
		model.MemoryKindPreference, model.MemoryKindReference:
		return nil
	default:
		return fmt.Errorf("unsupported chat memory kind %q", kind)
	}
}

func validateChatMemoryCandidateStatus(status string) error {
	if status != strings.TrimSpace(status) {
		return fmt.Errorf("chat memory candidate status must be canonical")
	}
	switch status {
	case "", model.MemoryCandidateStatusCandidate, model.MemoryCandidateStatusApproved,
		model.MemoryCandidateStatusDurable, model.MemoryCandidateStatusRejected:
		return nil
	default:
		return fmt.Errorf("unsupported chat memory candidate status %q", status)
	}
}

func boundedChatUIPage[T any](items []T, limit, offset int) ([]T, *int, bool) {
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
		next := offset + limit
		return items, &next, true
	}
	return items, nil, false
}
