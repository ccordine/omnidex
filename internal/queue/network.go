package queue

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/gryph/omnidex/internal/network"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) GetCoreURL(ctx context.Context) (string, error) {
	if r == nil || r.pool == nil {
		return "", nil
	}
	var raw []byte
	err := r.pool.QueryRow(ctx, `
		SELECT value
		FROM workspace_settings
		WHERE key = $1
	`, network.WorkspaceKey).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	var payload struct {
		URL string `json:"url"`
	}
	if len(raw) == 0 {
		return "", nil
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	return strings.TrimSpace(payload.URL), nil
}

func (r *Repository) SetCoreURL(ctx context.Context, coreURL string) (string, error) {
	if r == nil || r.pool == nil {
		return "", errors.New("repository unavailable")
	}
	coreURL = network.NormalizeCoreURL(coreURL)
	value, err := json.Marshal(map[string]string{"url": coreURL})
	if err != nil {
		return "", err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO workspace_settings (key, value, updated_at)
		VALUES ($1, $2::jsonb, NOW())
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value,
		    updated_at = NOW()
	`, network.WorkspaceKey, string(value))
	if err != nil {
		return "", err
	}
	return coreURL, nil
}
