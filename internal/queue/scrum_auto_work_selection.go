package queue

import (
	"context"
	"fmt"
	"time"
)

// ScrumAutoWorkCandidate is one database-selected next card. The scheduler
// never materializes the project or card catalogs to make this decision.
type ScrumAutoWorkCandidate struct {
	ProjectID int64
	CardID    string
	QueuedAt  time.Time
}

func (r *Repository) FindGlobalScrumAutoWorkCandidate(ctx context.Context) (ScrumAutoWorkCandidate, bool, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return ScrumAutoWorkCandidate{}, false, fmt.Errorf("PostgreSQL and context are required for global Scrum auto-work selection")
	}
	rows, err := r.pool.Query(ctx, `
		WITH configured AS (
			SELECT id,
			       CASE
			         WHEN jsonb_typeof(settings #> '{scrum_auto_work,source_columns}')='array'
			          AND jsonb_array_length(settings #> '{scrum_auto_work,source_columns}') > 0
			         THEN ARRAY(SELECT jsonb_array_elements_text(settings #> '{scrum_auto_work,source_columns}'))
			         ELSE ARRAY['assigned']::text[]
			       END AS source_columns
			FROM projects
			WHERE COALESCE(settings #>> '{scrum_auto_work,enabled}','false')='true'
		), candidates AS (
			SELECT project.id AS project_id, card.id AS card_id, card.updated_at
			FROM configured project
			JOIN LATERAL (
				SELECT candidate.id, candidate.updated_at
				FROM scrum_cards candidate
				WHERE candidate.project_id=project.id
				  AND NOT EXISTS (
					SELECT 1 FROM scrum_cards running
					WHERE running.project_id=project.id AND running.play_state='running'
				  )
				  AND (
					candidate.play_state='queued'
					OR (candidate.play_state NOT IN ('running','queued')
					    AND candidate.column_name=ANY(project.source_columns))
				  )
				ORDER BY CASE WHEN candidate.play_state='queued' THEN 0 ELSE 1 END,
				         candidate.queue_order ASC,
				         array_position(project.source_columns, candidate.column_name),
				         candidate.board_order ASC, candidate.updated_at ASC, candidate.id ASC
				LIMIT 1
			) card ON true
		)
		SELECT project_id, card_id, updated_at
		FROM candidates
		ORDER BY updated_at ASC, project_id ASC, card_id ASC
		LIMIT 1
	`)
	if err != nil {
		return ScrumAutoWorkCandidate{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return ScrumAutoWorkCandidate{}, false, rows.Err()
	}
	var candidate ScrumAutoWorkCandidate
	if err := rows.Scan(&candidate.ProjectID, &candidate.CardID, &candidate.QueuedAt); err != nil {
		return ScrumAutoWorkCandidate{}, false, err
	}
	return candidate, true, rows.Err()
}

func (r *Repository) RunningScrumPlayProjectID(ctx context.Context) (int64, bool, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return 0, false, fmt.Errorf("PostgreSQL and context are required for running Scrum selection")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT project_id
		FROM scrum_cards
		WHERE play_state='running'
		ORDER BY project_id ASC
		LIMIT 2
	`)
	if err != nil {
		return 0, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, false, rows.Err()
	}
	var projectID int64
	if err := rows.Scan(&projectID); err != nil {
		return 0, false, err
	}
	if rows.Next() {
		return 0, false, fmt.Errorf("global Scrum invariant rejected running cards in multiple projects")
	}
	return projectID, true, rows.Err()
}
