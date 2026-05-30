package queue

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/jackc/pgx/v5"
)

const WorkspaceAgentConfigKey = agentconfig.WorkspaceSettingsKey

func (r *Repository) GetWorkspaceAgentConfig(ctx context.Context) (map[string]string, error) {
	if r == nil || r.pool == nil {
		return map[string]string{}, nil
	}
	var raw []byte
	err := r.pool.QueryRow(ctx, `
		SELECT value
		FROM workspace_settings
		WHERE key = $1
	`, WorkspaceAgentConfigKey).Scan(&raw)
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
	clean := map[string]string{}
	for key, value := range out {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			clean[key] = value
		}
	}
	return clean, nil
}

func (r *Repository) SetWorkspaceAgentConfig(ctx context.Context, values map[string]string) (map[string]string, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("repository unavailable")
	}
	clean := map[string]string{}
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			clean[key] = value
		}
	}
	payload, err := json.Marshal(clean)
	if err != nil {
		return nil, err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO workspace_settings (key, value, updated_at)
		VALUES ($1, $2::jsonb, NOW())
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value,
		    updated_at = NOW()
	`, WorkspaceAgentConfigKey, string(payload))
	if err != nil {
		return nil, err
	}
	return clean, nil
}
