package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

const (
	scrumFlowEventColumnMove   = "column_move"
	scrumFlowEventPlayStarted  = "play_started"
	scrumFlowEventPlayFinished = "play_finished"
	scrumFlowEventPlayPaused   = "play_paused"
	scrumFlowEventConversation = "conversation"
)

type ScrumFlowMetrics struct {
	AssignedReturns     int      `json:"assigned_returns"`
	ReviewBounces       int      `json:"review_bounces"`
	RegressionCount     int      `json:"regression_count"`
	PlayRuns            int      `json:"play_runs"`
	ChannelMessages     int      `json:"channel_messages"`
	PlanningMessages    int      `json:"planning_messages"`
	ConversationChars   int      `json:"conversation_chars"`
	IncompleteScore     int      `json:"incomplete_score"`
	CompletionStatus    string   `json:"completion_status"`
	Signals             []string `json:"signals"`
	LastPlayOutcome     string   `json:"last_play_outcome,omitempty"`
	Column              string   `json:"column,omitempty"`
	UpdatedAt           string   `json:"updated_at,omitempty"`
}

type ScrumFlowProjectSummary struct {
	TotalCards           int `json:"total_cards"`
	LikelyIncomplete     int `json:"likely_incomplete"`
	Uncertain            int `json:"uncertain"`
	LikelyComplete       int `json:"likely_complete"`
	AssignedReturnsTotal int `json:"assigned_returns_total"`
	LongConversations    int `json:"long_conversations"`
}

var scrumColumnRank = map[string]int{
	"backlog":     0,
	"ready":       1,
	"assigned":    2,
	"in_progress": 3,
	"review":      4,
	"blocked":     4,
	"done":        5,
}

func scrumColumnRankValue(column string) int {
	if rank, ok := scrumColumnRank[normalizeScrumColumn(column)]; ok {
		return rank
	}
	return 0
}

func isScrumRegressionToAssigned(fromColumn, toColumn string) bool {
	to := normalizeScrumColumn(toColumn)
	from := normalizeScrumColumn(fromColumn)
	if to != "assigned" || from == "" || from == to {
		return false
	}
	return scrumColumnRankValue(from) > scrumColumnRankValue("assigned")
}

func isScrumReviewBounce(fromColumn, toColumn string) bool {
	from := normalizeScrumColumn(fromColumn)
	to := normalizeScrumColumn(toColumn)
	if from != "review" {
		return false
	}
	return to == "assigned" || to == "in_progress"
}

func isScrumColumnRegression(fromColumn, toColumn string) bool {
	from := normalizeScrumColumn(fromColumn)
	to := normalizeScrumColumn(toColumn)
	if from == "" || to == "" || from == to {
		return false
	}
	return scrumColumnRankValue(to) < scrumColumnRankValue(from)
}

func conversationStats(card ScrumCard) (channelMessages, planningMessages, totalChars int) {
	for _, msg := range card.Chat {
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		channelMessages++
		totalChars += len(msg.Content)
	}
	for _, msg := range card.PlanningChat {
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		planningMessages++
		totalChars += len(msg.Content)
	}
	return channelMessages, planningMessages, totalChars
}

func checklistIncomplete(card ScrumCard) bool {
	if len(card.Checklist) == 0 {
		return false
	}
	for _, item := range card.Checklist {
		if !item.Done {
			return true
		}
	}
	return false
}

func computeScrumFlowMetrics(card ScrumCard, events []queue.ScrumFlowEvent) ScrumFlowMetrics {
	metrics := ScrumFlowMetrics{
		Column:           normalizeScrumColumn(card.Column),
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
		CompletionStatus: "uncertain",
		Signals:          []string{},
	}
	metrics.ChannelMessages, metrics.PlanningMessages, metrics.ConversationChars = conversationStats(card)

	for _, event := range events {
		switch event.EventType {
		case scrumFlowEventColumnMove:
			if isScrumRegressionToAssigned(event.FromColumn, event.ToColumn) {
				metrics.AssignedReturns++
				metrics.RegressionCount++
			} else if isScrumReviewBounce(event.FromColumn, event.ToColumn) {
				metrics.ReviewBounces++
				metrics.RegressionCount++
			} else if isScrumColumnRegression(event.FromColumn, event.ToColumn) {
				metrics.RegressionCount++
			}
		case scrumFlowEventPlayStarted:
			metrics.PlayRuns++
		case scrumFlowEventPlayFinished:
			var payload map[string]any
			if err := json.Unmarshal(event.Payload, &payload); err == nil {
				if outcome, ok := payload["outcome"].(string); ok {
					metrics.LastPlayOutcome = strings.TrimSpace(outcome)
				}
			}
		}
	}

	score := 0
	addSignal := func(text string, weight int) {
		if weight <= 0 {
			return
		}
		score += weight
		metrics.Signals = append(metrics.Signals, text)
	}

	if metrics.AssignedReturns > 0 {
		addSignal(fmt.Sprintf("returned to assigned %d time(s) after review or later", metrics.AssignedReturns), metrics.AssignedReturns*25)
	}
	if metrics.ReviewBounces > 0 {
		addSignal(fmt.Sprintf("bounced out of review %d time(s)", metrics.ReviewBounces), metrics.ReviewBounces*20)
	}
	if metrics.RegressionCount > metrics.AssignedReturns+metrics.ReviewBounces {
		extra := metrics.RegressionCount - metrics.AssignedReturns - metrics.ReviewBounces
		addSignal(fmt.Sprintf("%d other column regression(s)", extra), extra*10)
	}
	if metrics.PlayRuns > 2 {
		addSignal(fmt.Sprintf("played %d times", metrics.PlayRuns), (metrics.PlayRuns-2)*10)
	}
	totalMessages := metrics.ChannelMessages + metrics.PlanningMessages
	if totalMessages >= 30 && metrics.Column != "done" {
		addSignal(fmt.Sprintf("long conversation (%d messages) without reaching done", totalMessages), 15)
	}
	if metrics.ConversationChars >= 10000 && metrics.Column != "done" {
		addSignal(fmt.Sprintf("heavy discussion (~%dk chars) still open", metrics.ConversationChars/1000), 10)
	}
	if metrics.Column == "blocked" {
		addSignal("currently blocked", 20)
	}
	if checklistIncomplete(card) && (metrics.Column == "review" || metrics.Column == "done") {
		addSignal("checklist still incomplete in review/done", 15)
	}
	if metrics.LastPlayOutcome == "failed" || metrics.LastPlayOutcome == "blocked" {
		addSignal("last play outcome: "+metrics.LastPlayOutcome, 15)
	}

	metrics.IncompleteScore = score
	switch {
	case score >= 50:
		metrics.CompletionStatus = "likely_incomplete"
	case score <= 15 && metrics.Column == "done" && metrics.AssignedReturns == 0:
		metrics.CompletionStatus = "likely_complete"
	default:
		metrics.CompletionStatus = "uncertain"
	}
	return metrics
}

func summarizeScrumFlowMetrics(cards []ScrumCard) ScrumFlowProjectSummary {
	summary := ScrumFlowProjectSummary{TotalCards: len(cards)}
	for _, card := range cards {
		metrics := parseScrumFlowMetrics(card.FlowMetrics)
		summary.AssignedReturnsTotal += metrics.AssignedReturns
		if metrics.ChannelMessages+metrics.PlanningMessages >= 30 {
			summary.LongConversations++
		}
		switch metrics.CompletionStatus {
		case "likely_incomplete":
			summary.LikelyIncomplete++
		case "likely_complete":
			summary.LikelyComplete++
		default:
			summary.Uncertain++
		}
	}
	return summary
}

func parseScrumFlowMetrics(raw json.RawMessage) ScrumFlowMetrics {
	out := ScrumFlowMetrics{CompletionStatus: "uncertain", Signals: []string{}}
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	if out.Signals == nil {
		out.Signals = []string{}
	}
	return out
}

func (s *Server) trackScrumCardFlow(ctx context.Context, projectID int64, previous, next ScrumCard, trigger string) json.RawMessage {
	if s.repo == nil || projectID <= 0 || strings.TrimSpace(next.ID) == "" {
		return next.FlowMetrics
	}
	prevColumn := normalizeScrumColumn(previous.Column)
	nextColumn := normalizeScrumColumn(next.Column)
	prevPlay := strings.TrimSpace(previous.PlayState)
	nextPlay := strings.TrimSpace(next.PlayState)

	if prevColumn != nextColumn {
		payload, _ := json.Marshal(map[string]any{
			"trigger":       trigger,
			"is_regression": isScrumColumnRegression(prevColumn, nextColumn),
			"to_assigned":   isScrumRegressionToAssigned(prevColumn, nextColumn),
			"review_bounce": isScrumReviewBounce(prevColumn, nextColumn),
		})
		_ = s.repo.RecordScrumFlowEvent(ctx, projectID, next.ID, scrumFlowEventColumnMove, prevColumn, nextColumn, prevPlay, nextPlay, payload)
	}

	if prevPlay != nextPlay {
		switch nextPlay {
		case scrumPlayRunning:
			payload, _ := json.Marshal(map[string]any{"trigger": trigger, "job_id": strings.TrimSpace(next.JobID)})
			_ = s.repo.RecordScrumFlowEvent(ctx, projectID, next.ID, scrumFlowEventPlayStarted, prevColumn, nextColumn, prevPlay, nextPlay, payload)
		case scrumPlayPaused:
			payload, _ := json.Marshal(map[string]any{"trigger": trigger, "job_id": strings.TrimSpace(previous.JobID)})
			_ = s.repo.RecordScrumFlowEvent(ctx, projectID, next.ID, scrumFlowEventPlayPaused, prevColumn, nextColumn, prevPlay, nextPlay, payload)
		}
	}

	prevChannel, prevPlanning, _ := conversationStats(previous)
	nextChannel, nextPlanning, nextChars := conversationStats(next)
	if nextChannel+nextPlanning > prevChannel+prevPlanning {
		payload, _ := json.Marshal(map[string]any{
			"trigger":            trigger,
			"channel_messages":   nextChannel,
			"planning_messages":  nextPlanning,
			"conversation_chars": nextChars,
		})
		_ = s.repo.RecordScrumFlowEvent(ctx, projectID, next.ID, scrumFlowEventConversation, nextColumn, nextColumn, nextPlay, nextPlay, payload)
	}

	events, err := s.repo.ListScrumFlowEvents(ctx, projectID, next.ID, 200)
	if err != nil {
		return next.FlowMetrics
	}
	metrics := computeScrumFlowMetrics(next, events)
	raw, _ := json.Marshal(metrics)
	_ = s.repo.UpdateScrumCardFlowMetrics(ctx, projectID, next.ID, raw)
	return raw
}

func (s *Server) refreshScrumFlowMetricsForBoard(ctx context.Context, projectID int64, board *ScrumBoard) {
	if s.repo == nil || projectID <= 0 || board == nil {
		return
	}
	for i, card := range board.Cards {
		events, err := s.repo.ListScrumFlowEvents(ctx, projectID, card.ID, 200)
		if err != nil {
			continue
		}
		metrics := computeScrumFlowMetrics(card, events)
		raw, _ := json.Marshal(metrics)
		board.Cards[i].FlowMetrics = raw
	}
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
	s.refreshScrumFlowMetricsForBoard(r.Context(), projectID, &board)
	cards := make([]map[string]any, 0, len(board.Cards))
	for _, card := range board.Cards {
		cards = append(cards, map[string]any{
			"card_id":       card.ID,
			"title":         card.Title,
			"column":        card.Column,
			"flow_metrics":  jsonRawOrObject(card.FlowMetrics),
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
