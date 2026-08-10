package cognitionstore

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/queue"
)

func (store *Store) actionRecord(
	ctx context.Context,
	binding cognitionruntime.Binding,
	record queue.CognitionActionRecord,
) (cognitionruntime.ActionRecord, error) {
	if store == nil || store.repository == nil {
		return cognitionruntime.ActionRecord{}, fmt.Errorf("cognition action store is uninitialized")
	}
	authority, err := queueAuthority(binding)
	if err != nil {
		return cognitionruntime.ActionRecord{}, err
	}
	action, err := record.ActionFor(authority)
	if err != nil {
		return cognitionruntime.ActionRecord{}, err
	}
	episode, err := store.repository.CognitionEpisode(ctx, record.EpisodeID)
	if err != nil {
		return cognitionruntime.ActionRecord{}, err
	}
	schema, exists := episode.ActionCatalog.Schema(action.Request.Kind)
	if !exists || schema.Ref() != record.Schema {
		return cognitionruntime.ActionRecord{}, fmt.Errorf("persisted cognition action schema is unavailable")
	}
	status, err := runtimeActionStatus(record.Status)
	if err != nil {
		return cognitionruntime.ActionRecord{}, err
	}
	mapped := cognitionruntime.ActionRecord{
		Episode: binding.Episode, ExpectedRevision: record.ExpectedRevision,
		SnapshotSHA256:    record.SnapshotSHA256,
		ContextProjection: record.ContextProjection, Schema: schema,
		Decision: record.Decision.Clone(), Action: action, Status: status,
	}
	if record.Failure != nil {
		failure := record.Failure.Clone()
		mapped.Failure = &failure
	}
	if record.ResultRevision != nil {
		revision := *record.ResultRevision
		mapped.ResultRevision = &revision
	}
	if err := mapped.ValidateFor(binding); err != nil {
		return cognitionruntime.ActionRecord{}, err
	}
	return mapped, nil
}

func runtimeActionStatus(status queue.CognitionActionStatus) (cognitionruntime.ActionStatus, error) {
	switch status {
	case queue.CognitionActionPrepared:
		return cognitionruntime.ActionPrepared, nil
	case queue.CognitionActionDispatched:
		return cognitionruntime.ActionDispatched, nil
	case queue.CognitionActionSucceeded:
		return cognitionruntime.ActionSucceeded, nil
	case queue.CognitionActionFailed:
		return cognitionruntime.ActionFailed, nil
	default:
		return "", fmt.Errorf("unregistered queue cognition action status %q", status)
	}
}
