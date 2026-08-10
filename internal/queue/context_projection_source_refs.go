package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func insertContextProjectionSelectedSourcesTx(
	ctx context.Context,
	tx pgx.Tx,
	projectionID string,
	selectionPosition int,
	refs []taskstate.Ref,
) error {
	for sourcePosition, ref := range refs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO context_projection_selected_source_refs (
				projection_id,selection_position,source_position,
				ref_uri,ref_version,ref_sha256,ref_relation
			) VALUES ($1,$2,$3,$4,$5,$6,$7)
		`, projectionID, selectionPosition, sourcePosition,
			ref.URI, ref.Version, ref.Hash, ref.Relation); err != nil {
			return fmt.Errorf("insert projection %q selection %d source %d: %w",
				projectionID, selectionPosition, sourcePosition, err)
		}
	}
	return nil
}

func sealContextProjectionSelectedSourcesTx(
	ctx context.Context,
	tx pgx.Tx,
	projectionID string,
	selectionPosition int,
) error {
	tag, err := tx.Exec(ctx, `
		UPDATE context_projection_selected_refs SET source_refs_sealed_at=NOW()
		WHERE projection_id=$1 AND position=$2 AND source_refs_sealed_at IS NULL
	`, projectionID, selectionPosition)
	if err != nil {
		return fmt.Errorf("seal projection %q selection %d sources: %w", projectionID, selectionPosition, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: projection selection source seal affected %d rows",
			ErrInvalidContextProjection, tag.RowsAffected())
	}
	return nil
}

func loadContextProjectionSelectedSourcesTx(
	ctx context.Context,
	tx pgx.Tx,
	projectionID string,
) (map[int][]taskstate.Ref, error) {
	rows, err := tx.Query(ctx, `
		SELECT selection_position,source_position,ref_uri,ref_version,ref_sha256,ref_relation
		FROM context_projection_selected_source_refs
		WHERE projection_id=$1 ORDER BY selection_position,source_position
	`, projectionID)
	if err != nil {
		return nil, fmt.Errorf("read projection %q selected sources: %w", projectionID, err)
	}
	defer rows.Close()
	result := make(map[int][]taskstate.Ref)
	for rows.Next() {
		var selectionPosition, sourcePosition int
		var uri, version, hash, relation string
		if err := rows.Scan(&selectionPosition, &sourcePosition, &uri, &version, &hash, &relation); err != nil {
			return nil, err
		}
		if selectionPosition < 0 || sourcePosition != len(result[selectionPosition]) {
			return nil, fmt.Errorf("%w: selected source reference order is invalid", ErrInvalidContextProjection)
		}
		ref := projectionRef(uri, version, hash, relation)
		if err := taskstate.ValidateRef(ref); err != nil {
			return nil, fmt.Errorf("%w: selected source reference: %v", ErrInvalidContextProjection, err)
		}
		result[selectionPosition] = append(result[selectionPosition], ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
