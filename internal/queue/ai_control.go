package queue

import (
	"context"
	"encoding/json"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const aiControlKey = "ai_control"

type AIControlState struct {
	Paused bool `json:"paused"`
}

func (r *Repository) GetAIControlState(ctx context.Context) (AIControlState, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `
		SELECT value
		FROM workspace_settings
		WHERE key = $1
	`, aiControlKey).Scan(&raw)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AIControlState{}, nil
		}
		return AIControlState{}, err
	}
	var state AIControlState
	_ = json.Unmarshal(raw, &state)
	return state, nil
}

func (r *Repository) IsAIPaused(ctx context.Context) (bool, error) {
	state, err := r.GetAIControlState(ctx)
	return state.Paused, err
}

func (r *Repository) SetAIPaused(ctx context.Context, paused bool) error {
	value, err := json.Marshal(AIControlState{Paused: paused})
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO workspace_settings (key, value, updated_at)
		VALUES ($1, $2::jsonb, NOW())
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value,
		    updated_at = NOW()
	`, aiControlKey, string(value))
	return err
}

func (r *Repository) ListJobIDsByStatuses(ctx context.Context, statuses ...string) ([]int64, error) {
	if len(statuses) == 0 {
		return []int64{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id
		FROM jobs
		WHERE status = ANY($1)
		ORDER BY id ASC
	`, statuses)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) CountJobsByStatuses(ctx context.Context) (map[string]int64, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT status, COUNT(*)
		FROM jobs
		WHERE status IN ($1, $2, $3)
		GROUP BY status
	`, model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int64{
		model.JobStatusPending: 0,
		model.JobStatusRunning: 0,
		model.JobStatusWaiting: 0,
	}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, rows.Err()
}
