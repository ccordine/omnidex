package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
)

func (s *Server) scrumMoveCard(r *http.Request, cardID, column, beforeCardID string) (ScrumCard, error) {
	column = normalizeScrumColumn(column)
	if column == "" {
		return ScrumCard{}, fmt.Errorf("invalid column")
	}
	beforeCardID = strings.TrimSpace(beforeCardID)

	if s.repo == nil {
		return ScrumCard{}, fmt.Errorf("postgres repository is required for Scrum")
	}
	projectID, err := s.resolveProjectID(r)
	if err != nil {
		return ScrumCard{}, err
	}
	board, err := s.scrumBoardFromProject(r.Context(), projectID)
	if err != nil {
		return ScrumCard{}, err
	}
	before := map[string]ScrumCard{}
	for _, card := range board.Cards {
		before[card.ID] = card
	}
	updated, changed, err := placeScrumCard(&board, cardID, column, beforeCardID)
	if err != nil {
		return ScrumCard{}, err
	}
	placements := make([]queue.ScrumCardPlacement, 0, len(changed))
	for _, card := range changed {
		placements = append(placements, queue.ScrumCardPlacement{
			CardID:     card.ID,
			Column:     card.Column,
			BoardOrder: card.BoardOrder,
		})
	}
	if err := s.repo.PlaceScrumCards(r.Context(), projectID, placements); err != nil {
		return ScrumCard{}, err
	}
	for _, card := range changed {
		previous := before[card.ID]
		metrics := s.trackScrumCardFlow(r.Context(), projectID, previous, card, "move")
		if card.ID == updated.ID {
			updated.FlowMetrics = metrics
		}
	}
	if normalizeScrumColumn(before[updated.ID].Column) != "review" && normalizeScrumColumn(updated.Column) == "review" {
		reviewed, reviewErr := s.maybeStartScrumAutoReview(r, projectID, board, updated, before[updated.ID].Column)
		if reviewErr != nil {
			return ScrumCard{}, fmt.Errorf("start Scrum auto-review: %w", reviewErr)
		}
		updated = reviewed
	}
	if err := s.ReconcileScrumPlayQueueForProjectAsync(projectID); err != nil {
		return ScrumCard{}, err
	}
	return updated, nil
}

func placeScrumCard(board *ScrumBoard, cardID, column, beforeCardID string) (ScrumCard, []ScrumCard, error) {
	movedIdx := -1
	for i, card := range board.Cards {
		if card.ID == cardID {
			movedIdx = i
			break
		}
	}
	if movedIdx < 0 {
		return ScrumCard{}, nil, fmt.Errorf("card not found")
	}

	moved := board.Cards[movedIdx]
	sourceColumn := normalizeScrumColumn(moved.Column)
	if sourceColumn == "" {
		sourceColumn = "backlog"
	}
	moved.Column = column

	targetCards := columnCards(board, column, cardID)
	insertAt := len(targetCards)
	if beforeCardID != "" {
		for i, card := range targetCards {
			if card.ID == beforeCardID {
				insertAt = i
				break
			}
		}
	}
	targetCards = append(targetCards[:insertAt], append([]ScrumCard{moved}, targetCards[insertAt:]...)...)
	assignBoardOrders(column, targetCards)

	changed := make([]ScrumCard, 0, len(targetCards)+len(board.Cards))
	changed = append(changed, targetCards...)
	if sourceColumn != column {
		sourceCards := columnCards(board, sourceColumn, cardID)
		assignBoardOrders(sourceColumn, sourceCards)
		changed = append(changed, sourceCards...)
	}

	changedMap := map[string]ScrumCard{}
	for _, card := range changed {
		changedMap[card.ID] = card
	}
	for i, card := range board.Cards {
		if updated, ok := changedMap[card.ID]; ok {
			board.Cards[i] = updated
		}
	}

	updated := changedMap[cardID]
	return updated, uniqueScrumCards(changed), nil
}

func columnCards(board *ScrumBoard, column, excludeID string) []ScrumCard {
	column = normalizeScrumColumn(column)
	out := make([]ScrumCard, 0)
	for _, card := range board.Cards {
		if card.ID == excludeID {
			continue
		}
		if normalizeScrumColumn(card.Column) == column {
			out = append(out, card)
		}
	}
	sortCardsForColumn(column, out)
	return out
}

func assignBoardOrders(column string, cards []ScrumCard) {
	if column == "assigned" {
		next := 0
		for i := range cards {
			if cards[i].PlayState == scrumPlayQueued {
				continue
			}
			cards[i].BoardOrder = next
			next++
		}
		for i := range cards {
			if cards[i].PlayState != scrumPlayQueued {
				continue
			}
			cards[i].BoardOrder = next
			next++
		}
		return
	}
	if column == "in_progress" {
		next := 0
		for i := range cards {
			if cards[i].PlayState == scrumPlayRunning {
				cards[i].BoardOrder = next
				next++
			}
		}
		for i := range cards {
			if cards[i].PlayState == scrumPlayRunning {
				continue
			}
			cards[i].BoardOrder = next
			next++
		}
		return
	}
	for i := range cards {
		cards[i].BoardOrder = i
	}
}

func uniqueScrumCards(cards []ScrumCard) []ScrumCard {
	seen := map[string]struct{}{}
	out := make([]ScrumCard, 0, len(cards))
	for _, card := range cards {
		if _, ok := seen[card.ID]; ok {
			continue
		}
		seen[card.ID] = struct{}{}
		out = append(out, card)
	}
	return out
}
