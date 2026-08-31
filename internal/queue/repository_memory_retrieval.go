package queue

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// HasScopedMemory reports whether exact scoped retrieval has any durable
// candidate before the caller spends an embedding provider request. It is a
// code-owned existence check; it does not inspect or classify memory content.
func (r *Repository) HasScopedMemory(ctx context.Context, scope model.MemoryScope) (bool, error) {
	if err := scope.Validate(); err != nil {
		return false, err
	}
	if r == nil || r.pool == nil {
		return false, fmt.Errorf("memory retrieval requires PostgreSQL")
	}
	var exists bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM memory_chunks
			WHERE project_id=$1 AND channel_id=$2 AND embedding IS NOT NULL
		)
	`, scope.ProjectID, scope.ChannelID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check exact scoped memory candidates: %w", err)
	}
	return exists, nil
}

// HasAdditionalScopedMemory reports whether term-directed retrieval can add
// exact memory content beyond the mechanically acquired context. Distinct
// durable identities do not create a semantic question when their bytes would
// be removed by the context compiler's exact-content deduplication.
func (r *Repository) HasAdditionalScopedMemory(
	ctx context.Context,
	scope model.MemoryScope,
	representedContents []string,
) (bool, error) {
	if ctx == nil {
		return false, fmt.Errorf("additional scoped memory requires a context")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := scope.Validate(); err != nil {
		return false, err
	}
	if r == nil || r.pool == nil {
		return false, fmt.Errorf("memory retrieval requires PostgreSQL")
	}
	for index, content := range representedContents {
		if strings.TrimSpace(content) == "" || !utf8.ValidString(content) ||
			strings.ContainsRune(content, '\x00') {
			return false, fmt.Errorf("represented context content %d is invalid", index)
		}
	}
	var exists bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM memory_chunks
			WHERE project_id=$1 AND channel_id=$2 AND embedding IS NOT NULL
			  AND NOT (content=ANY($3::text[]))
		)
	`, scope.ProjectID, scope.ChannelID, representedContents).Scan(&exists); err != nil {
		return false, fmt.Errorf("check additional exact scoped memory candidates: %w", err)
	}
	return exists, nil
}

func (r *Repository) FindRelevantMemory(
	ctx context.Context,
	scope model.MemoryScope,
	embedding []float64,
	limit int,
) ([]model.MemoryMatch, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 8 {
		return nil, fmt.Errorf("memory retrieval limit must be within 1..8")
	}
	if len(embedding) != model.MemoryEmbeddingDimensions {
		return nil, fmt.Errorf("memory retrieval embedding must contain exactly %d values", model.MemoryEmbeddingDimensions)
	}
	for _, value := range embedding {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("memory retrieval embedding values must be finite")
		}
	}
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("memory retrieval requires PostgreSQL")
	}
	rows, err := r.pool.Query(ctx, memoryCosineRetrievalSQL,
		scope.ProjectID, scope.ChannelID, embedding, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	matches := make([]model.MemoryMatch, 0, limit)
	for rows.Next() {
		var match model.MemoryMatch
		var categoryNames []string
		if err := rows.Scan(&match.ID, &match.Scope.ProjectID, &match.Scope.ChannelID,
			&match.Kind, &match.Content, &match.CreatedAt,
			&match.Tags, &categoryNames, &match.Score); err != nil {
			return nil, err
		}
		if _, err := model.ParseMemoryKind(string(match.Kind)); err != nil {
			return nil, fmt.Errorf("memory %d has invalid durable kind: %w", match.ID, err)
		}
		if err := model.ValidateMemoryTags(match.Tags); err != nil {
			return nil, fmt.Errorf("memory %d has invalid durable tags: %w", match.ID, err)
		}
		if len(categoryNames) == 0 {
			return nil, fmt.Errorf("memory %d has no durable structured category", match.ID)
		}
		match.Categories, err = model.ParseMemoryCategories(categoryNames)
		if err != nil {
			return nil, fmt.Errorf("memory %d has invalid durable categories: %w", match.ID, err)
		}
		matches = append(matches, match)
	}
	return matches, rows.Err()
}

const memoryCosineRetrievalSQL = `
	SELECT mc.id,mc.project_id,mc.channel_id,mc.kind,mc.content,mc.created_at,
		COALESCE((SELECT array_agg(DISTINCT t.name ORDER BY t.name)
			FROM memory_chunk_tags mct JOIN tags t ON t.id=mct.tag_id
			WHERE mct.memory_chunk_id=mc.id),ARRAY[]::text[]),
		COALESCE((SELECT array_agg(DISTINCT c.name ORDER BY c.name)
			FROM memory_chunk_categories mcc JOIN memory_categories c ON c.id=mcc.category_id
			WHERE mcc.memory_chunk_id=mc.id),ARRAY[]::text[]),
		COALESCE((
			SELECT SUM(stored.value*query.value) /
				NULLIF(SQRT(SUM(stored.value*stored.value))*SQRT(SUM(query.value*query.value)),0)
			FROM unnest(mc.embedding) WITH ORDINALITY AS stored(value,ordinal)
			JOIN unnest($3::double precision[]) WITH ORDINALITY AS query(value,ordinal)
			USING (ordinal)
		),0) AS score
	FROM memory_chunks mc
	WHERE mc.project_id=$1 AND mc.channel_id=$2 AND mc.embedding IS NOT NULL
	ORDER BY score DESC,mc.id ASC
	LIMIT $4`

func (r *Repository) ListMemoryCategories(ctx context.Context, limit int) ([]model.MemoryFacet, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT c.name,COUNT(mcc.memory_chunk_id)::bigint FROM memory_categories c
		JOIN memory_chunk_categories mcc ON mcc.category_id=c.id
		GROUP BY c.id,c.name ORDER BY COUNT(mcc.memory_chunk_id) DESC,c.name LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemoryFacets(rows)
}

func (r *Repository) ListMemoryTags(ctx context.Context, limit int) ([]model.MemoryFacet, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT t.name,COUNT(mct.memory_chunk_id)::bigint FROM tags t
		JOIN memory_chunk_tags mct ON mct.tag_id=t.id
		GROUP BY t.id,t.name ORDER BY COUNT(mct.memory_chunk_id) DESC,t.name LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemoryFacets(rows)
}

func scanMemoryFacets(rows pgx.Rows) ([]model.MemoryFacet, error) {
	facets := []model.MemoryFacet{}
	for rows.Next() {
		var facet model.MemoryFacet
		if err := rows.Scan(&facet.Name, &facet.Count); err != nil {
			return nil, err
		}
		facets = append(facets, facet)
	}
	return facets, rows.Err()
}
