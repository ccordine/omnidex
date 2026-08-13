package api

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/queue"
)

const scrumCardUIPageSize = 20

func (s *Server) scrumCardColumnPage(ctx context.Context, projectID int64, column string, offset int) ([]ScrumCard, bool, error) {
	column = normalizeScrumColumn(column)
	if column == "" {
		return nil, false, fmt.Errorf("Scrum card page requires a registered column")
	}
	page, err := s.repo.ListScrumCardPage(ctx, projectID, queue.ScrumCardPageRequest{
		Column: column, Limit: scrumCardUIPageSize, Offset: offset,
	})
	if err != nil {
		return nil, false, err
	}
	cards := make([]ScrumCard, 0, len(page.Items))
	for _, item := range page.Items {
		card, err := dbScrumCardToAPI(item)
		if err != nil {
			return nil, false, fmt.Errorf("decode Scrum card %q: %w", item.ID, err)
		}
		cards = append(cards, scrumCardBoardSummary(card))
	}
	return cards, page.HasMore, nil
}

func (s *Server) scrumColumnCountsFromRepository(ctx context.Context, projectID int64) (map[string]int, error) {
	stored, err := s.repo.CountScrumCardsByColumn(ctx, projectID)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(scrumColumns))
	for _, column := range scrumColumns {
		counts[column] = stored[column]
	}
	return counts, nil
}
