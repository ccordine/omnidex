package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func insertContextProjectionReferencesTx(
	ctx context.Context,
	tx pgx.Tx,
	record ContextProjectionRecord,
) error {
	projection, authority := record.Projection, record.Authority
	for position, selected := range projection.Selected {
		if _, err := tx.Exec(ctx, `
			INSERT INTO context_projection_selected_refs (
				projection_id, working_set_id, job_id, generation, position, item_id,
				ref_uri, ref_version, ref_sha256, ref_relation, role, authority,
				source_freshness, content_sha256, rendered_bytes
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		`, projection.ID, projection.WorkingSetID, authority.JobID, authority.Generation,
			position, selected.ItemID, selected.Ref.URI, selected.Ref.Version,
			selected.Ref.Hash, selected.Ref.Relation, selected.Role, selected.Authority,
			selected.SourceFreshness, selected.ContentSHA256, selected.RenderedBytes,
		); err != nil {
			return fmt.Errorf("insert context projection %q selected reference %d: %w", projection.ID, position, err)
		}
	}
	for position, omitted := range projection.Omitted {
		if _, err := tx.Exec(ctx, `
			INSERT INTO context_projection_omitted_refs (
				projection_id, working_set_id, job_id, generation, position, item_id,
				ref_uri, ref_version, ref_sha256, ref_relation, role, selector_id,
				omission_reason, authority, source_freshness
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		`, projection.ID, projection.WorkingSetID, authority.JobID, authority.Generation,
			position, omitted.ItemID, omitted.Ref.URI, omitted.Ref.Version,
			omitted.Ref.Hash, omitted.Ref.Relation, omitted.Role,
			nullableTaskText(omitted.SelectorID), omitted.Reason,
			nullableTaskText(string(omitted.Authority)), omitted.SourceFreshness,
		); err != nil {
			return fmt.Errorf("insert context projection %q omitted reference %d: %w", projection.ID, position, err)
		}
	}
	return nil
}
