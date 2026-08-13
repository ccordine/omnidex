package queue

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresCurrentMemoryPromotionIsAtomicAndIdempotent(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("atomic-memory-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	scope := createMemoryScopeForTest(t, repository)
	candidate := writePromotionTestCandidate(t, ctx, repository, scope, job.ID, job.CurrentGeneration, marker)
	request := MemoryCandidatePromotion{
		Candidate: candidate,
		Tier:      model.MemoryCandidateStatusDurable,
		Tags:      []string{"test:atomic-promotion"},
		Embedding: promotionTestEmbedding(),
	}

	first, err := repository.PromoteCurrentMemoryCandidate(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.PromoteCurrentMemoryCandidate(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Memory == nil || second.Memory == nil || first.Memory.ID != second.Memory.ID {
		t.Fatalf("promotion was not idempotent: first=%+v second=%+v", first, second)
	}
	assertPromotedCandidateState(t, ctx, pool, candidate.ID, model.MemoryCandidateStatusDurable, first.Memory.ID)
	if count := promotionChunkCount(t, ctx, pool, marker); count != 1 {
		t.Fatalf("promoted chunks=%d want 1", count)
	}
	if count := promotionChunkTagCount(t, ctx, pool, first.Memory.ID, "test:atomic-promotion"); count != 1 {
		t.Fatalf("promoted chunk tag links=%d want 1", count)
	}
	if _, err := repository.ReplanJob(ctx, testReplanCommand(t, job.ID, "retire-promoted", "Retire the already-promoted candidate generation.")); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.PromoteHistoricalMemoryCandidate(ctx, request); err == nil {
		t.Fatal("different promotion authority replay was accepted")
	}
	if count := promotionChunkCount(t, ctx, pool, marker); count != 1 {
		t.Fatalf("authority mismatch changed promoted chunks to %d", count)
	}
}

func TestPostgresCurrentMemoryPromotionRejectsRetiredGenerationWithoutPublishing(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("stale-memory-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	scope := createMemoryScopeForTest(t, repository)
	candidate := writePromotionTestCandidate(t, ctx, repository, scope, job.ID, job.CurrentGeneration, marker)
	if _, err := repository.ReplanJob(ctx, testReplanCommand(t, job.ID, "retire-candidate", "Retire the memory candidate generation.")); err != nil {
		t.Fatal(err)
	}
	request := MemoryCandidatePromotion{
		Candidate: candidate, Tier: model.MemoryCandidateStatusApproved,
		Embedding: promotionTestEmbedding(),
	}
	if _, err := repository.PromoteCurrentMemoryCandidate(ctx, request); !errors.Is(err, ErrStaleJobGeneration) {
		t.Fatalf("stale promotion error=%v", err)
	}
	assertUnpromotedCandidateState(t, ctx, pool, candidate.ID)
	if count := promotionChunkCount(t, ctx, pool, marker); count != 0 {
		t.Fatalf("stale promotion published %d chunks", count)
	}

	result, err := repository.PromoteHistoricalMemoryCandidate(ctx, request)
	if err != nil {
		t.Fatalf("explicit historical promotion: %v", err)
	}
	if result.Memory == nil {
		t.Fatal("explicit historical promotion returned no memory")
	}
}

func TestPostgresMemoryPromotionRollsBackChunkAndTagsWhenCandidateUpdateFails(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("rollback-memory-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	scope := createMemoryScopeForTest(t, repository)
	candidate := writePromotionTestCandidate(t, ctx, repository, scope, job.ID, job.CurrentGeneration, marker)
	constraint := fmt.Sprintf("reject_promotion_%d", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		ALTER TABLE memory_candidates ADD CONSTRAINT %s
		CHECK (content <> %s OR promoted_memory_id IS NULL)
	`, constraint, quotePostgresLiteral(marker))); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), fmt.Sprintf(
			"ALTER TABLE memory_candidates DROP CONSTRAINT IF EXISTS %s", constraint,
		))
	})

	_, err = repository.PromoteCurrentMemoryCandidate(ctx, MemoryCandidatePromotion{
		Candidate: candidate, Tier: model.MemoryCandidateStatusApproved,
		Tags: []string{"test:must-roll-back"}, Embedding: promotionTestEmbedding(),
	})
	if err == nil {
		t.Fatal("candidate update constraint did not reject promotion")
	}
	assertUnpromotedCandidateState(t, ctx, pool, candidate.ID)
	if count := promotionChunkCount(t, ctx, pool, marker); count != 0 {
		t.Fatalf("failed promotion left %d memory chunks", count)
	}
}

func TestPostgresGlobalMemoryPromotionRequiresExplicitGlobalPath(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("global-memory-%d", time.Now().UnixNano())
	scope := createMemoryScopeForTest(t, repository)
	id, err := repository.WriteMemoryCandidate(ctx, model.MemoryCandidate{
		Scope:         scope,
		CandidateKind: model.MemoryKindReference,
		Content:       marker,
		Provenance:    []byte(`{"scope_tags":["global:test"]}`),
		Confidence:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := repository.GetMemoryCandidate(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	request := MemoryCandidatePromotion{
		Candidate: candidate, Tier: model.MemoryCandidateStatusApproved,
		Embedding: promotionTestEmbedding(),
	}
	if _, err := repository.PromoteCurrentMemoryCandidate(ctx, request); err == nil {
		t.Fatal("current-generation path accepted a global candidate")
	}
	if count := promotionChunkCount(t, ctx, pool, marker); count != 0 {
		t.Fatalf("wrong authority path published %d global chunks", count)
	}
	result, err := repository.PromoteGlobalMemoryCandidate(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Memory == nil {
		t.Fatal("explicit global promotion returned no memory")
	}
}
