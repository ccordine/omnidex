package api

import (
	"context"
	"fmt"
	"log"
	"strings"
)

func (s *Server) publishScrumCardUpdate(ctx context.Context, projectID int64, card ScrumCard, reason string) {
	s.publishScrumCardUpdateWithToast(ctx, projectID, card, reason, "", "")
}

func (s *Server) publishScrumCardUpdateWithToast(_ context.Context, projectID int64, card ScrumCard, reason, toast, toastTone string) {
	if strings.TrimSpace(card.ID) == "" {
		log.Printf("scrum realtime card update rejected project=%d reason=%q: card id required", projectID, reason)
		return
	}
	card = scrumCardChannelPayload(card, scrumRealtimeChannelPageSize)
	msg := realtimeMessage{
		EventName: "scrum-card-updated",
		StateKey:  fmt.Sprintf("scrum-card:%d:%s", projectID, strings.TrimSpace(card.ID)),
		Reason:    strings.TrimSpace(reason),
		Toast:     strings.TrimSpace(toast),
		ToastTone: strings.TrimSpace(toastTone),
		ProjectID: projectID,
		CardID:    strings.TrimSpace(card.ID),
		Card:      &card,
	}
	s.broadcastRealtime([]string{realtimeTopicUI, realtimeTopicScrum}, msg)
}

func (s *Server) publishScrumBoardRefresh(ctx context.Context, projectID int64, reason string, board ScrumBoard) error {
	return s.publishScrumBoardRefreshWithToast(ctx, projectID, reason, board, "", "")
}

func (s *Server) publishScrumBoardRefreshWithToast(ctx context.Context, projectID int64, reason string, board ScrumBoard, toast, toastTone string) error {
	if projectID <= 0 {
		return fmt.Errorf("Scrum realtime board update requires a positive project ID")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "board updated"
	}
	msg := realtimeMessage{
		EventName: "scrum-board-refresh",
		StateKey:  fmt.Sprintf("scrum-board:%d", projectID),
		Reason:    reason,
		Toast:     strings.TrimSpace(toast),
		ToastTone: strings.TrimSpace(toastTone),
		ProjectID: projectID,
	}
	var bundle string
	var err error
	if len(board.Cards) > 0 || board.ID != "" {
		bundle, err = s.scrumBoardRealtimeHTML(ctx, projectID, board)
	} else {
		var loaded ScrumBoard
		loaded, err = s.scrumBoardFromProject(ctx, projectID)
		if err == nil {
			bundle, err = s.scrumBoardRealtimeHTML(ctx, projectID, loaded)
		}
	}
	if err != nil {
		s.publishScrumRealtimeFailure(projectID, reason, err)
		return err
	}
	msg.HTML = bundle
	if strings.TrimSpace(msg.HTML) == "" {
		err := fmt.Errorf("server-rendered Scrum board bundle is empty")
		s.publishScrumRealtimeFailure(projectID, reason, err)
		return err
	}
	if _, err := s.broadcastRealtimeChecked([]string{realtimeTopicUI, realtimeTopicScrum}, msg); err != nil {
		s.publishScrumRealtimeFailure(projectID, reason, err)
		return err
	}
	return nil
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
		s.publishScrumRealtimeFailure(projectID, "column "+nextCol, err)
		return
	}
	if err := s.publishScrumBoardRefreshWithToast(ctx, projectID, "column "+nextCol, board, toast, tone); err != nil {
		log.Printf("scrum realtime column transition failed project=%d card=%q: %v", projectID, saved.ID, err)
	}
}

func (s *Server) publishScrumRealtimeFailure(projectID int64, reason string, err error) {
	if err == nil {
		return
	}
	log.Printf("scrum realtime synchronization failed project=%d reason=%q: %v", projectID, reason, err)
	s.broadcastRealtime([]string{realtimeTopicUI, realtimeTopicScrum}, realtimeMessage{
		EventName: "scrum-sync-error",
		StateKey:  fmt.Sprintf("scrum-sync-error:%d", projectID),
		Reason:    strings.TrimSpace(reason),
		Toast:     "Live Scrum refresh failed; the last confirmed server state remains visible.",
		ToastTone: "error",
		ProjectID: projectID,
	})
}

func (s *Server) scrumBoardRealtimeHTML(ctx context.Context, projectID int64, board ScrumBoard) (string, error) {
	if projectID <= 0 {
		return "", fmt.Errorf("positive project id is required to render a realtime Scrum board")
	}
	fullBoard := board
	if err := s.refreshScrumFlowMetricsForBoard(ctx, projectID, &fullBoard); err != nil {
		return "", err
	}
	automation, err := s.scrumAutomationSettings(ctx, projectID)
	if err != nil {
		return "", err
	}
	autoWork := automation.AutoWork
	autoReview := automation.AutoReview
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
	return fragments.Bundle, nil
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
