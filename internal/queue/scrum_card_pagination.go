package queue

import (
	"context"
	"fmt"
	"strings"
)

const MaxScrumCardPageSize = 100

type ScrumCardPageRequest struct {
	Column string
	Limit  int
	Offset int
}

type ScrumCardPage struct {
	Items   []DBScrumCard
	Offset  int
	HasMore bool
}

func (request ScrumCardPageRequest) validate(projectID int64) error {
	if projectID <= 0 {
		return fmt.Errorf("Scrum card page requires a positive project id")
	}
	if request.Limit < 1 || request.Limit > MaxScrumCardPageSize {
		return fmt.Errorf("Scrum card page limit must be between 1 and %d", MaxScrumCardPageSize)
	}
	if request.Offset < 0 {
		return fmt.Errorf("Scrum card page offset must be non-negative")
	}
	return nil
}

func (r *Repository) ListScrumCardPage(ctx context.Context, projectID int64, request ScrumCardPageRequest) (ScrumCardPage, error) {
	if err := request.validate(projectID); err != nil {
		return ScrumCardPage{}, err
	}
	column := strings.TrimSpace(request.Column)
	rows, err := r.pool.Query(ctx, `
		SELECT id, project_id, title, description, column_name, checklist, ref_files, chat,
		       model_config, agent_config, card_ticket, card_prompt, recipe_id, recipe,
		       tags, planning_chat, coach_config, test_criteria, flow_metrics,
		       job_id, tags_job_id, ticket_job_id, console_log, play_state, queue_order, board_order,
		       sync_job_id, agent_stream_chat_cursor, agent_stream_console_cursor, step_context_cursor,
		       created_at, updated_at
		FROM scrum_cards
		WHERE project_id = $1 AND ($2 = '' OR column_name = $2)
		ORDER BY
			CASE WHEN $2 = '' THEN updated_at END DESC,
			CASE WHEN $2 = '' THEN id END ASC,
			CASE WHEN $2 = 'assigned' AND play_state = 'queued' THEN 1 ELSE 0 END ASC,
			CASE WHEN $2 = 'assigned' AND play_state = 'queued' THEN queue_order ELSE 0 END ASC,
			CASE WHEN $2 = 'in_progress' AND play_state = 'running' THEN 0 ELSE 1 END ASC,
			board_order ASC, updated_at DESC, id ASC
		LIMIT $3 OFFSET $4
	`, projectID, column, request.Limit+1, request.Offset)
	if err != nil {
		return ScrumCardPage{}, err
	}
	defer rows.Close()
	items := make([]DBScrumCard, 0, request.Limit+1)
	for rows.Next() {
		card, err := scanDBScrumCard(rows)
		if err != nil {
			return ScrumCardPage{}, err
		}
		items = append(items, card)
	}
	if err := rows.Err(); err != nil {
		return ScrumCardPage{}, err
	}
	hasMore := len(items) > request.Limit
	if hasMore {
		items = items[:request.Limit]
	}
	return ScrumCardPage{Items: items, Offset: request.Offset, HasMore: hasMore}, nil
}

func (r *Repository) CountScrumCardsByColumn(ctx context.Context, projectID int64) (map[string]int, error) {
	if projectID <= 0 {
		return nil, fmt.Errorf("Scrum card counts require a positive project id")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT column_name, COUNT(*)
		FROM scrum_cards
		WHERE project_id = $1
		GROUP BY column_name
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var column string
		var count int
		if err := rows.Scan(&column, &count); err != nil {
			return nil, err
		}
		counts[column] = count
	}
	return counts, rows.Err()
}
