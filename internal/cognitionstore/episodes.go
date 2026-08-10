package cognitionstore

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func (store *Store) TerminalProgress(
	ctx context.Context,
	binding cognitionruntime.Binding,
) (*cognitionruntime.EpisodeProgress, error) {
	if store == nil || store.repository == nil {
		return nil, fmt.Errorf("cognition episode store is uninitialized")
	}
	return store.repository.CognitionRuntimeTerminalProgress(ctx, binding)
}

func (store *Store) AdvanceSatisfied(
	ctx context.Context,
	command cognitionruntime.CompletionCommand,
) (cognitionruntime.EpisodeProgress, error) {
	if store == nil || store.repository == nil {
		return cognitionruntime.EpisodeProgress{}, fmt.Errorf("cognition episode store is uninitialized")
	}
	return store.repository.AdvanceCognitionRuntimeSatisfied(ctx, command)
}

func (store *Store) FailTerminal(
	ctx context.Context,
	command cognitionruntime.CompletionCommand,
) (cognitionruntime.EpisodeProgress, error) {
	if store == nil || store.repository == nil {
		return cognitionruntime.EpisodeProgress{}, fmt.Errorf("cognition episode store is uninitialized")
	}
	return store.repository.FailCognitionRuntimeTerminal(ctx, command)
}
