package queue

import (
	"context"
	"fmt"
	"strings"
)

type ScrumPlayQueueSnapshot struct {
	RunningCardID string
	QueuedCount   int
	QueuedCardIDs []string
	QueuedHasMore bool
}

func (r *Repository) FindRunningScrumCard(ctx context.Context, projectID int64) (DBScrumCard, bool, error) {
	return r.findOneScrumCard(ctx, projectID, `play_state = 'running'`, nil, true)
}

func (r *Repository) FindNextQueuedScrumCard(ctx context.Context, projectID int64) (DBScrumCard, bool, error) {
	return r.findOneScrumCard(ctx, projectID, `play_state = 'queued'`, []string{"queue_order ASC", "updated_at ASC", "id ASC"}, false)
}

func (r *Repository) FindNextEligibleScrumCard(ctx context.Context, projectID int64, columns []string) (DBScrumCard, bool, error) {
	if projectID <= 0 || len(columns) == 0 {
		return DBScrumCard{}, false, fmt.Errorf("eligible Scrum card query requires project and columns")
	}
	rows, err := r.pool.Query(ctx, scrumCardSelectionSQL+`
		AND column_name = ANY($2::text[])
		AND play_state NOT IN ('running','queued')
		ORDER BY array_position($2::text[], column_name), board_order ASC, updated_at DESC, id ASC
		LIMIT 1
	`, projectID, columns)
	if err != nil {
		return DBScrumCard{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return DBScrumCard{}, false, rows.Err()
	}
	card, err := scanDBScrumCard(rows)
	return card, err == nil, err
}

func (r *Repository) findOneScrumCard(ctx context.Context, projectID int64, predicate string, order []string, unique bool) (DBScrumCard, bool, error) {
	if projectID <= 0 {
		return DBScrumCard{}, false, fmt.Errorf("Scrum card query requires a positive project id")
	}
	if strings.TrimSpace(predicate) == "" {
		return DBScrumCard{}, false, fmt.Errorf("Scrum card query predicate is required")
	}
	orderSQL := "updated_at DESC, id ASC"
	if len(order) > 0 {
		orderSQL = strings.Join(order, ", ")
	}
	limit := 1
	if unique {
		limit = 2
	}
	rows, err := r.pool.Query(ctx, scrumCardSelectionSQL+" AND "+predicate+" ORDER BY "+orderSQL+" LIMIT $2", projectID, limit)
	if err != nil {
		return DBScrumCard{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return DBScrumCard{}, false, rows.Err()
	}
	card, err := scanDBScrumCard(rows)
	if err != nil {
		return DBScrumCard{}, false, err
	}
	if unique && rows.Next() {
		return DBScrumCard{}, false, fmt.Errorf("Scrum invariant rejected multiple cards matching %s", predicate)
	}
	return card, true, rows.Err()
}

func (r *Repository) ScrumPlayQueueSnapshot(ctx context.Context, projectID int64, queuedLimit int) (ScrumPlayQueueSnapshot, error) {
	if projectID <= 0 || queuedLimit < 1 || queuedLimit > MaxScrumCardPageSize {
		return ScrumPlayQueueSnapshot{}, fmt.Errorf("Scrum play queue requires project and a bounded queued limit")
	}
	var snapshot ScrumPlayQueueSnapshot
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(id) FILTER (WHERE play_state='running'), ''),
		       COUNT(*) FILTER (WHERE play_state='queued')
		FROM scrum_cards WHERE project_id=$1
	`, projectID).Scan(&snapshot.RunningCardID, &snapshot.QueuedCount)
	if err != nil {
		return ScrumPlayQueueSnapshot{}, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id FROM scrum_cards
		WHERE project_id=$1 AND play_state='queued'
		ORDER BY queue_order ASC, updated_at ASC, id ASC
		LIMIT $2
	`, projectID, queuedLimit)
	if err != nil {
		return ScrumPlayQueueSnapshot{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return ScrumPlayQueueSnapshot{}, err
		}
		snapshot.QueuedCardIDs = append(snapshot.QueuedCardIDs, id)
	}
	snapshot.QueuedHasMore = snapshot.QueuedCount > len(snapshot.QueuedCardIDs)
	return snapshot, rows.Err()
}

func (r *Repository) ScrumQueueOrderAndPosition(ctx context.Context, projectID int64, cardID string) (int, int, error) {
	cardID = strings.TrimSpace(cardID)
	if projectID <= 0 || cardID == "" {
		return 0, 0, fmt.Errorf("Scrum queue order requires project and card")
	}
	var nextOrder, position int
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(queue_order) FILTER (WHERE play_state='queued'),0)+1,
		       COUNT(*) FILTER (WHERE play_state='queued')+1
		FROM scrum_cards WHERE project_id=$1
	`, projectID).Scan(&nextOrder, &position)
	return nextOrder, position, err
}

func (r *Repository) ScrumProjectComplete(ctx context.Context, projectID int64) (bool, error) {
	var total int
	var incomplete bool
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(BOOL_OR(column_name NOT IN ('review','done')), false)
		FROM scrum_cards WHERE project_id=$1
	`, projectID).Scan(&total, &incomplete)
	return total > 0 && !incomplete, err
}

const scrumCardSelectionSQL = `
	SELECT id, project_id, title, description, column_name, checklist, ref_files, chat,
	       model_config, agent_config, card_ticket, card_prompt, recipe_id, recipe,
	       tags, planning_chat, coach_config, test_criteria, flow_metrics,
	       job_id, tags_job_id, ticket_job_id, console_log, play_state, queue_order, board_order,
	       sync_job_id, agent_stream_chat_cursor, agent_stream_console_cursor, step_context_cursor,
	       created_at, updated_at
	FROM scrum_cards WHERE project_id=$1`
