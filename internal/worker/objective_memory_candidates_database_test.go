package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/llmprovider"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestPostgresEmptyScopedMemoryNeedsNoConfiguredProvider(t *testing.T) {
	ctx, repository, pool := openRepositoryTestDatabase(t)
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: "empty-memory-no-embedding", Scope: model.ChannelScopeUser, Mode: model.ChannelModeAssistant,
		Name: "Empty memory", WorkspaceRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, job, err := repository.EnqueueChannelTurn(ctx, channel.ID, "Answer without durable memory.")
	if err != nil {
		t.Fatal(err)
	}
	transports := llmprovider.NewLazyFromConfig(config.Config{
		InferenceContextTokens: llm.DefaultInferenceContextTokens,
	})
	runtime := &nativeRuntimeV3{
		ctx: ctx, svc: &Service{
			repo: repository, stationClient: transports.Stations, embeddings: transports.Embeddings,
		},
	}
	set, err := (runtimeConversationCandidateProvider{runtime: runtime}).MemoryCandidates(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Candidates) != 0 || set.Replan != nil {
		t.Fatalf("empty scope returned objective context: %+v", set)
	}
	var gaps, discoveries, calls, evidence int
	if err := pool.QueryRow(ctx, `
		SELECT
		 (SELECT count(*) FROM station_gap_openings WHERE job_id=$1),
		 (SELECT count(*) FROM station_provider_discoveries WHERE job_id=$1),
		 (SELECT count(*) FROM station_call_openings WHERE job_id=$1),
		 (SELECT count(*) FROM llm_call_evidence WHERE job_id=$1)
	`, job.ID).Scan(&gaps, &discoveries, &calls, &evidence); err != nil {
		t.Fatal(err)
	}
	if gaps != 0 || discoveries != 0 || calls != 0 || evidence != 0 {
		t.Fatalf("deterministic memory closure created provider records: gaps/discoveries/calls/evidence=%d/%d/%d/%d",
			gaps, discoveries, calls, evidence)
	}
}

func TestPostgresScopedMemoryNeedFailsAfterPersistenceWithoutEmbeddingProvider(t *testing.T) {
	ctx, repository, _ := openRepositoryTestDatabase(t)
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: "memory-need-no-embedding", Scope: model.ChannelScopeUser, Mode: model.ChannelModeAssistant,
		Name: "Persisted memory need", WorkspaceRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	embedding := make([]float64, model.MemoryEmbeddingDimensions)
	embedding[0] = 1
	if _, err := repository.AddMemoryChunks(ctx, []queue.MemoryChunkWrite{{
		Input: model.MemoryInput{
			Scope:  model.MemoryScope{ProjectID: channel.ProjectID, ChannelID: channel.ID},
			Source: "manual", Kind: model.MemoryKindReference, Content: "Durable exact memory.",
			Tags: []string{"scope:user"}, Categories: []model.MemoryCategory{model.MemoryCategoryResearch},
		},
		Embedding: embedding,
	}}); err != nil {
		t.Fatal(err)
	}
	_, job, err := repository.EnqueueChannelTurn(ctx, channel.ID, "Use durable memory.")
	if err != nil {
		t.Fatal(err)
	}
	transports := llmprovider.NewLazyFromConfig(config.Config{
		InferenceContextTokens: llm.DefaultInferenceContextTokens,
	})
	runtime := &nativeRuntimeV3{ctx: ctx, svc: &Service{
		repo: repository, stationClient: transports.Stations, embeddings: transports.Embeddings,
	}}
	_, err = (runtimeConversationCandidateProvider{runtime: runtime}).MemoryCandidates(ctx, job)
	if err == nil || !strings.Contains(err.Error(), "EMBEDDING_PROVIDER is not configured") {
		t.Fatalf("persisted embedding need error=%v, want absent authority", err)
	}
	hasMemory, checkErr := repository.HasScopedMemory(ctx, model.MemoryScope{
		ProjectID: channel.ProjectID, ChannelID: channel.ID,
	})
	if checkErr != nil || !hasMemory {
		t.Fatalf("provider rejection changed persisted memory need: exists=%t error=%v", hasMemory, checkErr)
	}
}
