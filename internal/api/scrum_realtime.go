package api

import (
	"context"
	"fmt"
	"html"
	"strings"
)

func scrumModalCardLiveTarget(cardID string) string {
	cardID = strings.TrimSpace(cardID)
	if cardID == "" {
		return ""
	}
	return "scrum-modal-card-live-" + cardID
}

func renderScrumModalCardLiveHTML(card ScrumCard, reason string) string {
	target := scrumModalCardLiveTarget(card.ID)
	if target == "" {
		return ""
	}
	status := strings.TrimSpace(card.PlayState)
	if status == "" {
		status = normalizeScrumColumn(card.Column)
	}
	text := strings.TrimSpace(reason)
	if text == "" {
		text = "updated"
	}
	return fmt.Sprintf(
		`<template data-recyclr-target="%s"><span data-scrum-realtime-card="%s" data-scrum-realtime-status="%s" class="sr-only">%s</span></template>`,
		html.EscapeString(target),
		html.EscapeString(strings.TrimSpace(card.ID)),
		html.EscapeString(status),
		html.EscapeString(text),
	)
}

func (s *Server) publishScrumModalCardRefresh(ctx context.Context, projectID int64, card ScrumCard, reason string) {
	if strings.TrimSpace(card.ID) == "" {
		return
	}
	msg := realtimeMessage{
		ID:        s.nextRealtimeID(),
		HTML:      renderScrumModalCardLiveHTML(card, reason),
		EventName: "scrum-card-modal-refresh",
		Reason:    strings.TrimSpace(reason),
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
	if projectID <= 0 {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "board updated"
	}
	summary := scrumPlayQueueSummary(board)
	msg := realtimeMessage{
		ID:        s.nextRealtimeID(),
		EventName: "scrum-board-refresh",
		ProjectID: projectID,
		HTML:      renderScrumBoardLiveHTML(projectID, reason, summary),
	}
	s.ensureRealtimeHub().Broadcast([]string{"ui", "scrum"}, msg)
}

func renderScrumBoardLiveHTML(projectID int64, reason string, summary map[string]any) string {
	runningID, _ := summary["running_card_id"].(string)
	queuedCount := 0
	if raw, ok := summary["queued_count"].(int); ok {
		queuedCount = raw
	}
	return fmt.Sprintf(
		`<span data-scrum-board-refresh="%d" data-scrum-board-reason="%s" data-scrum-running-card="%s" data-scrum-queued-count="%d" class="sr-only">%s</span>`,
		projectID,
		html.EscapeString(reason),
		html.EscapeString(strings.TrimSpace(runningID)),
		queuedCount,
		html.EscapeString(reason),
	)
}
