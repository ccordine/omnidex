package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresMemoryBatchRollsBackEveryChunkOnLaterFailure(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	scope := createMemoryScopeForTest(t, repository)
	if _, err := pool.Exec(t.Context(), `
		CREATE FUNCTION reject_second_memory_chunk() RETURNS TRIGGER AS $$
		BEGIN
			IF NEW.content='reject-second' THEN
				RAISE EXCEPTION 'injected second memory failure';
			END IF;
			RETURN NEW;
		END $$ LANGUAGE plpgsql;
		CREATE TRIGGER reject_second_memory_chunk
		BEFORE INSERT ON memory_chunks FOR EACH ROW EXECUTE FUNCTION reject_second_memory_chunk();
	`); err != nil {
		t.Fatal(err)
	}
	writes := []MemoryChunkWrite{
		{Input: memoryInputInScope(scope, "first-memory")},
		{Input: memoryInputInScope(scope, "reject-second")},
	}
	if _, err := repository.AddMemoryChunks(t.Context(), writes); err == nil ||
		!strings.Contains(err.Error(), "injected second memory failure") {
		t.Fatalf("batch error=%v", err)
	}
	var count int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM memory_chunks WHERE content IN ('first-memory','reject-second')
	`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed memory batch persisted %d chunks", count)
	}
}

func TestPostgresNearVectorNeverOverwritesMemoryAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	scope := createMemoryScopeForTest(t, repository)
	first := memoryInputInScope(scope, "first exact authority")
	first.Kind = model.MemoryKind(model.MemoryKindInstruction)
	first.Source = "source:first"
	second := memoryInputInScope(scope, "second exact authority")
	second.Kind = model.MemoryKind(model.MemoryKindInstruction)
	second.Source = "source:second"
	embedding := make([]float64, 768)
	embedding[0] = 1
	chunks, err := repository.AddMemoryChunks(t.Context(), []MemoryChunkWrite{
		{Input: first, Embedding: embedding},
		{Input: second, Embedding: embedding},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 || chunks[0].ID == chunks[1].ID {
		t.Fatalf("near-vector writes=%+v", chunks)
	}
	var source model.MemorySource
	var content string
	if err := pool.QueryRow(t.Context(), `
		SELECT source,content FROM memory_chunks WHERE id=$1
	`, chunks[0].ID).Scan(&source, &content); err != nil {
		t.Fatal(err)
	}
	if source != first.Source || content != first.Content {
		t.Fatalf("first authority changed to source=%q content=%q", source, content)
	}
}

func TestPostgresExactDuplicateContentKeepsDistinctSourceAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	scope := createMemoryScopeForTest(t, repository)
	first := memoryInputInScope(scope, "same exact content")
	first.Source = "source:first"
	second := memoryInputInScope(scope, "same exact content")
	second.Source = "source:second"
	chunks, err := repository.AddMemoryChunks(t.Context(), []MemoryChunkWrite{
		{Input: first}, {Input: second},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 || chunks[0].ID == chunks[1].ID ||
		chunks[0].Source != first.Source || chunks[1].Source != second.Source {
		t.Fatalf("duplicate content collapsed source authority: %+v", chunks)
	}
}

func TestPostgresMemoryCandidateBatchRollsBackOnLaterFailure(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	scope := createMemoryScopeForTest(t, repository)
	if _, err := pool.Exec(t.Context(), `
		CREATE FUNCTION reject_second_memory_candidate() RETURNS TRIGGER AS $$
		BEGIN
			IF NEW.content='reject-candidate' THEN
				RAISE EXCEPTION 'injected candidate batch failure';
			END IF;
			RETURN NEW;
		END $$ LANGUAGE plpgsql;
		CREATE TRIGGER reject_second_memory_candidate
		BEFORE INSERT ON memory_candidates FOR EACH ROW
		EXECUTE FUNCTION reject_second_memory_candidate();
	`); err != nil {
		t.Fatal(err)
	}
	candidates := []model.MemoryCandidate{
		{Scope: scope, CandidateKind: model.MemoryKindReference, Content: "accepted-candidate", Provenance: []byte(`{}`), Confidence: 0.8},
		{Scope: scope, CandidateKind: model.MemoryKindReference, Content: "reject-candidate", Provenance: []byte(`{}`), Confidence: 0.8},
	}
	if _, err := repository.WriteMemoryCandidates(t.Context(), candidates); err == nil ||
		!strings.Contains(err.Error(), "injected candidate batch failure") {
		t.Fatalf("candidate batch error=%v", err)
	}
	var count int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM memory_candidates
		WHERE content IN ('accepted-candidate','reject-candidate')
	`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed candidate batch persisted %d rows", count)
	}
}
