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
		ProjectID: projectID,
		CardID:    strings.TrimSpace(card.ID),
	}
	s.ensureRealtimeHub().Broadcast([]string{"ui", "scrum"}, msg)
}
