package queue

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

type ScrumCardPlacement struct {
	CardID     string
	Column     string
	BoardOrder int
}

// PlaceScrumCards persists a complete reorder as one PostgreSQL transaction.
func (r *Repository) PlaceScrumCards(ctx context.Context, projectID int64, placements []ScrumCardPlacement) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("PostgreSQL repository is required for Scrum card placement")
	}
	if ctx == nil || projectID <= 0 {
		return fmt.Errorf("context and project are required for Scrum card placement")
	}
	if len(placements) == 0 {
		return fmt.Errorf("at least one Scrum card placement is required")
	}
	byID := make(map[string]ScrumCardPlacement, len(placements))
	ids := make([]string, 0, len(placements))
	for _, placement := range placements {
		placement.CardID = strings.TrimSpace(placement.CardID)
		placement.Column = strings.TrimSpace(placement.Column)
		if placement.CardID == "" || placement.Column == "" || placement.BoardOrder < 0 {
			return fmt.Errorf("Scrum card placement requires card, column, and non-negative order")
		}
		if _, exists := byID[placement.CardID]; exists {
			return fmt.Errorf("duplicate Scrum card placement %q", placement.CardID)
		}
		byID[placement.CardID] = placement
		ids = append(ids, placement.CardID)
	}
	sort.Strings(ids)

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer rollbackTx(ctx, tx, "Scrum card placement")

	rows, err := tx.Query(ctx, `
		SELECT id
		FROM scrum_cards
		WHERE project_id = $1 AND id = ANY($2)
		ORDER BY id
		FOR UPDATE
	`, projectID, ids)
	if err != nil {
		return err
	}
	locked := make([]string, 0, len(ids))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		locked = append(locked, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(locked) != len(ids) {
		return fmt.Errorf("locked %d Scrum cards; expected %d", len(locked), len(ids))
	}

	for _, id := range ids {
		placement := byID[id]
		tag, err := tx.Exec(ctx, `
			UPDATE scrum_cards
			SET column_name = $3, board_order = $4, updated_at = NOW()
			WHERE project_id = $1 AND id = $2
		`, projectID, id, placement.Column, placement.BoardOrder)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("Scrum card placement for %q updated %d rows; expected 1", id, tag.RowsAffected())
		}
	}
	tag, err := tx.Exec(ctx, `
		UPDATE projects
		SET last_seen_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, projectID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("Scrum card placement project %d was not found", projectID)
	}
	return tx.Commit(ctx)
}
