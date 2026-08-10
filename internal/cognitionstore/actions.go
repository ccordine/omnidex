package cognitionstore

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func (store *Store) Unresolved(
	ctx context.Context,
	binding cognitionruntime.Binding,
) (*cognitionruntime.ActionRecord, error) {
	if store == nil || store.repository == nil {
		return nil, fmt.Errorf("cognition action store is uninitialized")
	}
	if err := store.requireFactAuthority(ctx, binding.Episode); err != nil {
		return nil, err
	}
	authority, err := queueAuthority(binding)
	if err != nil {
		return nil, err
	}
	record, err := store.repository.UnresolvedCognitionAction(ctx, authority, binding.Episode.ID)
	if err != nil || record == nil {
		return nil, err
	}
	mapped, err := store.actionRecord(ctx, binding, *record)
	if err != nil {
		return nil, err
	}
	return &mapped, nil
}

func (store *Store) PrepareAction(
	ctx context.Context,
	command cognitionruntime.PrepareActionCommand,
) (cognitionruntime.ActionRecord, error) {
	if store == nil || store.repository == nil {
		return cognitionruntime.ActionRecord{}, fmt.Errorf("cognition action store is uninitialized")
	}
	if err := store.requireFactAuthority(ctx, command.Binding.Episode); err != nil {
		return cognitionruntime.ActionRecord{}, err
	}
	record, err := store.repository.PrepareCognitionAction(ctx, command)
	if err != nil {
		return cognitionruntime.ActionRecord{}, err
	}
	return store.actionRecord(ctx, command.Binding, record)
}

func (store *Store) MarkDispatched(
	ctx context.Context,
	command cognitionruntime.ActionMutation,
) (cognitionruntime.ActionRecord, error) {
	if store == nil || store.repository == nil {
		return cognitionruntime.ActionRecord{}, fmt.Errorf("cognition action store is uninitialized")
	}
	authority, err := queueAuthority(command.Binding)
	if err != nil {
		return cognitionruntime.ActionRecord{}, err
	}
	record, err := store.repository.DispatchCognitionAction(ctx, authority, command.ActionID)
	if err != nil {
		return cognitionruntime.ActionRecord{}, err
	}
	return store.actionRecord(ctx, command.Binding, record)
}

func (store *Store) RecordFailure(
	ctx context.Context,
	command cognitionruntime.FailureMutation,
) (cognitionruntime.ActionRecord, error) {
	if store == nil || store.repository == nil {
		return cognitionruntime.ActionRecord{}, fmt.Errorf("cognition action store is uninitialized")
	}
	authority, err := queueAuthority(command.Binding)
	if err != nil {
		return cognitionruntime.ActionRecord{}, err
	}
	record, err := store.repository.IngestCognitionFailure(
		ctx, authority, command.ActionID, command.Failure,
	)
	if err != nil {
		return cognitionruntime.ActionRecord{}, err
	}
	return store.actionRecord(ctx, command.Binding, record)
}

func (store *Store) RecordTransition(
	ctx context.Context,
	command cognitionruntime.TransitionMutation,
) (cognitionruntime.ActionRecord, error) {
	if store == nil || store.repository == nil {
		return cognitionruntime.ActionRecord{}, fmt.Errorf("cognition action store is uninitialized")
	}
	if err := store.requireFactAuthority(ctx, command.Binding.Episode); err != nil {
		return cognitionruntime.ActionRecord{}, err
	}
	authority, err := queueAuthority(command.Binding)
	if err != nil {
		return cognitionruntime.ActionRecord{}, err
	}
	record, err := store.repository.IngestCognitionTransition(
		ctx, authority, command.ActionID, command.Transition, store.facts,
	)
	if err != nil {
		return cognitionruntime.ActionRecord{}, err
	}
	return store.actionRecord(ctx, command.Binding, record)
}
