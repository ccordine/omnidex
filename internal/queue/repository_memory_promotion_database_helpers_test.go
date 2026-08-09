package queue

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func writePromotionTestCandidate(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	jobID, generation int64,
	content string,
) model.MemoryCandidate {
	t.Helper()
	candidate := model.MemoryCandidate{
		JobID: jobID, Generation: &generation, CandidateKind: model.MemoryKindReference,
		Content: content, Provenance: []byte(`{"scope_tags":["project:test"]}`), Confidence: 0.9,
		Status: model.MemoryCandidateStatusCandidate,
	}
	id, err := repository.WriteMemoryCandidate(ctx, candidate)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := repository.GetMemoryCandidate(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return persisted
}

func promotionTestEmbedding() []float64 {
	values := make([]float64, 768)
	values[0] = 1
	return values
}

func assertPromotedCandidateState(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	candidateID int64,
	wantStatus string,
	wantMemoryID int64,
) {
	t.Helper()
	var status string
	var memoryID *int64
	if err := pool.QueryRow(ctx, `
		SELECT status, promoted_memory_id FROM memory_candidates WHERE id=$1
	`, candidateID).Scan(&status, &memoryID); err != nil {
		t.Fatal(err)
	}
	if status != wantStatus || memoryID == nil || *memoryID != wantMemoryID {
		t.Fatalf("candidate status=%q memory=%v want status=%q memory=%d", status, memoryID, wantStatus, wantMemoryID)
	}
}

func assertUnpromotedCandidateState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, candidateID int64) {
	t.Helper()
	var status string
	var memoryID *int64
	if err := pool.QueryRow(ctx, `
		SELECT status, promoted_memory_id FROM memory_candidates WHERE id=$1
	`, candidateID).Scan(&status, &memoryID); err != nil {
		t.Fatal(err)
	}
	if status != model.MemoryCandidateStatusCandidate || memoryID != nil {
		t.Fatalf("candidate status=%q memory=%v, want unpromoted candidate", status, memoryID)
	}
}

func promotionChunkCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, content string) int64 {
	t.Helper()
	var count int64
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM memory_chunks WHERE content=$1`, content).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func promotionChunkTagCount(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	memoryID int64,
	tag string,
) int64 {
	t.Helper()
	var count int64
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM memory_chunk_tags AS links
		JOIN tags ON tags.id=links.tag_id
		WHERE links.memory_chunk_id=$1 AND tags.name=$2
	`, memoryID, tag).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func quotePostgresLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
