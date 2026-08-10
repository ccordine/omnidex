package cognitionstore

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func (store *Store) Cancel(
	ctx context.Context,
	command cognitionruntime.CancellationCommand,
) (cognitionruntime.CancellationSeal, error) {
	if store == nil || store.repository == nil {
		return cognitionruntime.CancellationSeal{}, fmt.Errorf("cognition cancellation store is uninitialized")
	}
	return store.repository.CancelCognitionEpisode(ctx, command)
}
