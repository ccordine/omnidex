package api

import (
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/queue"
)

func (s *Server) handleScrumFlowMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query, err := decodeScrumFlowMetricsQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "scrum flow metrics require database")
		return
	}
	page, err := s.repo.ListScrumCardPage(r.Context(), query.ProjectID, queue.ScrumCardPageRequest{
		Limit: query.Limit, Offset: query.Offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	board := ScrumBoard{Cards: make([]ScrumCard, 0, len(page.Items))}
	for _, stored := range page.Items {
		card, err := dbScrumCardSummaryToAPI(stored)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		board.Cards = append(board.Cards, card)
	}
	cards := make([]scrumFlowMetricsCardResponse, 0, len(board.Cards))
	for _, card := range board.Cards {
		flowMetrics, err := jsonRawOrObject(card.FlowMetrics)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("decode flow metrics for card %q: %v", card.ID, err))
			return
		}
		cards = append(cards, scrumFlowMetricsCardResponse{
			CardID: card.ID, Title: card.Title, Column: card.Column, FlowMetrics: flowMetrics,
		})
	}
	summary, err := s.scrumFlowSummaryFromRepository(r.Context(), query.ProjectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, scrumFlowMetricsResponse{
		ProjectID: query.ProjectID,
		Summary:   summary,
		Cards:     cards,
		Limit:     query.Limit,
		Offset:    query.Offset,
		HasMore:   page.HasMore,
	})
}

type scrumFlowMetricsCardResponse struct {
	CardID      string `json:"card_id"`
	Title       string `json:"title"`
	Column      string `json:"column"`
	FlowMetrics any    `json:"flow_metrics"`
}

type scrumFlowMetricsResponse struct {
	ProjectID int64                          `json:"project_id"`
	Summary   ScrumFlowProjectSummary        `json:"summary"`
	Cards     []scrumFlowMetricsCardResponse `json:"cards"`
	Limit     int                            `json:"limit"`
	Offset    int                            `json:"offset"`
	HasMore   bool                           `json:"has_more"`
}

func (s *Server) handleMetricsScrum(w http.ResponseWriter, r *http.Request) {
	s.handleScrumFlowMetrics(w, r)
}
