package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5"
)

func loadContextProjectionSelectedTx(
	ctx context.Context,
	tx pgx.Tx,
	record ContextProjectionRecord,
	expected int,
) ([]contextbuilder.Selection, error) {
	sources, err := loadContextProjectionSelectedSourcesTx(ctx, tx, record.Projection.ID)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT projection_id, working_set_id, job_id, generation, position, item_id,
		       ref_uri, ref_version, ref_sha256, ref_relation, role, authority,
		       source_freshness, content_sha256, rendered_bytes, source_ref_count,
		       source_refs_sealed_at IS NOT NULL
		FROM context_projection_selected_refs
		WHERE projection_id=$1 ORDER BY position ASC LIMIT $2
	`, record.Projection.ID, maxContextProjectionSelected+1)
	if err != nil {
		return nil, fmt.Errorf("read context projection %q selected refs: %w", record.Projection.ID, err)
	}
	defer rows.Close()
	owner := projectionRefOwner(record)
	selected := make([]contextbuilder.Selection, 0, expected)
	for rows.Next() {
		var projectionID string
		var workingSetID workingset.SetID
		var jobID, generation int64
		var position int
		var sourceCount int
		var sourcesSealed bool
		var item contextbuilder.Selection
		var uri, version, hash, relation string
		if err := rows.Scan(
			&projectionID, &workingSetID, &jobID, &generation, &position, &item.ItemID,
			&uri, &version, &hash, &relation, &item.Role, &item.Authority,
			&item.SourceFreshness, &item.ContentSHA256, &item.RenderedBytes, &sourceCount, &sourcesSealed,
		); err != nil {
			return nil, fmt.Errorf("scan context projection %q selected ref: %w", record.Projection.ID, err)
		}
		if position != len(selected) || !contextProjectionRefMatches(owner, projectionID, workingSetID, jobID, generation) {
			return nil, fmt.Errorf("%w: selected reference authority or order is invalid", ErrInvalidContextProjection)
		}
		item.Ref = projectionRef(uri, version, hash, relation)
		item.SourceRefs = sources[position]
		if item.SourceRefs == nil {
			item.SourceRefs = []taskstate.Ref{}
		}
		if !sourcesSealed || sourceCount != len(item.SourceRefs) {
			return nil, fmt.Errorf("%w: selected source reference seal is invalid", ErrInvalidContextProjection)
		}
		selected = append(selected, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate context projection %q selected refs: %w", record.Projection.ID, err)
	}
	if len(selected) != expected {
		return nil, fmt.Errorf("%w: selected reference count is %d, expected %d", ErrInvalidContextProjection, len(selected), expected)
	}
	return selected, nil
}

func loadContextProjectionOmittedTx(
	ctx context.Context,
	tx pgx.Tx,
	record ContextProjectionRecord,
	expected int,
) ([]contextbuilder.Omission, error) {
	rows, err := tx.Query(ctx, `
		SELECT projection_id, working_set_id, job_id, generation, position, item_id,
		       ref_uri, ref_version, ref_sha256, ref_relation, role, selector_id,
		       omission_reason, authority, source_freshness
		FROM context_projection_omitted_refs
		WHERE projection_id=$1 ORDER BY position ASC LIMIT $2
	`, record.Projection.ID, maxContextProjectionRecords+1)
	if err != nil {
		return nil, fmt.Errorf("read context projection %q omitted refs: %w", record.Projection.ID, err)
	}
	defer rows.Close()
	owner := projectionRefOwner(record)
	omitted := make([]contextbuilder.Omission, 0, expected)
	for rows.Next() {
		var projectionID string
		var workingSetID workingset.SetID
		var jobID, generation int64
		var position int
		var item contextbuilder.Omission
		var uri, version, hash, relation string
		var selector, authority *string
		if err := rows.Scan(
			&projectionID, &workingSetID, &jobID, &generation, &position, &item.ItemID,
			&uri, &version, &hash, &relation, &item.Role, &selector,
			&item.Reason, &authority, &item.SourceFreshness,
		); err != nil {
			return nil, fmt.Errorf("scan context projection %q omitted ref: %w", record.Projection.ID, err)
		}
		if position != len(omitted) || !contextProjectionRefMatches(owner, projectionID, workingSetID, jobID, generation) {
			return nil, fmt.Errorf("%w: omitted reference authority or order is invalid", ErrInvalidContextProjection)
		}
		item.Ref = projectionRef(uri, version, hash, relation)
		item.SelectorID = exactOptionalText(selector)
		item.Authority = taskstate.Authority(exactOptionalText(authority))
		omitted = append(omitted, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate context projection %q omitted refs: %w", record.Projection.ID, err)
	}
	if len(omitted) != expected {
		return nil, fmt.Errorf("%w: omitted reference count is %d, expected %d", ErrInvalidContextProjection, len(omitted), expected)
	}
	return omitted, nil
}
