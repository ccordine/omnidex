package api

import (
	"context"
	"fmt"
	"time"
)

func (s *Server) scrumChannelPage(
	ctx context.Context,
	projectID int64,
	cardID string,
	limit int,
	before string,
) (scrumChannelMessagePage, string, error) {
	if s == nil || s.repo == nil {
		return scrumChannelMessagePage{}, "", fmt.Errorf("PostgreSQL is required for Scrum channel paging")
	}
	ordinal, err := parseScrumChannelCursor(before)
	if err != nil {
		return scrumChannelMessagePage{}, "", err
	}
	stored, err := s.repo.ScrumChannelPage(ctx, projectID, cardID, limit, ordinal)
	if err != nil {
		return scrumChannelMessagePage{}, "", err
	}
	messages := make([]ScrumChatMessage, 0, len(stored.Messages))
	for _, message := range stored.Messages {
		messages = append(messages, ScrumChatMessage{
			ID: message.ID, Role: message.Role, Content: message.Content,
			CreatedAt: message.CreatedAt.UTC().Format(time.RFC3339Nano),
			Status:    message.Status, OperationID: message.OperationID,
		})
	}
	beforeCursor, err := encodeScrumChannelCursor(stored.Start, stored.HasMore)
	if err != nil {
		return scrumChannelMessagePage{}, "", err
	}
	page := scrumChannelMessagePage{
		Messages: messages, BeforeCursor: beforeCursor,
		HasMore: stored.HasMore, Total: stored.Total,
	}
	if _, err := scrumChannelBusyState(stored.PlayState); err != nil {
		return scrumChannelMessagePage{}, "", err
	}
	return page, stored.PlayState, nil
}

func (s *Server) scrumChannelCardProjection(
	ctx context.Context,
	projectID int64,
	cardID string,
	limit int,
) (ScrumCard, error) {
	if s == nil || s.repo == nil {
		return ScrumCard{}, fmt.Errorf("PostgreSQL is required for Scrum card projection")
	}
	stored, err := s.repo.GetScrumCard(ctx, projectID, cardID)
	if err != nil {
		return ScrumCard{}, err
	}
	card, err := dbScrumCardToAPI(stored)
	if err != nil {
		return ScrumCard{}, err
	}
	page, _, err := s.scrumChannelPage(ctx, projectID, card.ID, limit, "")
	if err != nil {
		return ScrumCard{}, err
	}
	card.Chat = page.Messages
	card.ChatCount = page.Total
	card.ChannelBeforeCursor = page.BeforeCursor
	card.ChannelHasMore = page.HasMore
	return card, nil
}

func scrumChannelBusyState(playState string) (bool, error) {
	switch playState {
	case scrumPlayRunning, scrumPlayQueued:
		return true, nil
	case "", scrumPlayPaused:
		return false, nil
	}
	return false, fmt.Errorf("Scrum channel play state %q is not registered", playState)
}
