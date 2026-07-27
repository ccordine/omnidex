package api

import (
	"context"
	"fmt"
	"strings"
)

// refreshScrumCardLlmJobs reconciles tag/ticket LLM job columns on every board read so
// clients only observe server-authoritative card state.
func (s *Server) refreshScrumCardLlmJobs(ctx context.Context, projectID int64, board ScrumBoard) (ScrumBoard, error) {
	if s.repo == nil || projectID <= 0 {
		return board, fmt.Errorf("postgres repository and project are required to refresh Scrum card LLM jobs")
	}
	for i, card := range board.Cards {
		prevTags := strings.TrimSpace(card.TagsJobID)
		prevTicket := strings.TrimSpace(card.TicketJobID)
		reconciled, changed, err := s.reconcileScrumCardLlmJobs(ctx, projectID, card)
		if err != nil {
			return board, err
		}
		if !changed {
			continue
		}
		saved, err := s.persistScrumCardFromContext(ctx, projectID, reconciled)
		if err != nil {
			return board, err
		}
		board.Cards[i] = saved
		if toast, tone := scrumCardLLMCompletionToast(prevTags, prevTicket, saved); toast != "" {
			s.publishScrumCardUpdateWithToast(ctx, projectID, saved, "llm job finished", toast, tone)
		}
	}
	return board, nil
}

func scrumCardLLMCompletionToast(prevTags, prevTicket string, card ScrumCard) (toast, tone string) {
	title := strings.TrimSpace(card.Title)
	if title == "" {
		title = card.ID
	}
	if prevTags != "" && strings.TrimSpace(card.TagsJobID) == "" {
		return fmt.Sprintf("Tags updated for %s", title), "ok"
	}
	if prevTicket != "" && strings.TrimSpace(card.TicketJobID) == "" {
		return fmt.Sprintf("Card ticket updated for %s", title), "ok"
	}
	return "", ""
}
