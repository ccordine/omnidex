package api

import (
	"context"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

func (s *Server) scrumAvailable() bool {
	return s.repo != nil
}

func (s *Server) scrumCreateCard(
	ctx context.Context,
	projectID int64,
	title, description string,
	column queue.ScrumCardColumn,
) (ScrumCard, error) {
	if s.repo == nil || ctx == nil || projectID <= 0 {
		return ScrumCard{}, fmt.Errorf("postgres repository is required for Scrum")
	}
	card, err := s.repo.CreateScrumCard(ctx, projectID, "", title, description, string(column), nil, nil)
	if err != nil {
		return ScrumCard{}, err
	}
	return dbScrumCardToAPI(card)
}

func (s *Server) scrumDeleteCard(
	ctx context.Context,
	projectID int64,
	cardID string,
	expectedUpdatedAt time.Time,
) error {
	if s.repo == nil || ctx == nil || projectID <= 0 {
		return fmt.Errorf("postgres repository is required for Scrum")
	}
	return s.repo.DeleteScrumCardAtRevision(ctx, projectID, cardID, expectedUpdatedAt)
}

func (s *Server) scrumBoardResponse(
	ctx context.Context,
	projectID int64,
	visibleColumn string,
	cardOffset int,
) (map[string]any, error) {
	if ctx == nil || projectID <= 0 {
		return nil, fmt.Errorf("Scrum board response requires context and a positive project ID")
	}
	if _, err := queue.ParseScrumCardColumn(visibleColumn); err != nil {
		return nil, err
	}
	if cardOffset < 0 || cardOffset > maxScrumCardPageOffset {
		return nil, fmt.Errorf("Scrum board card offset is outside its accepted range")
	}
	board, err := s.scrumBoardMetadataFromProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	fullBoard := board
	columnCounts, err := s.scrumColumnCountsFromRepository(ctx, projectID)
	if err != nil {
		return nil, err
	}
	pageCards, cardHasMore, err := s.scrumCardColumnPage(ctx, projectID, visibleColumn, cardOffset)
	if err != nil {
		return nil, err
	}
	board.Columns = []string{visibleColumn}
	board.Cards = pageCards
	playQueue, err := s.scrumPlayQueuePayload(ctx, projectID)
	if err != nil {
		return nil, err
	}
	flowSummary, err := s.scrumFlowSummaryFromRepository(ctx, projectID)
	if err != nil {
		return nil, err
	}
	cardsByCol, err := cardsByColumn(board)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"board":          board,
		"cards_by_col":   cardsByCol,
		"play_queue":     playQueue,
		"flow_summary":   flowSummary,
		"all_columns":    append([]string(nil), fullBoard.Columns...),
		"visible_column": visibleColumn,
		"column_counts":  columnCounts,
		"card_offset":    cardOffset,
		"card_has_more":  cardHasMore,
	}
	if projectID > 0 {
		automation, err := s.scrumAutomationSettings(ctx, projectID)
		if err != nil {
			return nil, err
		}
		payload["project_id"] = projectID
		payload["auto_work"] = automation.AutoWork
		complete, err := s.repo.ScrumProjectComplete(ctx, projectID)
		if err != nil {
			return nil, err
		}
		payload["auto_work_complete"] = complete
		fullBoard, err = s.scrumFocusBoard(ctx, projectID, fullBoard, automation.AutoWork)
		if err != nil {
			return nil, err
		}
	}
	if err := scrumBoardFragmentsForPayload(payload, fullBoard); err != nil {
		return nil, err
	}
	return payload, nil
}
