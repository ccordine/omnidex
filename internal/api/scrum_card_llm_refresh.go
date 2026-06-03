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
		return board, nil
	}
	for i, card := range board.Cards {
		prevTags := strings.TrimSpace(card.TagsJobID)
		prevTicket := strings.TrimSpace(card.TicketJobID)
		if reconciled, ok := s.reconcileScrumCardLlmJobs(ctx, projectID, card); ok {
			if saved, err := s.persistScrumCardFromContext(ctx, projectID, reconciled); err == nil {
				if dbCard, loadErr := s.repo.GetScrumCard(ctx, projectID, saved.ID); loadErr == nil {
					saved = dbScrumCardToAPI(dbCard)
				}
				board.Cards[i] = saved
				if toast, tone := scrumCardLLMCompletionToast(prevTags, prevTicket, saved); toast != "" {
					s.publishScrumModalCardRefreshWithToast(ctx, projectID, saved, "llm job finished", toast, tone)
				}
			}
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
