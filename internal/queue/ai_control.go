package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const aiControlKey = "ai_control"

type AIControlState struct {
	Paused bool `json:"paused"`
}

func (r *Repository) GetAIControlState(ctx context.Context) (AIControlState, error) {
	if r == nil || r.pool == nil {
		return AIControlState{}, fmt.Errorf("postgres repository is not configured")
	}
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
	var stored struct {
		Paused *bool `json:"paused"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&stored); err != nil {
		return AIControlState{}, fmt.Errorf("decode AI control state: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return AIControlState{}, fmt.Errorf("AI control state contains trailing data")
	}
	if stored.Paused == nil {
		return AIControlState{}, fmt.Errorf("AI control state requires paused")
	}
	return AIControlState{Paused: *stored.Paused}, nil
}

func (r *Repository) IsAIPaused(ctx context.Context) (bool, error) {
	state, err := r.GetAIControlState(ctx)
	return state.Paused, err
}

func (r *Repository) SetAIPaused(ctx context.Context, paused bool) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("postgres repository is not configured")
	}
	_, err := r.pool.Exec(ctx, setAIPausedSQL, aiControlKey, paused)
	return err
}

const setAIPausedSQL = `
	INSERT INTO workspace_settings (key, value, updated_at)
	VALUES ($1, jsonb_build_object('paused', $2::boolean), NOW())
	ON CONFLICT (key) DO UPDATE
	SET value = EXCLUDED.value,
	    updated_at = NOW()
`

// PauseAI atomically makes the pause authoritative and cancels active work.
func (r *Repository) PauseAI(ctx context.Context, reason string) (projectIDs, jobIDs []int64, err error) {
	if r == nil || r.pool == nil {
		return nil, nil, fmt.Errorf("postgres repository is not configured")
	}
	if ctx == nil {
		return nil, nil, fmt.Errorf("AI pause context is required")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, nil, fmt.Errorf("AI pause reason is required")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, nil, err
	}
	defer rollbackTx(ctx, tx, "global AI pause")
	if _, err := tx.Exec(ctx, setAIPausedSQL, aiControlKey, true); err != nil {
		return nil, nil, err
	}
	projectIDs, err = queryInt64s(ctx, tx, `
		SELECT DISTINCT project_id
		FROM scrum_cards
		WHERE play_state = 'running'
		ORDER BY project_id
	`)
	if err != nil {
		return nil, nil, err
	}
	jobIDs, err = queryInt64s(ctx, tx, `
		SELECT id
		FROM jobs
		WHERE status IN ($1, $2)
		ORDER BY id
	`, model.JobStatusRunning, model.JobStatusWaiting)
	if err != nil {
		return nil, nil, err
	}
	for _, jobID := range jobIDs {
		operationID, err := NewLifecycleOperationID(
			"global-ai-pause-cancel-v1", strconv.FormatInt(jobID, 10),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("build cancellation identity for job %d: %w", jobID, err)
		}
		if _, err := cancelJobTx(ctx, tx, CancelJobCommand{
			OperationID: operationID, JobID: jobID, Reason: reason,
		}); err != nil {
			return nil, nil, fmt.Errorf("cancel job %d during global AI pause: %w", jobID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return projectIDs, jobIDs, nil
}

type int64RowsQuery interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func queryInt64s(ctx context.Context, query int64RowsQuery, statement string, args ...any) ([]int64, error) {
	rows, err := query.Query(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]int64, 0)
	for rows.Next() {
		var value int64
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return values, nil
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
