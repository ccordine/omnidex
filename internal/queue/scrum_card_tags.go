package queue

import (
	"context"
	"fmt"
	"strings"
)

func (r *Repository) ListScrumCardTagValues(ctx context.Context, projectID int64, limit int) ([]string, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("postgres repository is not configured")
	}
	if projectID <= 0 {
		return nil, fmt.Errorf("project_id is required")
	}
	if limit <= 0 || limit > 500 {
		return nil, fmt.Errorf("tag limit must be between 1 and 500")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT btrim(tag.value)
		FROM scrum_cards AS card
		CROSS JOIN LATERAL jsonb_array_elements_text(card.tags) AS tag(value)
		WHERE card.project_id = $1
		  AND btrim(tag.value) <> ''
		ORDER BY btrim(tag.value)
		LIMIT $2
	`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]string, 0, limit)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, strings.TrimSpace(value))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return values, nil
}
