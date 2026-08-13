package api

import (
	"context"

	"github.com/gryph/omnidex/internal/queue"
)

const scrumQueuedSummaryLimit = 20

func (s *Server) scrumPlayQueuePayload(ctx context.Context, projectID int64) (map[string]any, error) {
	snapshot, err := s.repo.ScrumPlayQueueSnapshot(ctx, projectID, scrumQueuedSummaryLimit)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"running_card_id": snapshot.RunningCardID,
		"queued_count": snapshot.QueuedCount,
		"queued_card_ids": snapshot.QueuedCardIDs,
		"queued_has_more": snapshot.QueuedHasMore,
	}, nil
}

func (s *Server) runningScrumCard(ctx context.Context, projectID int64) (*ScrumCard, error) {
	stored, found, err := s.repo.FindRunningScrumCard(ctx, projectID)
	if err != nil || !found {
		return nil, err
	}
	card, err := dbScrumCardToAPI(stored)
	return &card, err
}

func (s *Server) nextScrumWorkCard(ctx context.Context, projectID int64, columns []string) (*ScrumCard, error) {
	stored, found, err := s.repo.FindNextQueuedScrumCard(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if !found {
		stored, found, err = s.repo.FindNextEligibleScrumCard(ctx, projectID, columns)
		if err != nil || !found {
			return nil, err
		}
	}
	card, err := dbScrumCardToAPI(stored)
	return &card, err
}

func scrumBoardWithCards(board ScrumBoard, cards ...ScrumCard) ScrumBoard {
	board.Cards = append([]ScrumCard(nil), cards...)
	return board
}

var _ = queue.MaxScrumCardPageSize
