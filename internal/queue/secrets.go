package queue

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/gryph/omnidex/internal/secrets"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) GetAPISecrets(ctx context.Context) (map[string]string, error) {
	if r == nil || r.pool == nil {
		return map[string]string{}, nil
	}
	var raw []byte
	err := r.pool.QueryRow(ctx, `
		SELECT value
		FROM workspace_settings
		WHERE key = $1
	`, secrets.WorkspaceKey).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	out := map[string]string{}
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if err := secrets.ValidateStored(out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) SetAPISecrets(ctx context.Context, updates map[string]string, clearKeys []string) (map[string]string, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("repository unavailable")
	}
	current, err := r.GetAPISecrets(ctx)
	if err != nil {
		return nil, err
	}
	merged, err := secrets.MergeStored(current, updates, clearKeys)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(merged)
	if err != nil {
		return nil, err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO workspace_settings (key, value, updated_at)
		VALUES ($1, $2::jsonb, NOW())
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value,
		    updated_at = NOW()
	`, secrets.WorkspaceKey, string(payload))
	if err != nil {
		return nil, err
	}
	return merged, nil
}
