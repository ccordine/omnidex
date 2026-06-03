package api

import (
	"context"
	"fmt"
	"strings"
)

func (s *Server) publishScrumModalCardRefresh(ctx context.Context, projectID int64, card ScrumCard, reason string) {
	s.publishScrumModalCardRefreshWithToast(ctx, projectID, card, reason, "", "")
}

func (s *Server) publishScrumModalCardRefreshWithToast(ctx context.Context, projectID int64, card ScrumCard, reason, toast, toastTone string) {
	if strings.TrimSpace(card.ID) == "" {
		return
	}
	msg := realtimeMessage{
		ID:        s.nextRealtimeID(),
		HTML:      renderScrumCardLLMSectionBundle(card),
		EventName: "scrum-card-modal-refresh",
		Reason:    strings.TrimSpace(reason),
		Toast:     strings.TrimSpace(toast),
		ToastTone: strings.TrimSpace(toastTone),
		ProjectID: projectID,
		CardID:    strings.TrimSpace(card.ID),
	}
	s.ensureRealtimeHub().Broadcast([]string{"ui", "scrum"}, msg)
}

func (s *Server) publishScrumCardChatUpdate(ctx context.Context, projectID int64, card ScrumCard, reason string) {
	if strings.TrimSpace(card.ID) == "" {
		return
	}
	msg := realtimeMessage{
		ID:        s.nextRealtimeID(),
		HTML:      renderScrumCardChatBundle(card),
		EventName: "chat-component-update",
		Reason:    strings.TrimSpace(reason),
		ProjectID: projectID,
		CardID:    strings.TrimSpace(card.ID),
	}
	s.ensureRealtimeHub().Broadcast([]string{"ui", "scrum"}, msg)
}

func (s *Server) publishScrumBoardRefresh(ctx context.Context, projectID int64, reason string, board ScrumBoard) {
	s.publishScrumBoardRefreshWithToast(ctx, projectID, reason, board, "", "")
}

func (s *Server) publishScrumBoardRefreshWithToast(ctx context.Context, projectID int64, reason string, board ScrumBoard, toast, toastTone string) {
	if projectID <= 0 {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "board updated"
	}
	msg := realtimeMessage{
		ID:        s.nextRealtimeID(),
		EventName: "scrum-board-refresh",
		Reason:    reason,
		Toast:     strings.TrimSpace(toast),
		ToastTone: strings.TrimSpace(toastTone),
		ProjectID: projectID,
	}
	if len(board.Cards) > 0 || board.ID != "" {
		msg.HTML = s.scrumBoardRealtimeHTML(ctx, projectID, board)
	} else if board, err := s.scrumBoardFromProject(ctx, projectID); err == nil {
		msg.HTML = s.scrumBoardRealtimeHTML(ctx, projectID, board)
	}
	s.ensureRealtimeHub().Broadcast([]string{"ui", "scrum"}, msg)
}

// notifyScrumCardColumnTransition emits a toast (and board bundle) only when a card lands in in_progress or review.
func (s *Server) notifyScrumCardColumnTransition(ctx context.Context, projectID int64, previous, saved ScrumCard) {
	if s == nil || projectID <= 0 || strings.TrimSpace(saved.ID) == "" {
		return
	}
	prevCol := normalizeScrumColumn(previous.Column)
	nextCol := normalizeScrumColumn(saved.Column)
	if prevCol == nextCol {
		return
	}
	var toast, tone string
	title := scrumCardShortTitle(saved)
	switch nextCol {
	case "in_progress":
		if prevCol != "in_progress" {
			toast = fmt.Sprintf("Working on %s", title)
			tone = "busy"
		}
	case "review":
		if prevCol != "review" {
			toast = fmt.Sprintf("%s moved to review", title)
			tone = "ok"
		}
	default:
		return
	}
	if toast == "" {
		return
	}
	board, err := s.scrumBoardFromProject(ctx, projectID)
	if err != nil {
		s.publishScrumBoardRefreshWithToast(ctx, projectID, "column "+nextCol, ScrumBoard{}, toast, tone)
		return
	}
	s.publishScrumBoardRefreshWithToast(ctx, projectID, "column "+nextCol, board, toast, tone)
}

func (s *Server) scrumBoardRealtimeHTML(ctx context.Context, projectID int64, board ScrumBoard) string {
	if projectID <= 0 {
		return ""
	}
	fullBoard := board
	s.refreshScrumFlowMetricsForBoard(ctx, projectID, &fullBoard)
	autoWork := s.scrumAutoWorkConfig(ctx, projectID)
	autoReview := s.scrumAutoReviewConfig(ctx, projectID)
	visibleColumn := scrumRealtimeViewportColumn(fullBoard, autoWork)
	columnCounts := scrumColumnCounts(cardsByColumn(fullBoard))
	viewportBoard := scrumBoardColumnViewport(fullBoard, visibleColumn)
	cardsByCol := cardsByColumn(viewportBoard)
	playQueue := scrumPlayQueueSummary(fullBoard)
	flowSummary := summarizeScrumFlowMetrics(fullBoard.Cards)
	fragments := renderScrumBoardFragments(
		viewportBoard,
		cardsByCol,
		fullBoard,
		visibleColumn,
		columnCounts,
		playQueue,
		autoWork.Enabled,
		autoReview,
		autoWork,
		flowSummary,
	)
	return fragments.Bundle
}

func scrumRealtimeViewportColumn(board ScrumBoard, autoWork ScrumAutoWorkConfig) string {
	if running := findRunningScrumCardInBoard(board); running != nil {
		if col := normalizeScrumColumn(running.Column); col != "" {
			return col
		}
	}
	byCol := cardsByColumn(board)
	for _, col := range normalizeScrumAutoWorkColumns(autoWork.SourceColumns) {
		if len(byCol[col]) > 0 {
			return col
		}
	}
	return "assigned"
}

func findRunningScrumCardInBoard(board ScrumBoard) *ScrumCard {
	for i, card := range board.Cards {
		if card.PlayState == scrumPlayRunning || card.PlayState == scrumPlayReviewing {
			return &board.Cards[i]
		}
	}
	return nil
}

func scrumCardShortTitle(card ScrumCard) string {
	title := strings.TrimSpace(card.Title)
	if title != "" {
		return title
	}
	id := strings.TrimSpace(card.ID)
	if len(id) > 12 {
		return id[:12] + "…"
	}
	return id
}
