package api

import (
	"fmt"
	"net/http"
	"strings"
)

type scrumModalRenderContext struct {
	Card                ScrumCard
	Board               ScrumBoard
	ProjectID           int64
	Tab                 string
	Files               []string
	Dirs                []string
	FilePath            string
	FileParent          string
	FileHasParent       bool
	FileOffset          int
	FileHasPrevious     bool
	FilePreviousOffset  int
	FileHasMore         bool
	FileNextOffset      int
	PlayQueue           map[string]any
	PilotPending        bool
	ChannelBeforeCursor string
	ChannelHasMore      bool
}

func scrumPlayControlUnlocked(card ScrumCard) bool {
	if normalizeScrumColumn(card.Column) == "assigned" {
		return true
	}
	switch strings.TrimSpace(card.PlayState) {
	case "running", "queued", "paused":
		return true
	default:
		return false
	}
}

func (s *Server) buildScrumModalContext(
	r *http.Request,
	cardID string,
	query scrumModalQuery,
) (*scrumModalRenderContext, error) {
	cardID = strings.TrimSpace(cardID)
	if cardID == "" {
		return nil, fmt.Errorf("card id is required")
	}
	if s.repo == nil {
		return nil, fmt.Errorf("postgres repository is required for Scrum")
	}
	board, err := s.scrumBoardMetadataFromProject(r.Context(), query.ProjectID)
	if err != nil {
		return nil, err
	}
	stored, err := s.repo.GetScrumCard(r.Context(), query.ProjectID, cardID)
	if err != nil {
		return nil, err
	}
	card, err := dbScrumCardToAPI(stored)
	if err != nil {
		return nil, err
	}
	playQueue, err := s.scrumPlayQueuePayload(r.Context(), query.ProjectID)
	if err != nil {
		return nil, err
	}
	board.Cards = []ScrumCard{}
	ctx := &scrumModalRenderContext{
		Card: card, Board: board, ProjectID: query.ProjectID,
		Tab: string(query.Tab), PlayQueue: playQueue,
	}
	switch ctx.Tab {
	case "files":
		if err := s.populateScrumModalFileContext(
			r, board.ProjectDirectory, query.FilePath, query.FileOffset, ctx,
		); err != nil {
			return nil, err
		}
	case "channel":
		if err := s.populateScrumModalChannelContext(r, ctx); err != nil {
			return nil, err
		}
	}
	return ctx, nil
}

func (s *Server) populateScrumModalChannelContext(r *http.Request, ctx *scrumModalRenderContext) error {
	channelPage, _, err := s.scrumChannelPage(
		r.Context(), ctx.ProjectID, ctx.Card.ID, scrumChannelDefaultPageSize, "",
	)
	if err != nil {
		return err
	}
	ctx.Card.Chat = channelPage.Messages
	ctx.Card.ChatCount = channelPage.Total
	ctx.Card.ChannelBeforeCursor = channelPage.BeforeCursor
	ctx.Card.ChannelHasMore = channelPage.HasMore
	ctx.ChannelBeforeCursor = channelPage.BeforeCursor
	ctx.ChannelHasMore = channelPage.HasMore
	return nil
}
