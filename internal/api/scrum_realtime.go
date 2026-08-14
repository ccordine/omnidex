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

func (s *Server) publishScrumCardUpdateWithToast(ctx context.Context, projectID int64, card ScrumCard, reason, toast, toastTone string) {
	if strings.TrimSpace(card.ID) == "" {
		log.Printf("scrum realtime card update rejected project=%d reason=%q: card id required", projectID, reason)
		return
	}
	projected, err := s.scrumChannelCardProjection(ctx, projectID, card.ID, scrumRealtimeChannelPageSize)
	if err != nil {
		log.Printf("scrum realtime channel projection rejected project=%d card=%s reason=%q: %v", projectID, card.ID, reason, err)
		return
	}
	card = projected
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
		loaded, err = s.scrumBoardMetadataFromProject(ctx, projectID)
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
	board, err := s.scrumBoardMetadataFromProject(ctx, projectID)
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
	automation, err := s.scrumAutomationSettings(ctx, projectID)
	if err != nil {
		return "", err
	}
	autoWork := automation.AutoWork
	focusBoard, err := s.scrumFocusBoard(ctx, projectID, board, autoWork)
	if err != nil {
		return "", err
	}
	visibleColumn, err := scrumRealtimeViewportColumn(focusBoard, autoWork)
	if err != nil {
		return "", err
	}
	columnCounts, err := s.scrumColumnCountsFromRepository(ctx, projectID)
	if err != nil {
		return "", err
	}
	pageCards, cardHasMore, err := s.scrumCardColumnPage(ctx, projectID, visibleColumn, 0)
	if err != nil {
		return "", err
	}
	viewportBoard := board
	viewportBoard.Columns = []string{visibleColumn}
	viewportBoard.Cards = pageCards
	cardsByCol, err := cardsByColumn(viewportBoard)
	if err != nil {
		return "", err
	}
	playQueue, err := s.scrumPlayQueuePayload(ctx, projectID)
	if err != nil {
		return "", err
	}
	flowSummary, err := s.scrumFlowSummaryFromRepository(ctx, projectID)
	if err != nil {
		return "", err
	}
	complete, err := s.repo.ScrumProjectComplete(ctx, projectID)
	if err != nil {
		return "", err
	}
	fragments, err := renderScrumBoardFragments(
		viewportBoard,
		cardsByCol,
		focusBoard,
		visibleColumn,
		columnCounts,
		playQueue,
		autoWork.Enabled,
		autoWork,
		complete,
		flowSummary,
		scrumCardPageState{Count: len(pageCards), HasMore: cardHasMore},
	)
	if err != nil {
		return "", err
	}
	return fragments.Bundle, nil
}

func scrumRealtimeViewportColumn(board ScrumBoard, autoWork ScrumAutoWorkConfig) (string, error) {
	if running := findRunningScrumCardInBoard(board); running != nil {
		if col := normalizeScrumColumn(running.Column); col != "" {
			return col, nil
		}
		return "", fmt.Errorf("running Scrum card %q contains noncanonical column %q", running.ID, running.Column)
	}
	byCol, err := cardsByColumn(board)
	if err != nil {
		return "", err
	}
	validated, err := validateScrumAutoWorkConfig(autoWork)
	if err != nil {
		return "", err
	}
	for _, col := range validated.SourceColumns {
		if len(byCol[col]) > 0 {
			return col, nil
		}
	}
	return "assigned", nil
}

func findRunningScrumCardInBoard(board ScrumBoard) *ScrumCard {
	for i, card := range board.Cards {
		if card.PlayState == scrumPlayRunning {
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
