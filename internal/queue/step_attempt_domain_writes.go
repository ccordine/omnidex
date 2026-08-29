package queue

import (
	"context"

	"github.com/gryph/omnidex/internal/model"
)

func (r *Repository) AddMemoryChunkByStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	scope model.MemoryScope,
	source, kind, content string,
	tags []string,
	embedding []float64,
) (model.MemoryChunk, error) {
	return underActiveStepAttemptFence(ctx, r, authority, "add durable memory", func() (model.MemoryChunk, error) {
		return r.AddMemoryChunk(ctx, scope, source, kind, content, tags, embedding)
	})
}
