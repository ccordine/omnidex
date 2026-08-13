package worker

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

type objectiveMemoryCountingEmbedding struct{ calls atomic.Int64 }

func (client *objectiveMemoryCountingEmbedding) Embedding(context.Context, string) ([]float64, error) {
	client.calls.Add(1)
	return make([]float64, model.MemoryEmbeddingDimensions), nil
}

func TestPostgresEmptyScopedMemoryMakesNoEmbeddingCall(t *testing.T) {
	ctx, repository, _ := openRepositoryTestDatabase(t)
	channel, err := repository.CreateChannel(ctx, model.Channel{
		ID: "empty-memory-no-embedding", Scope: model.ChannelScopeUser,
		Name: "Empty memory", WorkspaceRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, job, err := repository.EnqueueChannelTurn(ctx, channel.ID, "Answer without durable memory.")
	if err != nil {
		t.Fatal(err)
	}
	embeddings := &objectiveMemoryCountingEmbedding{}
	runtime := &nativeRuntimeV3{
		ctx: ctx, svc: &Service{repo: repository, embeddings: embeddings},
	}
	set, err := (runtimeConversationCandidateProvider{runtime: runtime}).MemoryCandidates(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Candidates) != 0 || set.Replan != nil {
		t.Fatalf("empty scope returned objective context: %+v", set)
	}
	if calls := embeddings.calls.Load(); calls != 0 {
		t.Fatalf("empty exact memory scope made %d embedding calls", calls)
	}
}
