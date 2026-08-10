package cognitionstore

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/queue"
)

func (store *Store) LoadProjection(
	ctx context.Context,
	ref cognition.ContextProjectionRef,
) (contextbuilder.Projection, error) {
	if store == nil || store.repository == nil || ctx == nil {
		return contextbuilder.Projection{}, fmt.Errorf("cognition projection load requires an initialized store and context")
	}
	if err := ref.Validate(); err != nil {
		return contextbuilder.Projection{}, err
	}
	record, err := store.repository.GetContextProjection(ctx, string(ref.ID))
	if err != nil {
		return contextbuilder.Projection{}, err
	}
	projection := record.Projection
	persisted := cognition.ContextProjectionRef{
		ID: cognition.ContextProjectionID(projection.ID), SHA256: projection.RenderedSHA256,
		WorkingSetID:      cognition.WorkingSetID(projection.WorkingSetID),
		WorkingSetVersion: projection.WorkingSetVersion, RendererVersion: projection.RendererVersion,
	}
	if record.Authority.Mode != queue.ContextProjectionModeLive || persisted != ref {
		return contextbuilder.Projection{}, fmt.Errorf("cognition projection differs from the exact live reference")
	}
	return projection, nil
}
