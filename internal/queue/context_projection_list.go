package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) ListContextProjectionSummaries(
	ctx context.Context,
	jobID, generation, afterRecordID int64,
	limit int,
) ([]ContextProjectionSummary, error) {
	if jobID <= 0 || generation <= 0 || afterRecordID < 0 ||
		limit < 1 || limit > maxContextProjectionPageSize {
		return nil, fmt.Errorf(
			"%w: list requires positive job/generation, nonnegative cursor, and limit 1..%d",
			ErrInvalidContextProjection, maxContextProjectionPageSize,
		)
	}
	if err := validateContextProjectionRepository(r, ctx); err != nil {
		return nil, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("begin context projection page: %w", err)
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM job_generations WHERE job_id=$1 AND generation=$2
		)
	`, jobID, generation).Scan(&exists); err != nil {
		return nil, fmt.Errorf("validate context projection generation: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: job %d generation %d", ErrContextProjectionNotFound, jobID, generation)
	}
	rows, err := tx.Query(ctx, `
		SELECT record_id, projection_id, step_id, work_id, work_kind, usage_mode,
		       spec_name, spec_version, spec_sha256, renderer_version,
		       working_set_id, working_set_version, selected_count, omitted_count,
		       rendered_sha256, rendered_bytes, estimated_tokens, token_estimator, created_at
		FROM context_projections
		WHERE job_id=$1 AND generation=$2 AND record_id>$3
		ORDER BY record_id ASC LIMIT $4
	`, jobID, generation, afterRecordID, limit)
	if err != nil {
		return nil, fmt.Errorf("list context projections for job %d: %w", jobID, err)
	}
	defer rows.Close()
	items := make([]ContextProjectionSummary, 0, limit)
	for rows.Next() {
		var item ContextProjectionSummary
		var workingSetVersion int64
		item.Authority.JobID, item.Authority.Generation = jobID, generation
		if err := rows.Scan(
			&item.RecordID, &item.ProjectionID, &item.Authority.StepID,
			&item.WorkID, &item.Authority.WorkKind, &item.Authority.Mode,
			&item.SpecName, &item.SpecVersion, &item.SpecSHA256, &item.RendererVersion,
			&item.WorkingSetID, &workingSetVersion, &item.SelectedCount, &item.OmittedCount,
			&item.RenderedSHA256, &item.RenderedBytes, &item.EstimatedTokens,
			&item.TokenEstimator, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan context projection summary: %w", err)
		}
		if workingSetVersion < 0 {
			return nil, fmt.Errorf("%w: summary has negative working-set version", ErrInvalidContextProjection)
		}
		item.WorkingSetVersion = uint64(workingSetVersion)
		if err := validateContextProjectionSummary(item, afterRecordID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate context projection page: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit context projection page: %w", err)
	}
	return items, nil
}

func validateContextProjectionSummary(item ContextProjectionSummary, afterRecordID int64) error {
	if item.RecordID <= afterRecordID || !validContextProjectionID(item.ProjectionID) ||
		item.Authority.StepID <= 0 || len(item.WorkID) != 64 || !llmEvidenceLowerHex(item.WorkID) ||
		!validContextProjectionMode(item.Authority.Mode) ||
		item.SelectedCount < 1 || item.SelectedCount > maxContextProjectionSelected ||
		item.OmittedCount < 0 || item.SelectedCount+item.OmittedCount > maxContextProjectionRecords ||
		item.RenderedBytes < 1 || item.RenderedBytes > 1024*1024 ||
		item.EstimatedTokens != (item.RenderedBytes+3)/4 ||
		len(item.SpecSHA256) != 64 || !llmEvidenceLowerHex(item.SpecSHA256) ||
		len(item.RenderedSHA256) != 64 || !llmEvidenceLowerHex(item.RenderedSHA256) ||
		item.CreatedAt.IsZero() {
		return fmt.Errorf("%w: projection summary contains invalid durable evidence", ErrInvalidContextProjection)
	}
	for field, value := range map[string]string{
		"work kind": item.Authority.WorkKind, "spec name": item.SpecName,
		"spec version": item.SpecVersion, "renderer version": item.RendererVersion,
		"working-set ID": item.WorkingSetID, "token estimator": item.TokenEstimator,
	} {
		if err := validateContextProjectionExact(value, field, 512); err != nil {
			return err
		}
	}
	return nil
}
