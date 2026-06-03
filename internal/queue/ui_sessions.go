package queue

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type UISession struct {
	SessionID string          `json:"session_id"`
	State     json.RawMessage `json:"state"`
	UpdatedAt time.Time       `json:"updated_at"`
	ExpiresAt time.Time       `json:"expires_at"`
}

func (r *Repository) GetUISession(ctx context.Context, sessionID string) (UISession, bool, error) {
	var session UISession
	if sessionID == "" {
		return session, false, nil
	}
	err := r.pool.QueryRow(ctx, `
		SELECT session_id, state_json, updated_at, expires_at
		FROM ui_sessions
		WHERE session_id = $1 AND expires_at > NOW()
	`, sessionID).Scan(&session.SessionID, &session.State, &session.UpdatedAt, &session.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return UISession{}, false, nil
	}
	if err != nil {
		return UISession{}, false, err
	}
	return session, true, nil
}

func (r *Repository) UpsertUISession(ctx context.Context, sessionID string, state json.RawMessage, ttl time.Duration) (UISession, error) {
	if sessionID == "" {
		return UISession{}, errors.New("session id is required")
	}
	if len(state) == 0 {
		state = json.RawMessage(`{}`)
	}
	if ttl < time.Minute {
		ttl = 30 * time.Minute
	}
	var session UISession
	ttlSeconds := int64(ttl.Seconds())
	err := r.pool.QueryRow(ctx, `
		INSERT INTO ui_sessions (session_id, state_json, expires_at)
		VALUES ($1, $2::jsonb, NOW() + ($3::bigint * INTERVAL '1 second'))
		ON CONFLICT (session_id) DO UPDATE
		SET state_json = EXCLUDED.state_json,
		    updated_at = NOW(),
		    expires_at = EXCLUDED.expires_at
		RETURNING session_id, state_json, updated_at, expires_at
	`, sessionID, string(state), ttlSeconds).Scan(&session.SessionID, &session.State, &session.UpdatedAt, &session.ExpiresAt)
	return session, err
}

func (r *Repository) PruneExpiredUISessions(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM ui_sessions WHERE expires_at <= NOW()`)
	return err
}
