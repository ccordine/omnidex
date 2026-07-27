package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) UpdateProjectSetting(ctx context.Context, projectID int64, key string, value json.RawMessage) error {
	if r == nil || r.pool == nil || ctx == nil || projectID <= 0 {
		return fmt.Errorf("PostgreSQL, context, and project are required for project settings")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("project setting key is required")
	}
	for _, removed := range removedProjectPlanningSettingKeys {
		if key == removed {
			return fmt.Errorf("project setting %q was removed; use the PostgreSQL project planning API", key)
		}
	}
	var decoded any
	if len(value) == 0 {
		return fmt.Errorf("project setting %q requires JSON", key)
	}
	if err := json.Unmarshal(value, &decoded); err != nil {
		return fmt.Errorf("project setting %q requires valid JSON: %w", key, err)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer rollbackTx(ctx, tx, "project setting update")
	var settingsType string
	if err := tx.QueryRow(ctx, `SELECT jsonb_typeof(settings) FROM projects WHERE id = $1 FOR UPDATE`, projectID).Scan(&settingsType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrProjectNotFound
		}
		return err
	}
	if settingsType != "object" {
		return fmt.Errorf("project %d settings must be a JSON object, received %s", projectID, settingsType)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE projects
		SET settings = jsonb_set(settings, ARRAY[$2::text], $3::jsonb, true),
		    last_seen_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
	`, projectID, key, string(value))
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrProjectNotFound
	}
	return tx.Commit(ctx)
}
