package queue

import (
	"context"
	"fmt"
)

func (r *Repository) SetScrumAutoWorkConfig(
	ctx context.Context,
	projectID int64,
	config ScrumAutoWorkConfig,
) (ScrumAutoWorkConfig, error) {
	if r == nil || r.pool == nil || ctx == nil || projectID <= 0 {
		return ScrumAutoWorkConfig{}, fmt.Errorf("PostgreSQL, context, and project are required for Scrum auto-work config")
	}
	tx, err := r.beginLockedProjectTx(ctx, projectID, "set Scrum auto-work config")
	if err != nil {
		return ScrumAutoWorkConfig{}, err
	}
	defer rollbackTx(ctx, tx, "set Scrum auto-work config")
	var settings []byte
	if err := tx.QueryRow(ctx, `SELECT settings FROM projects WHERE id=$1 FOR UPDATE`, projectID).Scan(&settings); err != nil {
		return ScrumAutoWorkConfig{}, err
	}
	encoded, err := encodeScrumAutoWorkSettings(settings, config)
	if err != nil {
		return ScrumAutoWorkConfig{}, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE projects SET settings=$2::jsonb,updated_at=clock_timestamp(),last_seen_at=clock_timestamp()
		WHERE id=$1
	`, projectID, string(encoded))
	if err != nil {
		return ScrumAutoWorkConfig{}, err
	}
	if tag.RowsAffected() != 1 {
		return ScrumAutoWorkConfig{}, ErrProjectNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return ScrumAutoWorkConfig{}, err
	}
	return ValidateScrumAutoWorkConfig(config)
}
