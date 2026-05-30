package worker

import (
	"context"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type dataSourceMemoryWriter struct {
	repo *queue.Repository
}

func (w *dataSourceMemoryWriter) AddMemory(ctx context.Context, source, kind, content string, tags []string) error {
	if w == nil || w.repo == nil {
		return nil
	}
	if strings.TrimSpace(kind) == "" {
		kind = model.MemoryKindReference
	}
	_, err := w.repo.AddMemoryChunk(ctx, source, kind, content, tags, nil)
	return err
}
