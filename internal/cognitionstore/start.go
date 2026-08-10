package cognitionstore

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/queue"
)

// StartEpisode persists the episode with the exact executable fact authority
// owned by this store. Callers cannot omit or replace that authority.
func (store *Store) StartEpisode(
	ctx context.Context,
	command queue.CognitionEpisodeStart,
) (queue.CognitionEpisode, error) {
	if store == nil || store.repository == nil {
		return queue.CognitionEpisode{}, fmt.Errorf("cognition episode store is uninitialized")
	}
	return store.repository.StartCognitionEpisode(ctx, command, store.facts)
}
