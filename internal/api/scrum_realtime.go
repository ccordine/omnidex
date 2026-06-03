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
	if toast == "" {
		toast, toastTone = scrumBoardRefreshToast(ctx, s, projectID, reason, board)
	}
	msg := realtimeMessage{
		ID:        s.nextRealtimeID(),
		HTML:      s.scrumBoardRealtimeHTML(ctx, projectID, board),
		EventName: "scrum-board-refresh",
		Reason:    reason,
		Toast:     strings.TrimSpace(toast),
		ToastTone: strings.TrimSpace(toastTone),
		ProjectID: projectID,
	}
	s.ensureRealtimeHub().Broadcast([]string{"ui", "scrum"}, msg)
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

func scrumBoardRefreshToast(ctx context.Context, s *Server, projectID int64, reason string, board ScrumBoard) (toast, tone string) {
	reason = strings.ToLower(strings.TrimSpace(reason))
	if s == nil {
		return "", ""
	}
	projectName := ""
	if s.repo != nil && projectID > 0 {
		if project, err := s.repo.GetProject(ctx, projectID); err == nil {
			projectName = strings.TrimSpace(project.Name)
		}
	}
	prefix := "Auto-work"
	if projectName != "" {
		prefix = fmt.Sprintf("Auto-work (%s)", projectName)
	}
	running := findRunningScrumCardInBoard(board)
	switch {
	case strings.Contains(reason, "job finished"), strings.Contains(reason, "global running reconcile"):
		if running != nil {
			return fmt.Sprintf("%s: working on %s", prefix, scrumCardShortTitle(*running)), "busy"
		}
		autoWork := s.scrumAutoWorkConfig(ctx, projectID)
		if autoWork.Enabled {
			reviewCfg := s.scrumAutoReviewConfig(ctx, projectID)
			if scrumAutoPlayThroughCompleteWithReview(board, reviewCfg.Enabled) {
				return fmt.Sprintf("%s: nothing left to run", prefix), "ok"
			}
			if s.nextAutoWorkScrumCard(board, autoWork) != nil {
				return fmt.Sprintf("%s: card finished; picking up next", prefix), "info"
			}
		}
		return "", ""
	case strings.Contains(reason, "global auto-work"):
		if running != nil {
			return fmt.Sprintf("%s: started %s", prefix, scrumCardShortTitle(*running)), "busy"
		}
		return "", ""
	case strings.Contains(reason, "auto-work started"):
		if running != nil {
			return fmt.Sprintf("%s: started %s", prefix, scrumCardShortTitle(*running)), "busy"
		}
		if autoWork := s.scrumAutoWorkConfig(ctx, projectID); autoWork.Enabled && s.nextAutoWorkScrumCard(board, autoWork) == nil {
			return fmt.Sprintf("%s: enabled; no eligible cards", prefix), "info"
		}
		return fmt.Sprintf("%s: enabled", prefix), "info"
	case strings.Contains(reason, "auto-work paused"), strings.Contains(reason, "project auto-work paused"):
		return fmt.Sprintf("%s: paused", prefix), "info"
	case strings.Contains(reason, "already running"):
		if running != nil {
			return fmt.Sprintf("%s: already running %s", prefix, scrumCardShortTitle(*running)), "info"
		}
		return fmt.Sprintf("%s: already running", prefix), "info"
	default:
		return "", ""
	}
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
