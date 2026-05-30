package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ScrumFlowEvent struct {
	ID            int64
	ProjectID     int64
	CardID        string
	EventType     string
	FromColumn    string
	ToColumn      string
	FromPlayState string
	ToPlayState   string
	Payload       json.RawMessage
	CreatedAt     time.Time
}

func (r *Repository) RecordScrumFlowEvent(ctx context.Context, projectID int64, cardID, eventType, fromColumn, toColumn, fromPlayState, toPlayState string, payload json.RawMessage) error {
	if r == nil || projectID <= 0 {
		return fmt.Errorf("database unavailable")
	}
	cardID = strings.TrimSpace(cardID)
	eventType = strings.TrimSpace(eventType)
	if cardID == "" || eventType == "" {
		return fmt.Errorf("card id and event type are required")
	}
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO scrum_flow_events (
			project_id, card_id, event_type,
			from_column, to_column, from_play_state, to_play_state, payload
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
	`, projectID, cardID, eventType, strings.TrimSpace(fromColumn), strings.TrimSpace(toColumn), strings.TrimSpace(fromPlayState), strings.TrimSpace(toPlayState), string(payload))
	return err
}

func (r *Repository) ListScrumFlowEvents(ctx context.Context, projectID int64, cardID string, limit int) ([]ScrumFlowEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	cardID = strings.TrimSpace(cardID)
	query := `
		SELECT id, project_id, card_id, event_type, from_column, to_column,
		       from_play_state, to_play_state, payload, created_at
		FROM scrum_flow_events
		WHERE project_id = $1
	`
	args := []any{projectID}
	if cardID != "" {
		query += ` AND card_id = $2`
		args = append(args, cardID)
	}
	query += ` ORDER BY created_at DESC LIMIT $` + fmt.Sprint(len(args)+1)
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ScrumFlowEvent{}
	for rows.Next() {
		var event ScrumFlowEvent
		var payload []byte
		if err := rows.Scan(
			&event.ID,
			&event.ProjectID,
			&event.CardID,
			&event.EventType,
			&event.FromColumn,
			&event.ToColumn,
			&event.FromPlayState,
			&event.ToPlayState,
			&payload,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}
		event.Payload = json.RawMessage(payload)
		out = append(out, event)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateScrumCardFlowMetrics(ctx context.Context, projectID int64, cardID string, metrics json.RawMessage) error {
	if len(metrics) == 0 {
		metrics = json.RawMessage(`{}`)
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE scrum_cards
		SET flow_metrics = $3::jsonb, updated_at = NOW()
		WHERE project_id = $1 AND id = $2
	`, projectID, strings.TrimSpace(cardID), string(metrics))
	return err
}

func (r *Repository) CountScrumFlowEventsByType(ctx context.Context, projectID int64, cardID, eventType string) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM scrum_flow_events
		WHERE project_id = $1 AND card_id = $2 AND event_type = $3
	`, projectID, strings.TrimSpace(cardID), strings.TrimSpace(eventType)).Scan(&count)
	return count, err
}
