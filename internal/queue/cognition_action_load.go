package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/jackc/pgx/v5"
)

func loadCognitionActionTx(
	ctx context.Context,
	tx pgx.Tx,
	actionID cognition.ActionID,
	lock bool,
) (CognitionActionRecord, bool, error) {
	query := `
		SELECT actions.episode_id,actions.job_id,actions.generation,actions.step_id,
		       actions.origin_attempt,actions.origin_worker_id,actions.obligation_node_id,
		       actions.policy_call_id,actions.reconciliation_id,actions.reconciliation_sha256,
		       actions.expected_revision,actions.expected_revision_sha256,
		       actions.snapshot_sha256,actions.projection_id,projections.rendered_sha256,
		       projections.working_set_id,projections.working_set_version,projections.renderer_version,
		       actions.action_schema_id,
		       actions.action_schema_version,actions.action_schema_sha256,actions.decision_json,
		       actions.registered_action_json,actions.status,actions.failure_json,
		       actions.result_revision,actions.result_revision_sha256,actions.created_at,
		       actions.dispatched_at,actions.resolved_at,episodes.action_catalog_json
		FROM cognition_actions actions
		JOIN cognition_episodes episodes ON episodes.episode_id=actions.episode_id
		JOIN context_projections projections ON projections.projection_id=actions.projection_id
		WHERE actions.action_id=$1`
	if lock {
		query += ` FOR UPDATE OF actions`
	}
	var record CognitionActionRecord
	var decisionJSON, actionJSON, catalogJSON []byte
	var failureJSON []byte
	var expectedRevision int64
	var workingSetVersion int64
	var resultRevision *int64
	var resultSHA *string
	err := tx.QueryRow(ctx, query, actionID).Scan(
		&record.EpisodeID, &record.Origin.JobID, &record.Origin.Generation, &record.Origin.StepID,
		&record.Origin.Attempt, &record.Origin.WorkerID, &record.ObligationID, &record.PolicyCallID,
		&record.ReconciliationID, &record.ReconciliationSHA256, &expectedRevision,
		&record.ExpectedRevision.SHA256, &record.SnapshotSHA256,
		&record.ContextProjection.ID, &record.ContextProjection.SHA256,
		&record.ContextProjection.WorkingSetID, &workingSetVersion,
		&record.ContextProjection.RendererVersion, &record.Schema.ID, &record.Schema.Version, &record.Schema.SHA256,
		&decisionJSON, &actionJSON, &record.Status, &failureJSON, &resultRevision, &resultSHA,
		&record.CreatedAt, &record.DispatchedAt, &record.ResolvedAt, &catalogJSON,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return CognitionActionRecord{}, false, nil
	}
	if err != nil {
		return CognitionActionRecord{}, false, fmt.Errorf("load cognition action %q: %w", actionID, err)
	}
	record.ExpectedRevision = cognition.WorldRevision{
		EpisodeID: record.EpisodeID, Number: uint64(expectedRevision), SHA256: record.ExpectedRevision.SHA256,
	}
	if workingSetVersion < 0 {
		return CognitionActionRecord{}, false, fmt.Errorf("%w: cognition action working-set version is invalid", ErrCognitionConflict)
	}
	record.ContextProjection.WorkingSetVersion = uint64(workingSetVersion)
	if err := record.ContextProjection.Validate(); err != nil {
		return CognitionActionRecord{}, false, fmt.Errorf("%w: cognition action projection: %v", ErrCognitionConflict, err)
	}
	if err := json.Unmarshal(decisionJSON, &record.Decision); err != nil {
		return CognitionActionRecord{}, false, fmt.Errorf("decode cognition decision: %w", err)
	}
	if err := json.Unmarshal(actionJSON, &record.Action); err != nil {
		return CognitionActionRecord{}, false, fmt.Errorf("decode cognition action: %w", err)
	}
	var catalog cognition.ActionCatalog
	if err := json.Unmarshal(catalogJSON, &catalog); err != nil {
		return CognitionActionRecord{}, false, fmt.Errorf("decode cognition action catalog: %w", err)
	}
	schema, exists := catalog.Schema(record.Action.Request.Kind)
	if !exists || schema.Ref() != record.Schema {
		return CognitionActionRecord{}, false, fmt.Errorf("%w: cognition action schema is absent", ErrCognitionConflict)
	}
	if err := record.Action.Validate(schema); err != nil {
		return CognitionActionRecord{}, false, err
	}
	if err := record.Decision.Validate(schema); err != nil {
		return CognitionActionRecord{}, false, err
	}
	if len(failureJSON) != 0 {
		var failure cognition.ActionFailure
		if err := json.Unmarshal(failureJSON, &failure); err != nil {
			return CognitionActionRecord{}, false, fmt.Errorf("decode cognition failure: %w", err)
		}
		if err := failure.Validate(record.Action, record.ExpectedRevision); err != nil {
			return CognitionActionRecord{}, false, err
		}
		record.Failure = &failure
	}
	if resultRevision != nil && resultSHA != nil {
		revision := cognition.WorldRevision{
			EpisodeID: record.EpisodeID, Number: uint64(*resultRevision), SHA256: *resultSHA,
		}
		if err := revision.Validate(); err != nil {
			return CognitionActionRecord{}, false, err
		}
		record.ResultRevision = &revision
	}
	return record, true, nil
}

func (r *Repository) CognitionAction(
	ctx context.Context,
	actionID cognition.ActionID,
) (CognitionActionRecord, error) {
	if ctx == nil || r == nil || r.pool == nil || actionID == "" {
		return CognitionActionRecord{}, fmt.Errorf("cognition action read requires PostgreSQL, context, and action ID")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return CognitionActionRecord{}, err
	}
	defer tx.Rollback(ctx)
	record, found, err := loadCognitionActionTx(ctx, tx, actionID, false)
	if err != nil {
		return CognitionActionRecord{}, err
	}
	if !found {
		return CognitionActionRecord{}, fmt.Errorf("%w: %s", ErrCognitionActionNotFound, actionID)
	}
	if err := tx.Commit(ctx); err != nil {
		return CognitionActionRecord{}, err
	}
	return record, nil
}
