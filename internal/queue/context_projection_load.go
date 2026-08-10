package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) GetContextProjection(
	ctx context.Context,
	projectionID string,
) (ContextProjectionRecord, error) {
	if !validContextProjectionID(projectionID) {
		return ContextProjectionRecord{}, fmt.Errorf("%w: projection ID is malformed", ErrInvalidContextProjection)
	}
	if err := validateContextProjectionRepository(r, ctx); err != nil {
		return ContextProjectionRecord{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return ContextProjectionRecord{}, fmt.Errorf("begin context projection read: %w", err)
	}
	defer tx.Rollback(ctx)
	record, err := loadContextProjectionTx(ctx, tx, projectionID)
	if err != nil {
		return ContextProjectionRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ContextProjectionRecord{}, fmt.Errorf("commit context projection read: %w", err)
	}
	return record, nil
}

func loadContextProjectionTx(
	ctx context.Context,
	tx pgx.Tx,
	projectionID string,
) (ContextProjectionRecord, error) {
	var record ContextProjectionRecord
	var projection contextbuilder.Projection
	var workingSetVersion int64
	var selectedCount, omittedCount int
	err := tx.QueryRow(ctx, `
		SELECT record_id, projection_id, schema_name, job_id, generation, step_id, step_attempt, worker_id,
		       work_id, work_kind, usage_mode, spec_name, spec_version, spec_sha256, renderer_version,
		       scope_ref_uri, scope_ref_version, scope_ref_sha256, scope_ref_relation,
		       working_set_id, working_set_version, selected_count, omitted_count,
		       rendered_context, rendered_sha256, rendered_bytes,
		       estimated_tokens, token_estimator, created_at
		FROM context_projections WHERE projection_id=$1
	`, projectionID).Scan(
		&record.RecordID, &projection.ID, &projection.Schema,
		&record.Authority.JobID, &record.Authority.Generation, &record.Authority.StepID,
		&record.Authority.Attempt, &record.Authority.WorkerID,
		&projection.WorkID, &record.Authority.WorkKind, &record.Authority.Mode, &projection.SpecName,
		&projection.SpecVersion, &projection.SpecSHA256, &projection.RendererVersion,
		&projection.ScopeRef.URI, &projection.ScopeRef.Version, &projection.ScopeRef.Hash,
		&projection.ScopeRef.Relation, &projection.WorkingSetID, &workingSetVersion,
		&selectedCount, &omittedCount, &projection.Rendered, &projection.RenderedSHA256,
		&projection.RenderedBytes, &projection.EstimatedTokens, &projection.TokenEstimator,
		&record.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ContextProjectionRecord{}, fmt.Errorf("%w: %s", ErrContextProjectionNotFound, projectionID)
	}
	if err != nil {
		return ContextProjectionRecord{}, fmt.Errorf("read context projection %q: %w", projectionID, err)
	}
	if workingSetVersion < 0 || selectedCount < 1 || selectedCount > maxContextProjectionSelected ||
		omittedCount < 0 || selectedCount+omittedCount > maxContextProjectionRecords {
		return ContextProjectionRecord{}, fmt.Errorf("%w: projection %q has invalid durable counts", ErrInvalidContextProjection, projectionID)
	}
	projection.WorkingSetVersion = uint64(workingSetVersion)
	record.Projection = projection
	projection.Selected, err = loadContextProjectionSelectedTx(ctx, tx, record, selectedCount)
	if err != nil {
		return ContextProjectionRecord{}, err
	}
	projection.Omitted, err = loadContextProjectionOmittedTx(ctx, tx, record, omittedCount)
	if err != nil {
		return ContextProjectionRecord{}, err
	}
	record.Projection = projection
	if record.RecordID <= 0 || record.CreatedAt.IsZero() {
		return ContextProjectionRecord{}, fmt.Errorf("%w: projection %q has invalid record metadata", ErrInvalidContextProjection, projectionID)
	}
	if err := validateContextProjectionStoreAuthority(record.Authority, projection); err != nil {
		return ContextProjectionRecord{}, err
	}
	return record, nil
}

func validateContextProjectionStoreAuthority(
	authority ContextProjectionAuthority,
	projection contextbuilder.Projection,
) error {
	if err := validateStepAttemptAuthority(authority.StepAttemptAuthority); err != nil {
		return fmt.Errorf("%w: durable owner is invalid", ErrInvalidContextProjection)
	}
	if err := validateContextProjectionExact(authority.WorkKind, "work kind", 256); err != nil {
		return err
	}
	if !validContextProjectionMode(authority.Mode) {
		return fmt.Errorf("%w: durable context projection mode %q is invalid", ErrInvalidContextProjection, authority.Mode)
	}
	if len(projection.WorkID) != 64 || !llmEvidenceLowerHex(projection.WorkID) {
		return fmt.Errorf("%w: durable work ID is invalid", ErrInvalidContextProjection)
	}
	if err := projection.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidContextProjection, err)
	}
	return nil
}

type contextProjectionRefOwner struct {
	projectionID string
	workingSetID workingset.SetID
	jobID        int64
	generation   int64
}

func projectionRefOwner(record ContextProjectionRecord) contextProjectionRefOwner {
	return contextProjectionRefOwner{
		projectionID: record.Projection.ID, workingSetID: record.Projection.WorkingSetID,
		jobID: record.Authority.JobID, generation: record.Authority.Generation,
	}
}

func contextProjectionRefMatches(
	owner contextProjectionRefOwner,
	projectionID string,
	workingSetID workingset.SetID,
	jobID, generation int64,
) bool {
	return projectionID == owner.projectionID && workingSetID == owner.workingSetID &&
		jobID == owner.jobID && generation == owner.generation
}

func projectionRef(
	uri, version, hash string,
	relation string,
) taskstate.Ref {
	return taskstate.Ref{
		URI: uri, Version: version, Hash: hash, Relation: taskstate.RefRelation(relation),
	}
}
