package queue

import (
	"context"
	"encoding/json"

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

func (r *Repository) UpdateProjectSettingByStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	projectID int64,
	key string,
	value json.RawMessage,
) error {
	return underActiveStepAttemptWriteFence(ctx, r, authority, "update project setting", func() error {
		return r.UpdateProjectSetting(ctx, projectID, key, value)
	})
}

func (r *Repository) UpdateScrumCardByStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	projectID int64,
	cardID string,
	patch map[string]any,
) (DBScrumCard, error) {
	return underActiveStepAttemptFence(ctx, r, authority, "update Scrum card", func() (DBScrumCard, error) {
		return r.UpdateScrumCard(ctx, projectID, cardID, patch)
	})
}
