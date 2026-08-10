package cognitionstore

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/queue"
)

func (store *Store) PrepareSnapshot(
	ctx context.Context,
	binding cognitionruntime.Binding,
) (cognitionruntime.PreparedSnapshot, error) {
	if store == nil || store.repository == nil {
		return cognitionruntime.PreparedSnapshot{}, fmt.Errorf("cognition snapshot store is uninitialized")
	}
	if err := store.requireFactAuthority(ctx, binding.Episode); err != nil {
		return cognitionruntime.PreparedSnapshot{}, err
	}
	authority, err := queueAuthority(binding)
	if err != nil {
		return cognitionruntime.PreparedSnapshot{}, err
	}
	record, err := store.repository.PrepareCognitionRuntimeSnapshot(ctx, queue.CognitionRuntimeSnapshotCommand{
		Authority: authority, EpisodeID: binding.Episode.ID,
	})
	if err != nil {
		return cognitionruntime.PreparedSnapshot{}, err
	}
	if err := record.Prepared.ValidateFor(binding); err != nil {
		return cognitionruntime.PreparedSnapshot{}, err
	}
	return record.Prepared, nil
}

func (store *Store) Reconcile(
	ctx context.Context,
	command cognitionruntime.ReconciliationCommand,
) (cognitionruntime.ReconciliationReceipt, error) {
	if store == nil || store.repository == nil {
		return cognitionruntime.ReconciliationReceipt{}, fmt.Errorf("cognition reconciler store is uninitialized")
	}
	if err := store.requireFactAuthority(ctx, command.Binding.Episode); err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	return store.repository.ReconcileCognitionRuntimeDecision(ctx, command)
}
