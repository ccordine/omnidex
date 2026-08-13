package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

func (s *Server) trackScrumCardFlow(ctx context.Context, projectID int64, previous, next ScrumCard, trigger string) json.RawMessage {
	if s.repo == nil || projectID <= 0 || strings.TrimSpace(next.ID) == "" {
		log.Printf("scrum flow tracking rejected project=%d card=%q: repository, project, and card are required", projectID, next.ID)
		return next.FlowMetrics
	}
	prevColumn := normalizeScrumColumn(previous.Column)
	nextColumn := normalizeScrumColumn(next.Column)
	prevPlay := strings.TrimSpace(previous.PlayState)
	nextPlay := strings.TrimSpace(next.PlayState)

	if prevColumn != nextColumn {
		s.recordScrumFlowEvent(ctx, projectID, next.ID, scrumFlowEventColumnMove, prevColumn, nextColumn, prevPlay, nextPlay, map[string]any{
			"trigger":       trigger,
			"is_regression": isScrumColumnRegression(prevColumn, nextColumn),
			"to_assigned":   isScrumRegressionToAssigned(prevColumn, nextColumn),
			"review_bounce": isScrumReviewBounce(prevColumn, nextColumn),
		})
	}

	if prevPlay != nextPlay {
		switch nextPlay {
		case scrumPlayRunning:
			s.recordScrumFlowEvent(ctx, projectID, next.ID, scrumFlowEventPlayStarted, prevColumn, nextColumn, prevPlay, nextPlay, map[string]any{"trigger": trigger, "job_id": strings.TrimSpace(next.JobID)})
		case scrumPlayPaused:
			s.recordScrumFlowEvent(ctx, projectID, next.ID, scrumFlowEventPlayPaused, prevColumn, nextColumn, prevPlay, nextPlay, map[string]any{"trigger": trigger, "job_id": strings.TrimSpace(previous.JobID)})
		}
	}

	prevChannel, _ := conversationStats(previous)
	nextChannel, nextChars := conversationStats(next)
	if nextChannel > prevChannel {
		s.recordScrumFlowEvent(ctx, projectID, next.ID, scrumFlowEventConversation, nextColumn, nextColumn, nextPlay, nextPlay, map[string]any{
			"trigger":            trigger,
			"channel_messages":   nextChannel,
			"conversation_chars": nextChars,
		})
	}

	events, err := s.repo.ListScrumFlowEvents(ctx, projectID, next.ID, 200)
	if err != nil {
		log.Printf("scrum flow event load failed project=%d card=%q: %v", projectID, next.ID, err)
		return next.FlowMetrics
	}
	metrics := computeScrumFlowMetrics(next, events)
	raw, err := json.Marshal(metrics)
	if err != nil {
		log.Printf("scrum flow metrics encode failed project=%d card=%q: %v", projectID, next.ID, err)
		return next.FlowMetrics
	}
	if err := s.repo.UpdateScrumCardFlowMetrics(ctx, projectID, next.ID, raw); err != nil {
		log.Printf("scrum flow metrics persistence failed project=%d card=%q: %v", projectID, next.ID, err)
	}
	return raw
}

func (s *Server) recordScrumFlowEvent(ctx context.Context, projectID int64, cardID, eventType, fromColumn, toColumn, fromPlayState, toPlayState string, payload map[string]any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		log.Printf("scrum flow event encode failed project=%d card=%q event=%q: %v", projectID, cardID, eventType, err)
		return
	}
	if err := s.repo.RecordScrumFlowEvent(ctx, projectID, cardID, eventType, fromColumn, toColumn, fromPlayState, toPlayState, raw); err != nil {
		log.Printf("scrum flow event persistence failed project=%d card=%q event=%q: %v", projectID, cardID, eventType, err)
	}
}

func (s *Server) refreshScrumFlowMetricsForBoard(ctx context.Context, projectID int64, board *ScrumBoard) error {
	if s.repo == nil || projectID <= 0 || board == nil {
		return fmt.Errorf("scrum flow board refresh requires repository, project, and board")
	}
	for i, card := range board.Cards {
		events, err := s.repo.ListScrumFlowEvents(ctx, projectID, card.ID, 200)
		if err != nil {
			return fmt.Errorf("load Scrum flow events project=%d card=%q: %w", projectID, card.ID, err)
		}
		metrics := computeScrumFlowMetrics(card, events)
		raw, err := json.Marshal(metrics)
		if err != nil {
			return fmt.Errorf("encode Scrum flow metrics project=%d card=%q: %w", projectID, card.ID, err)
		}
		board.Cards[i].FlowMetrics = raw
	}
	return nil
}

func (s *Server) handleScrumFlowMetrics(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "scrum flow metrics require database")
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	projectID, err := s.resolveProjectID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	board, err := s.scrumBoardFromProject(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.refreshScrumFlowMetricsForBoard(r.Context(), projectID, &board); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cards := make([]map[string]any, 0, len(board.Cards))
	for _, card := range board.Cards {
		flowMetrics, err := jsonRawOrObject(card.FlowMetrics)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("decode flow metrics for card %q: %v", card.ID, err))
			return
		}
		cards = append(cards, map[string]any{
			"card_id":      card.ID,
			"title":        card.Title,
			"column":       card.Column,
			"flow_metrics": flowMetrics,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project_id": projectID,
		"summary":    summarizeScrumFlowMetrics(board.Cards),
		"cards":      cards,
	})
}

func (s *Server) handleMetricsScrum(w http.ResponseWriter, r *http.Request) {
	s.handleScrumFlowMetrics(w, r)
}
