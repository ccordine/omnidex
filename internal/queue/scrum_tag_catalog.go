package queue

import (
	"context"
	"fmt"
	"strings"
)

const MaxScrumTagPageSize = 100

func (r *Repository) ListScrumCardTags(ctx context.Context, projectID int64, query string, limit int) ([]string, error) {
	if projectID <= 0 {
		return nil, fmt.Errorf("Scrum tag catalog requires a positive project id")
	}
	if limit < 1 || limit > MaxScrumTagPageSize {
		return nil, fmt.Errorf("Scrum tag catalog limit must be between 1 and %d", MaxScrumTagPageSize)
	}
	query = strings.ToLower(strings.TrimSpace(query))
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT LOWER(BTRIM(tag.value)) AS normalized_tag
		FROM scrum_cards AS card
		CROSS JOIN LATERAL jsonb_array_elements_text(
			CASE WHEN jsonb_typeof(card.tags) = 'array' THEN card.tags ELSE '[]'::jsonb END
		) AS tag(value)
		WHERE card.project_id = $1
		  AND BTRIM(tag.value) <> ''
		  AND ($2 = '' OR STRPOS(LOWER(BTRIM(tag.value)), $2) > 0)
		ORDER BY normalized_tag ASC
		LIMIT $3
	`, projectID, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tags := make([]string, 0, limit)
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}
