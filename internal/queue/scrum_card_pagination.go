package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const MaxScrumCardPageSize = 100

type ScrumCardPageRequest struct {
	Column string
	Limit  int
	Offset int
}

// DBScrumCardSummary is the exact bounded projection used by board and metrics
// lists. Growing channel, planning, and console bodies are deliberately absent.
type DBScrumCardSummary struct {
	ID                string
	ProjectID         int64
	Title             string
	Description       string
	Column            string
	ChecklistDone     int
	ChecklistTotal    int
	RefFileCount      int
	ChatCount         int64
	TestCriteriaDone  int
	TestCriteriaTotal int
	HasCardTicket     bool
	Tags              json.RawMessage
	FlowMetrics       json.RawMessage
	JobID             string
	PlayState         string
	QueueOrder        int
	BoardOrder        int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type ScrumCardPage struct {
	Items   []DBScrumCardSummary
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
	rows, err := r.pool.Query(ctx, scrumCardSummaryPageSQL, projectID, column, request.Limit+1, request.Offset)
	if err != nil {
		return ScrumCardPage{}, err
	}
	defer rows.Close()
	items := make([]DBScrumCardSummary, 0, request.Limit+1)
	for rows.Next() {
		card, err := scanDBScrumCardSummary(rows)
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

func scanDBScrumCardSummary(row pgx.Row) (DBScrumCardSummary, error) {
	var card DBScrumCardSummary
	var checklistValid, refFilesValid, testCriteriaValid bool
	err := row.Scan(
		&card.ID, &card.ProjectID, &card.Title, &card.Description, &card.Column,
		&card.ChecklistDone, &card.ChecklistTotal, &card.RefFileCount,
		&card.ChatCount, &card.TestCriteriaDone, &card.TestCriteriaTotal,
		&card.HasCardTicket, &card.Tags, &card.FlowMetrics, &card.JobID,
		&card.PlayState, &card.QueueOrder, &card.BoardOrder, &card.CreatedAt, &card.UpdatedAt,
		&checklistValid, &refFilesValid, &testCriteriaValid,
	)
	if err != nil {
		return DBScrumCardSummary{}, err
	}
	for _, field := range []struct {
		name  string
		valid bool
	}{
		{"checklist", checklistValid}, {"ref_files", refFilesValid}, {"test_criteria", testCriteriaValid},
	} {
		if !field.valid {
			return DBScrumCardSummary{}, fmt.Errorf("Scrum card %q %s must contain the registered typed JSON array", card.ID, field.name)
		}
	}
	if err := validateStoredScrumCardSummary(card); err != nil {
		return DBScrumCardSummary{}, err
	}
	return card, nil
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
