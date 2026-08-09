package queue

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) StoreContextProjection(
	ctx context.Context,
	authority ContextProjectionAuthority,
	projection contextbuilder.Projection,
) (ContextProjectionRecord, error) {
	if err := validateContextProjectionStore(r, ctx, authority, projection); err != nil {
		return ContextProjectionRecord{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ContextProjectionRecord{}, fmt.Errorf("begin context projection store: %w", err)
	}
	defer tx.Rollback(ctx)
	if existing, err := loadContextProjectionTx(ctx, tx, projection.ID); err == nil {
		return exactContextProjectionReplay(existing, authority, projection)
	} else if !errors.Is(err, ErrContextProjectionNotFound) {
		return ContextProjectionRecord{}, err
	}
	if err := validateCurrentContextProjectionAuthorityTx(ctx, tx, authority, projection); err != nil {
		return ContextProjectionRecord{}, err
	}
	record, inserted, err := insertContextProjectionTx(ctx, tx, authority, projection)
	if err != nil {
		return ContextProjectionRecord{}, err
	}
	if !inserted {
		existing, err := loadContextProjectionTx(ctx, tx, projection.ID)
		if err != nil {
			return ContextProjectionRecord{}, err
		}
		return exactContextProjectionReplay(existing, authority, projection)
	}
	if err := insertContextProjectionReferencesTx(ctx, tx, record); err != nil {
		return ContextProjectionRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ContextProjectionRecord{}, fmt.Errorf("commit context projection %q: %w", projection.ID, err)
	}
	return record, nil
}

func validateCurrentContextProjectionAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	authority ContextProjectionAuthority,
	projection contextbuilder.Projection,
) error {
	header, err := loadTaskLedgerHeaderTx(ctx, tx, authority.JobID, true)
	if err != nil {
		return err
	}
	if err := requireCurrentWorkingSetAuthority(header, authority.Generation); err != nil {
		return err
	}
	if header.JobStatus != model.JobStatusRunning {
		return fmt.Errorf("%w: job %d status is %q, expected running", ErrInvalidContextProjection, authority.JobID, header.JobStatus)
	}
	var stepGeneration int64
	var stepStatus string
	var supersededAt *int64
	if err := tx.QueryRow(ctx, `
		SELECT generation, status, superseded_at_generation
		FROM job_steps WHERE id=$1 AND job_id=$2 FOR UPDATE
	`, authority.StepID, authority.JobID).Scan(&stepGeneration, &stepStatus, &supersededAt); err != nil {
		return fmt.Errorf("lock context projection step %d: %w", authority.StepID, err)
	}
	if stepGeneration != authority.Generation || supersededAt != nil || stepStatus != model.StepStatusRunning {
		return fmt.Errorf(
			"%w: step %d is not running in current generation %d",
			ErrInvalidContextProjection, authority.StepID, authority.Generation,
		)
	}
	var setVersion int64
	var setStatus string
	if err := tx.QueryRow(ctx, `
		SELECT version, status FROM working_sets
		WHERE id=$1 AND job_id=$2 AND generation=$3 FOR SHARE
	`, projection.WorkingSetID, authority.JobID, authority.Generation).Scan(&setVersion, &setStatus); err != nil {
		return fmt.Errorf("lock projection working set %q: %w", projection.WorkingSetID, err)
	}
	if setVersion < 0 || uint64(setVersion) != projection.WorkingSetVersion ||
		setStatus != string(workingset.StatusActive) {
		return fmt.Errorf(
			"%w: working set %q version or status changed before projection",
			ErrInvalidContextProjection, projection.WorkingSetID,
		)
	}
	return nil
}

func insertContextProjectionTx(
	ctx context.Context,
	tx pgx.Tx,
	authority ContextProjectionAuthority,
	projection contextbuilder.Projection,
) (ContextProjectionRecord, bool, error) {
	record := ContextProjectionRecord{Authority: authority, Projection: projection}
	err := tx.QueryRow(ctx, `
		INSERT INTO context_projections (
			projection_id, schema_name, job_id, generation, step_id, work_id, work_kind, usage_mode,
			spec_name, spec_version, spec_sha256, renderer_version,
			scope_ref_uri, scope_ref_version, scope_ref_sha256, scope_ref_relation,
			working_set_id, working_set_version, selected_count, omitted_count,
			rendered_context, rendered_sha256, rendered_bytes,
			estimated_tokens, token_estimator
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25
		)
		ON CONFLICT (projection_id) DO NOTHING
		RETURNING record_id, created_at
	`, projection.ID, projection.Schema, authority.JobID, authority.Generation, authority.StepID,
		projection.WorkID, authority.WorkKind, authority.Mode, projection.SpecName, projection.SpecVersion,
		projection.SpecSHA256, projection.RendererVersion, projection.ScopeRef.URI,
		projection.ScopeRef.Version, projection.ScopeRef.Hash, projection.ScopeRef.Relation,
		projection.WorkingSetID, int64(projection.WorkingSetVersion), len(projection.Selected),
		len(projection.Omitted), projection.Rendered, projection.RenderedSHA256,
		projection.RenderedBytes, projection.EstimatedTokens, projection.TokenEstimator,
	).Scan(&record.RecordID, &record.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ContextProjectionRecord{}, false, nil
	}
	if err != nil {
		return ContextProjectionRecord{}, false, fmt.Errorf("insert context projection %q: %w", projection.ID, err)
	}
	return record, true, nil
}

func exactContextProjectionReplay(
	existing ContextProjectionRecord,
	authority ContextProjectionAuthority,
	projection contextbuilder.Projection,
) (ContextProjectionRecord, error) {
	if existing.Authority != authority || !reflect.DeepEqual(existing.Projection, projection) {
		return ContextProjectionRecord{}, fmt.Errorf(
			"%w: projection ID %q is already bound to different authority or content",
			ErrContextProjectionConflict, projection.ID,
		)
	}
	return existing, nil
}
