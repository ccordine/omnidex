package api

import (
	"context"
)

const scrumQueuedSummaryLimit = 20

func (s *Server) scrumPlayQueuePayload(ctx context.Context, projectID int64) (map[string]any, error) {
	snapshot, err := s.repo.ScrumPlayQueueSnapshot(ctx, projectID, scrumQueuedSummaryLimit)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"running_card_id": snapshot.RunningCardID,
		"queued_count":    snapshot.QueuedCount,
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
	seen := make(map[string]struct{}, len(cards))
	board.Cards = make([]ScrumCard, 0, len(cards))
	for _, card := range cards {
		if card.ID == "" {
			continue
		}
		if _, exists := seen[card.ID]; exists {
			continue
		}
		seen[card.ID] = struct{}{}
		board.Cards = append(board.Cards, card)
	}
	return board
}

func (s *Server) scrumFocusBoard(ctx context.Context, projectID int64, board ScrumBoard, autoWork ScrumAutoWorkConfig) (ScrumBoard, error) {
	cards := make([]ScrumCard, 0, 2)
	running, err := s.runningScrumCard(ctx, projectID)
	if err != nil {
		return board, err
	}
	if running != nil {
		cards = append(cards, *running)
	}
	columns := []string{"in_progress", "assigned"}
	if autoWork.Enabled {
		validated, err := validateScrumAutoWorkConfig(autoWork)
		if err != nil {
			return board, err
		}
		columns = validated.SourceColumns
	}
	next, err := s.nextScrumWorkCard(ctx, projectID, columns)
	if err != nil {
		return board, err
	}
	if next != nil {
		cards = append(cards, *next)
	}
	return scrumBoardWithCards(board, cards...), nil
}
